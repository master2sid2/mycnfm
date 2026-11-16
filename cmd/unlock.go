package cmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"time"

	"mycnfm/internal/config"
	"mycnfm/internal/keyring"
	"mycnfm/internal/mycnf"
)

// passwordFromEnv returns the automation password, if one is configured:
// MYCNFM_PASSWORD (visible to other processes of the same user through
// /proc/<pid>/environ for the life of the process — convenient, not
// hardened) or MYCNFM_PASSWORD_FILE (a file read once, so the secret never
// sits in the environment). It's meant for cron/CI, where there is no login
// session to cache anything in.
func passwordFromEnv() (pw []byte, ok bool, err error) {
	if v, set := os.LookupEnv("MYCNFM_PASSWORD"); set {
		return []byte(v), true, nil
	}
	if path := os.Getenv("MYCNFM_PASSWORD_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false, fmt.Errorf("read MYCNFM_PASSWORD_FILE: %w", err)
		}
		return bytes.TrimRight(data, "\r\n"), true, nil
	}
	return nil, false, nil
}

// openVault unlocks the vault at vaultPath. Order:
//
//  1. the automation password from the environment, if set;
//  2. (only if useCache) the key cached in this login session's kernel
//     keyring by an earlier unlock — a stale one (vault re-keyed or replaced)
//     just fails authentication and is discarded;
//  3. an interactive password prompt, after which the derived key is cached
//     for the rest of the session so later runs don't ask again.
//
// The cache holds the derived key, never the password itself, and lives only
// in kernel memory: it disappears at logout or with `mycnfm lock`. useCache
// is false for actions that should re-prove knowledge of the password.
func openVaultUnreconciled(vaultPath string, useCache bool) (*config.Store, error) {
	if pw, ok, err := passwordFromEnv(); err != nil {
		return nil, err
	} else if ok {
		return openWithPassword(vaultPath, pw)
	}

	if useCache {
		if key, err := keyring.Load(vaultPath); err == nil {
			if store, err := config.OpenWithKey(vaultPath, key); err == nil {
				return store, nil
			}
			_ = keyring.Delete(vaultPath) // stale: drop it and ask normally
		}
	}

	// A new vault is encrypted with whatever is typed now, and a typo would
	// lock it for good: ask twice.
	_, statErr := os.Stat(vaultPath)
	creating := errors.Is(statErr, fs.ErrNotExist)
	prompt := "Master password: "
	if creating {
		fmt.Fprintf(os.Stderr, "No vault at %s yet: choose a master password for the new one.\n", vaultPath)
		prompt = "New master password: "
	}
	pw, err := readPassword(prompt)
	if err != nil {
		return nil, fmt.Errorf("read password: %w", err)
	}
	if creating {
		again, err := readPassword("Confirm master password: ")
		if err != nil {
			return nil, fmt.Errorf("read password: %w", err)
		}
		if !bytes.Equal(pw, again) {
			return nil, errors.New("passwords do not match; no vault was created")
		}
	}
	store, err := openWithPassword(vaultPath, pw)
	if err != nil {
		return nil, err
	}

	// Cache only for a vault that already exists on disk; a brand-new one
	// gets its salt (and so its key) at first save, so caching now is moot.
	if useCache {
		if _, statErr := os.Stat(vaultPath); statErr == nil {
			if err := keyring.Save(vaultPath, store.SessionKey()); err == nil {
				fmt.Fprintln(os.Stderr, "Unlocked for this session — `mycnfm lock` forgets it earlier.")
			}
		}
	}
	return store, nil
}

// openVault is openVaultUnreconciled plus a check that the active profile still
// matches ~/.my.cnf: a profile whose auto-wipe deadline passed while nothing
// was running (the watcher didn't survive a reboot, sleep or logout) is wiped
// now, and a file that was already wiped is dropped as the active profile.
func openVault(vaultPath string, useCache bool) (*config.Store, error) {
	store, err := openVaultUnreconciled(vaultPath, useCache)
	if err != nil {
		return nil, err
	}
	if path, perr := mycnf.DefaultPath(); perr == nil {
		if out, _ := mycnf.Reconcile(store, path, time.Now()); out == mycnf.Expired {
			fmt.Fprintln(os.Stderr, "The auto-wipe timer had expired: ~/.my.cnf was wiped.")
		}
	}
	return store, nil
}

func openWithPassword(vaultPath string, pw []byte) (*config.Store, error) {
	store, err := config.Open(vaultPath, pw)
	if err != nil {
		return nil, fmt.Errorf("open vault: %w", err)
	}
	return store, nil
}

// runLock forgets the key cached for the vault in this login session, so the
// next command asks for the master password again.
func runLock(args []string) error {
	fs := flag.NewFlagSet("lock", flag.ExitOnError)
	vaultFlag := fs.String("vault", "", "vault file (default: ~/.config/mycnfm/vault.enc)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	vaultPath := *vaultFlag
	if vaultPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve vault path: %w", err)
		}
		vaultPath = p
	}

	err := keyring.Delete(vaultPath)
	switch {
	case errors.Is(err, keyring.ErrUnsupported):
		fmt.Println("Nothing to forget: this platform doesn't cache the unlock key.")
		return nil
	case err != nil:
		return fmt.Errorf("forget cached key: %w", err)
	}
	fmt.Println("Locked: the cached key for this session was forgotten.")
	return nil
}
