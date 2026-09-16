package cmd

import (
	"flag"
	"fmt"
	"os"

	"mycnfm/internal/config"
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

	oldPass, err := readPassword("Current master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	store, err := config.Open(vaultPath, oldPass)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
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

	fmt.Printf("Password changed for %s\n", vaultPath)
	return nil
}
