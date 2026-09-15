package cmd

import (
	"context"
	"fmt"
	"os"

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

	fmt.Fprint(os.Stderr, "Master password: ")
	rawPass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
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
	fmt.Fprintln(os.Stderr, "  mycnfm            Launch the interactive TUI")
	fmt.Fprintln(os.Stderr, "  mycnfm clean      Wipe ~/.my.cnf (no credentials left on disk)")
	fmt.Fprintln(os.Stderr, "  mycnfm --version  Print the version")
	fmt.Fprintln(os.Stderr, "  mycnfm --help     Show this message")
}
