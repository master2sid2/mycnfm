package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mycnfm/internal/config"
)

// defaultJSONPath returns ./mycnfm-export.json relative to the current
// working directory — deliberately not relative to the binary's own
// location, since the binary may be installed anywhere.
func defaultJSONPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Join(wd, "mycnfm-export.json"), nil
}

// runExport decrypts the vault (prompting for its master password) and
// writes its contents as plaintext JSON. Meant for one-off maintenance:
// changing the master password, or handing a config to a colleague who will
// re-encrypt it under their own password via `mycnfm import`.
func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	vaultFlag := fs.String("vault", "", "vault file (default: ~/.config/mycnfm/vault.enc)")
	jsonFlag := fs.String("json", "", "output JSON file (default: ./mycnfm-export.json)")
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

	jsonPath := *jsonFlag
	if jsonPath == "" {
		p, err := defaultJSONPath()
		if err != nil {
			return err
		}
		jsonPath = p
	}
	if _, err := os.Stat(jsonPath); err == nil {
		if !confirm(fmt.Sprintf("%s already exists and will be overwritten.", jsonPath)) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	pass, err := readPassword("Master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	store, err := config.Open(vaultPath, pass)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}

	data, err := store.ExportJSON()
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", jsonPath, err)
	}

	fmt.Printf("Exported %s to %s\n", vaultPath, jsonPath)
	fmt.Println("⚠  That file is PLAINTEXT and contains every stored password — delete it once you're done with it.")
	return nil
}

// runImport reads a plaintext JSON export and encrypts it into a vault under
// a newly chosen master password, warning before overwriting an existing
// vault at the destination.
func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	vaultFlag := fs.String("vault", "", "destination vault file (default: ~/.config/mycnfm/vault.enc)")
	jsonFlag := fs.String("json", "", "input JSON file (default: ./mycnfm-export.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	jsonPath := *jsonFlag
	if jsonPath == "" {
		p, err := defaultJSONPath()
		if err != nil {
			return err
		}
		jsonPath = p
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", jsonPath, err)
	}

	vaultPath := *vaultFlag
	if vaultPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve vault path: %w", err)
		}
		vaultPath = p
	}
	if _, err := os.Stat(vaultPath); err == nil {
		if !confirm(fmt.Sprintf("%s already exists and will be overwritten.", vaultPath)) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	pass1, err := readPassword("New master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	pass2, err := readPassword("Confirm password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if string(pass1) != string(pass2) {
		return fmt.Errorf("passwords do not match")
	}

	store, err := config.ImportJSON(vaultPath, pass1, data)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save vault: %w", err)
	}

	fmt.Printf("Imported %s into %s\n", jsonPath, vaultPath)
	return nil
}
