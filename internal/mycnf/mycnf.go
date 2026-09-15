// Package mycnf renders a config.Record into ~/.my.cnf and wipes it back to
// a credential-free state on demand.
package mycnf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mycnfm/internal/config"
)

const (
	managedHeader = "# Managed by mycnfm — DO NOT EDIT MANUALLY, changes will be overwritten."
	cleanHeader   = "# mycnfm: no active profile. Run `mycnfm` to select one."

	// defaultPort is mysql's own default — omitted from the file since not
	// writing it has the exact same effect and keeps ~/.my.cnf minimal.
	defaultPort = 3306
)

// DefaultPath returns ~/.my.cnf
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".my.cnf"), nil
}

// Label returns a human-readable "folder/path/name" identifier for a record,
// used in the header comment and status messages.
func Label(store *config.Store, r config.Record) string {
	var parts []string
	for _, f := range store.FolderPath(r.FolderID) {
		parts = append(parts, f.Name)
	}
	parts = append(parts, r.Name)
	return strings.Join(parts, "/")
}

// Apply renders r's credentials into a [client] section and writes it to path
// atomically with 0600 permissions. defs is the current global option
// catalog — only entries r has switched on are written, using the catalog's
// current key/value (renaming or editing a definition takes effect on the
// next apply without touching the record).
func Apply(path string, label string, r config.Record, defs []config.OptionDef) error {
	var sb strings.Builder
	sb.WriteString(managedHeader + "\n")
	fmt.Fprintf(&sb, "# Active: %s  ·  applied %s\n\n", label, time.Now().Format("2006-01-02 15:04:05"))
	sb.WriteString("[client]\n")
	if r.Host != "" {
		fmt.Fprintf(&sb, "host     = %s\n", r.Host)
	}
	if r.Port != 0 && r.Port != defaultPort {
		fmt.Fprintf(&sb, "port     = %d\n", r.Port)
	}
	if r.Socket != "" {
		fmt.Fprintf(&sb, "socket   = %s\n", r.Socket)
	}
	if r.User != "" {
		fmt.Fprintf(&sb, "user     = %s\n", quoteIfNeeded(r.User))
	}
	if r.Password != "" {
		fmt.Fprintf(&sb, "password = %s\n", quotePassword(r.Password))
	}
	if r.Database != "" {
		fmt.Fprintf(&sb, "database = %s\n", quoteIfNeeded(r.Database))
	}
	for _, def := range defs {
		if !r.GlobalOptions[def.Key] {
			continue
		}
		writeOption(&sb, def.Key, def.Value)
	}
	for _, opt := range r.CustomOptions {
		if !opt.Enabled || opt.Key == "" {
			continue
		}
		writeOption(&sb, opt.Key, opt.Value)
	}

	return writeAtomic(path, sb.String())
}

func writeOption(sb *strings.Builder, key, value string) {
	if value == "" {
		fmt.Fprintf(sb, "%s\n", key)
	} else {
		fmt.Fprintf(sb, "%s = %s\n", key, quoteIfNeeded(value))
	}
}

// Clean overwrites path with a minimal, credential-free file.
func Clean(path string) error {
	content := cleanHeader + "\n"
	return writeAtomic(path, content)
}

// escapeQuoted backslash-escapes backslashes and double quotes so the value
// survives round-tripping through a double-quoted option-file string.
func escapeQuoted(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)
}

// quoteIfNeeded wraps a value in double quotes if it contains characters that
// would otherwise break the option-file grammar (spaces, #, quotes, backslashes).
func quoteIfNeeded(v string) string {
	if strings.ContainsAny(v, " \t#\"'\\") {
		return `"` + escapeQuoted(v) + `"`
	}
	return v
}

// quotePassword always wraps the password in double quotes — passwords
// routinely contain characters (spaces, #, $, etc.) that mysql's option-file
// parser would otherwise choke on or silently truncate.
func quotePassword(v string) string {
	return `"` + escapeQuoted(v) + `"`
}

func writeAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}
