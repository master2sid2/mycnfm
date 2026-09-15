package mycnf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycnfm/internal/config"
)

func TestApplyWritesClientSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{
		Name:     "auth-01",
		Host:     "10.0.0.5",
		Port:     3306,
		User:     "root",
		Password: "s3cret",
		Database: "appdb",
	}

	if err := Apply(path, "stage/auth-01", r); err != nil {
		t.Fatalf("apply: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	for _, want := range []string{
		"[client]",
		"host     = 10.0.0.5",
		"port     = 3306",
		"user     = root",
		"password = s3cret",
		"database = appdb",
		"stage/auth-01",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected content to contain %q, got:\n%s", want, content)
		}
	}
}

func TestCleanRemovesCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{Name: "x", Host: "h", Port: 1, User: "u", Password: "secret-value"}
	if err := Apply(path, "x", r); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if err := Clean(path); err != nil {
		t.Fatalf("clean: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "secret-value") {
		t.Fatal("clean left the old password in the file")
	}
	if strings.Contains(string(data), "[client]") {
		t.Fatal("clean should not leave a [client] section behind")
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	cases := map[string]string{
		"simple":     "simple",
		"has space":  `"has space"`,
		`with"quote`: `"with\"quote"`,
	}
	for in, want := range cases {
		if got := quoteIfNeeded(in); got != want {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", in, got, want)
		}
	}
}
