package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"mycnfm/internal/config"
	"mycnfm/internal/mycnf"
	"mycnfm/internal/tui"
	"mycnfm/internal/version"
)

// Execute is the CLI entry point. It parses minimal flags/subcommands,
// unlocks the vault, and hands control to the TUI.
func Execute(ctx context.Context) error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "clean":
			return runClean()
		case "export":
			return runExport(os.Args[2:])
		case "import":
			return runImport(os.Args[2:])
		case "passwd":
			return runPasswd(os.Args[2:])
		case "--version", "-v":
			fmt.Println("mycnfm", version.Version)
			return nil
		case "--help", "-h":
			printUsage()
			return nil
		default:
			printUsage()
			return fmt.Errorf("unknown command %q", os.Args[1])
		}
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("config path: %w", err)
	}

	rawPass, err := readPassword("Master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	store, err := config.Open(cfgPath, rawPass)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}

	return tui.Run(ctx, store)
}

// runClean wipes ~/.my.cnf back to a credential-free state without touching
// the vault or requiring the master password.
func runClean() error {
	path, err := mycnf.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve ~/.my.cnf: %w", err)
	}
	if err := mycnf.Clean(path); err != nil {
		return fmt.Errorf("clean %s: %w", path, err)
	}
	fmt.Printf("%s cleaned — no credentials stored.\n", path)
	return nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "mycnfm — encrypted MySQL credential switcher for ~/.my.cnf")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  mycnfm                     Launch the interactive TUI")
	fmt.Fprintln(os.Stderr, "  mycnfm clean               Wipe ~/.my.cnf (no credentials left on disk)")
	fmt.Fprintln(os.Stderr, "  mycnfm passwd [flags]      Change the vault's master password in place")
	fmt.Fprintln(os.Stderr, "  mycnfm export [flags]      Decrypt the vault to plaintext JSON")
	fmt.Fprintln(os.Stderr, "  mycnfm import [flags]      Encrypt a plaintext JSON export into a vault")
	fmt.Fprintln(os.Stderr, "  mycnfm --version           Print the version")
	fmt.Fprintln(os.Stderr, "  mycnfm --help              Show this message")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "passwd/export/import flags:")
	fmt.Fprintln(os.Stderr, "  -vault <path>   vault file (default: ~/.config/mycnfm/vault.enc)")
	fmt.Fprintln(os.Stderr, "  -json <path>    plaintext JSON file, export/import only (default: ./mycnfm-export.json)")
}

// readPassword prompts on stderr and reads a line without echoing it.
func readPassword(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return pw, err
}

// confirm asks a yes/no question on stderr, defaulting to "no".
func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt+" [y/N] ")
	var line string
	fmt.Fscanln(os.Stdin, &line)
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}
