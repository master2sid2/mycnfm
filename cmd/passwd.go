package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/master2sid2/mycnfm/internal/config"
	"github.com/master2sid2/mycnfm/internal/keyring"
)

// runPasswd changes the vault's master password in place: current password
// to unlock it, new password (typed twice) to re-encrypt it. Unlike
// export/import, plaintext never touches disk.
func runPasswd(args []string) error {
	fs := flag.NewFlagSet("passwd", flag.ExitOnError)
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
	if _, err := os.Stat(vaultPath); err != nil {
		return fmt.Errorf("vault not found at %s: %w", vaultPath, err)
	}

	// Changing the master password re-proves the current one: never satisfied
	// from the session cache.
	store, err := openVault(vaultPath, false)
	if err != nil {
		return err
	}

	newPass1, err := readPassword("New master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	newPass2, err := readPassword("Confirm new password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if string(newPass1) != string(newPass2) {
		return fmt.Errorf("passwords do not match")
	}

	if err := store.Rekey(newPass1); err != nil {
		return fmt.Errorf("rekey vault: %w", err)
	}
	_ = keyring.Delete(vaultPath) // the cached key belongs to the old password

	fmt.Printf("Password changed for %s\n", vaultPath)
	return nil
}
