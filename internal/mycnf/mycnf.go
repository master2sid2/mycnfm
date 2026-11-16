// Package mycnf renders a config.Record into ~/.my.cnf and wipes it back to
// a credential-free state on demand.
package mycnf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mycnfm/internal/config"
	"mycnfm/internal/fsutil"
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

// optLine is one key/value pair queued for a specific option-file section.
type optLine struct{ key, value string }

// Render builds the option-file content for r exactly as Apply would write
// it, without touching disk — used both by Apply and by callers that just
// want to preview or print the result (e.g. `mycnfm apply -o -`). defs is
// the current global option catalog — only entries r has switched on are
// included, using the catalog's current key/value/section.
//
// Credentials always go in [client], shared by every client program. Any
// option whose Section is something else (e.g. "mysqldump") is instead
// collected into its own [section] block, emitted after [client] — a
// program-specific group is read after [client] and overrides it for the
// same key, and some settings (default-character-set for mysqldump, notably)
// aren't reliably honored from [client] alone across MySQL/MariaDB versions.
func Render(label string, r config.Record, defs []config.OptionDef) string {
	var sb strings.Builder
	sb.WriteString(managedHeader + "\n")
	fmt.Fprintf(&sb, "# Active: %s  ·  applied %s\n\n", label, time.Now().Format("2006-01-02 15:04:05"))

	sections := make(map[string][]optLine)
	for _, def := range defs {
		if !r.GlobalOptions[def.ID()] {
			continue
		}
		sec := def.SectionOrDefault()
		sections[sec] = append(sections[sec], optLine{def.Key, def.Value})
	}
	for _, opt := range r.CustomOptions {
		if !opt.Enabled || opt.Key == "" {
			continue
		}
		sec := opt.SectionOrDefault()
		sections[sec] = append(sections[sec], optLine{opt.Key, opt.Value})
	}

	fmt.Fprintf(&sb, "[%s]\n", config.DefaultSection)
	if r.Port != 0 && r.Port != defaultPort {
		fmt.Fprintf(&sb, "port     = %d\n", r.Port)
	}
	if r.Socket != "" {
		fmt.Fprintf(&sb, "socket   = %s\n", quoteIfNeeded(r.Socket))
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
	for _, l := range sections[config.DefaultSection] {
		writeOption(&sb, l.key, l.value)
	}

	var others []string
	for name := range sections {
		if name != config.DefaultSection {
			others = append(others, name)
		}
	}
	slices.Sort(others)
	for _, name := range others {
		fmt.Fprintf(&sb, "\n[%s]\n", name)
		for _, l := range sections[name] {
			writeOption(&sb, l.key, l.value)
		}
	}

	return sb.String()
}

// Apply renders r's credentials into a [client] section and writes it to path
// atomically with 0600 permissions. It returns the hex-encoded sha256 of the
// exact bytes written, for later drift detection via CurrentHash.
func Apply(path string, label string, r config.Record, defs []config.OptionDef) (string, error) {
	content := Render(label, r, defs)
	if err := writeAtomic(path, content); err != nil {
		return "", err
	}
	return Hash(content), nil
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

// Hash returns the hex-encoded sha256 of content.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// CurrentHash returns Hash of path's current contents, for comparison
// against a previously recorded Apply hash to detect drift — the file was
// edited or removed outside mycnfm since it was last applied. ok is false
// if the file doesn't exist.
func CurrentHash(path string) (hash string, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return Hash(string(data)), true, nil
}

// escapeQuoted backslash-escapes backslashes, double quotes and line breaks
// so the value survives round-tripping through a double-quoted option-file
// string. An unescaped newline would end the line and turn the rest of the
// value into an option line of its own.
func escapeQuoted(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(v)
}

// quoteIfNeeded wraps a value in double quotes if it contains characters that
// would otherwise break the option-file grammar (spaces, #, quotes,
// backslashes, line breaks).
func quoteIfNeeded(v string) string {
	if strings.ContainsAny(v, " \t\n\r#\"'\\") {
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
	if err := fsutil.WriteFileAtomic(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Outcome describes what Reconcile found or did.
type Outcome int

const (
	// Unchanged: nothing to report.
	Unchanged Outcome = iota
	// Expired: the auto-wipe deadline had passed with the profile's file still
	// in place, so Reconcile wiped it (the backstop for a watcher that didn't
	// survive a reboot, sleep or logout).
	Expired
	// FoundCleaned: ~/.my.cnf is already the wiped placeholder — by the
	// watcher or `mycnfm clean` — so the stale active profile was dropped.
	FoundCleaned
)

// CleanHash is the hash of the content Clean writes, used to recognise a
// wiped file.
func CleanHash() string { return Hash(cleanHeader + "\n") }

// Reconcile brings the vault's notion of the active profile in line with
// reality. If the file is already the wiped placeholder, the active profile is
// cleared; if the profile's deadline has passed while the file still holds
// exactly what was applied, the file is wiped and the profile cleared. A
// missing or hand-edited file is left alone (that's drift, reported
// elsewhere). Nothing happens when no profile is active.
func Reconcile(store *config.Store, path string, now time.Time) (Outcome, error) {
	if store.ActiveID() == "" {
		return Unchanged, nil
	}
	cur, ok, err := CurrentHash(path)
	if err != nil || !ok {
		return Unchanged, err
	}
	if cur == CleanHash() {
		return FoundCleaned, store.ClearActive()
	}
	if exp := store.ActiveExpires(); !exp.IsZero() && !now.Before(exp) && cur == store.ActiveHash() {
		if err := Clean(path); err != nil {
			return Unchanged, err
		}
		return Expired, store.ClearActive()
	}
	return Unchanged, nil
}
