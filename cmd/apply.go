package cmd

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"mycnfm/internal/config"
	"mycnfm/internal/mycnf"
)

// runApply resolves one record non-interactively (by full path, or by bare
// name if that's unique) and either writes it to ~/.my.cnf — exactly what
// pressing enter in the TUI does, including drift tracking — or, with
// -o -, prints the rendered [client] section to stdout without touching the
// vault or the real ~/.my.cnf at all. Meant for scripting: combine with
// MYCNFM_PASSWORD/MYCNFM_PASSWORD_FILE to run unattended.
func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	vaultFlag := fs.String("vault", "", "vault file (default: ~/.config/mycnfm/vault.enc)")
	outFlag := fs.String("o", "", "output path (default: ~/.my.cnf); use -o - for stdout")
	// The stdlib flag package stops parsing at the first non-flag argument,
	// so `apply <path> -o -` would otherwise treat "-o" and "-" as extra
	// positional args. Move flags (and their values) to the front first so
	// the path can go either before or after them.
	if err := fs.Parse(reorderFlagsFirst(args, "vault", "o")); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mycnfm apply <path-or-name> [-vault <path>] [-o <path>|-]")
	}
	query := fs.Arg(0)

	vaultPath := *vaultFlag
	if vaultPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve vault path: %w", err)
		}
		vaultPath = p
	}

	pass, err := resolveUnlockPassword("Master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	store, err := config.Open(vaultPath, pass)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}

	r, err := findRecord(store, query)
	if err != nil {
		return err
	}
	label := mycnf.Label(store, r)
	defs := store.OptionDefs()

	if *outFlag == "-" {
		fmt.Print(mycnf.Render(label, r, defs))
		return nil
	}

	outPath := *outFlag
	trackActive := outPath == ""
	if outPath == "" {
		p, err := mycnf.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve ~/.my.cnf: %w", err)
		}
		outPath = p
	}

	hash, err := mycnf.Apply(outPath, label, r, defs)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if trackActive {
		if err := store.SetActive(r.ID, hash); err != nil {
			return fmt.Errorf("applied but failed to save state: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Applied %s to %s\n", label, outPath)
	return nil
}

// runList prints every record's full path (env/.../name — what `apply`
// accepts and what mycnf.Label renders), one per line, plain text and
// sorted, for scripting.
func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
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

	pass, err := resolveUnlockPassword("Master password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	store, err := config.Open(vaultPath, pass)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}

	var paths []string
	for _, r := range store.RecordsUnder("") {
		paths = append(paths, mycnf.Label(store, r))
	}
	slices.Sort(paths)
	for _, p := range paths {
		fmt.Println(p)
	}
	return nil
}

// findRecord resolves query to exactly one record: first by exact full path
// (as mycnf.Label renders it), falling back to a bare Name match if that's
// unique across the whole vault.
func findRecord(store *config.Store, query string) (config.Record, error) {
	all := store.RecordsUnder("")

	for _, r := range all {
		if mycnf.Label(store, r) == query {
			return r, nil
		}
	}

	var matches []config.Record
	for _, r := range all {
		if r.Name == query {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return config.Record{}, fmt.Errorf("no record matches %q (run `mycnfm list` to see valid paths)", query)
	case 1:
		return matches[0], nil
	default:
		var paths []string
		for _, r := range matches {
			paths = append(paths, mycnf.Label(store, r))
		}
		slices.Sort(paths)
		return config.Record{}, fmt.Errorf("%q matches multiple records — use a full path:\n  %s", query, strings.Join(paths, "\n  "))
	}
}
