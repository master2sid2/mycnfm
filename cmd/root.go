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
		case "apply":
			return runApply(os.Args[2:])
		case "list":
			return runList(os.Args[2:])
		case "lock":
			return runLock(os.Args[2:])
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

	store, err := openVault(cfgPath, true)
	if err != nil {
		return err
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
	fmt.Fprintln(os.Stderr, "  mycnfm apply <path>        Apply one record non-interactively")
	fmt.Fprintln(os.Stderr, "  mycnfm list [flags]        Print every record's path, one per line")
	fmt.Fprintln(os.Stderr, "  mycnfm clean               Wipe ~/.my.cnf (no credentials left on disk)")
	fmt.Fprintln(os.Stderr, "  mycnfm lock                Forget the unlock cached for this login session")
	fmt.Fprintln(os.Stderr, "  mycnfm passwd [flags]      Change the vault's master password in place")
	fmt.Fprintln(os.Stderr, "  mycnfm export [flags]      Decrypt the vault to plaintext JSON")
	fmt.Fprintln(os.Stderr, "  mycnfm import [flags]      Encrypt a plaintext JSON export into a vault")
	fmt.Fprintln(os.Stderr, "  mycnfm --version           Print the version")
	fmt.Fprintln(os.Stderr, "  mycnfm --help              Show this message")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "apply/list/passwd/export/import flags:")
	fmt.Fprintln(os.Stderr, "  -vault <path>   vault file (default: ~/.config/mycnfm/vault.enc)")
	fmt.Fprintln(os.Stderr, "  -json <path>    plaintext JSON file, export/import only (default: ./mycnfm-export.json)")
	fmt.Fprintln(os.Stderr, "  -o <path>|-     apply only: output path (default: ~/.my.cnf); - for stdout")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Unlocking: the master password is asked once per login session (Linux); the")
	fmt.Fprintln(os.Stderr, "derived key is then held in the kernel session keyring, so later runs don't ask.")
	fmt.Fprintln(os.Stderr, "Logging out or `mycnfm lock` forgets it. `passwd` always asks for the password.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Unattended use (cron/CI, where there is no login session):")
	fmt.Fprintln(os.Stderr, "  MYCNFM_PASSWORD       master password, set directly")
	fmt.Fprintln(os.Stderr, "  MYCNFM_PASSWORD_FILE  path to a file containing it (safer: never sits in the environment)")
}

// readPassword prompts on stderr and reads a line without echoing it.
func readPassword(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return pw, err
}

// reorderFlagsFirst moves any of the named single-value flags (and the
// argument immediately following each) to the front of args, leaving
// everything else — including a positional argument — after them. The
// stdlib flag package otherwise stops recognizing flags once it hits the
// first non-flag argument, which would make `apply <path> -o -` silently
// swallow "-o" and "-" as extra positional args instead of parsing "-o".
func reorderFlagsFirst(args []string, names ...string) []string {
	isFlag := make(map[string]bool, len(names))
	for _, n := range names {
		isFlag["-"+n] = true
		isFlag["--"+n] = true
	}

	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, _, hasEq := strings.Cut(a, "=")
		if isFlag[name] {
			flags = append(flags, a)
			if !hasEq && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, rest...)
}

// confirm asks a yes/no question on stderr, defaulting to "no".
func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt+" [y/N] ")
	var line string
	fmt.Fscanln(os.Stdin, &line)
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}
