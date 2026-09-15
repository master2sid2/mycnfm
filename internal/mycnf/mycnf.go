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
// atomically with 0600 permissions.
func Apply(path string, label string, r config.Record) error {
	var sb strings.Builder
	sb.WriteString(managedHeader + "\n")
	fmt.Fprintf(&sb, "# Active: %s  ·  applied %s\n\n", label, time.Now().Format("2006-01-02 15:04:05"))
	sb.WriteString("[client]\n")
	if r.Host != "" {
		fmt.Fprintf(&sb, "host     = %s\n", r.Host)
	}
	if r.Port != 0 {
		fmt.Fprintf(&sb, "port     = %d\n", r.Port)
	}
	if r.Socket != "" {
		fmt.Fprintf(&sb, "socket   = %s\n", r.Socket)
	}
	if r.User != "" {
		fmt.Fprintf(&sb, "user     = %s\n", r.User)
	}
	if r.Password != "" {
		fmt.Fprintf(&sb, "password = %s\n", quoteIfNeeded(r.Password))
	}
	if r.Database != "" {
		fmt.Fprintf(&sb, "database = %s\n", r.Database)
	}

	return writeAtomic(path, sb.String())
}

// Clean overwrites path with a minimal, credential-free file.
func Clean(path string) error {
	content := cleanHeader + "\n"
	return writeAtomic(path, content)
}

// quoteIfNeeded wraps a value in double quotes if it contains characters that
// would otherwise break the option-file grammar (spaces, #, quotes).
func quoteIfNeeded(v string) string {
	if strings.ContainsAny(v, " \t#\"'") {
		escaped := strings.ReplaceAll(v, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return v
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
