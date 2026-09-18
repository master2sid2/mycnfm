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
		Port:     3307,
		Socket:   "/var/run/mysqld/mysqld.sock",
		User:     "root",
		Password: "s3cret",
		Database: "appdb",
	}

	if _, err := Apply(path, "stage/auth-01", r, nil); err != nil {
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
		"port     = 3307",
		"socket   = /var/run/mysqld/mysqld.sock",
		"user     = root",
		`password = "s3cret"`,
		"database = appdb",
		"stage/auth-01",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected content to contain %q, got:\n%s", want, content)
		}
	}
}

func TestApplyOmitsBlankConnectionFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{Name: "just-creds", User: "root", Password: "s3cret"}

	if _, err := Apply(path, "just-creds", r, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	for _, unwanted := range []string{"port", "socket", "database"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("expected no %q line when field is blank, got:\n%s", unwanted, content)
		}
	}
	if !strings.Contains(content, "user     = root") || !strings.Contains(content, `password = "s3cret"`) {
		t.Errorf("expected user/password lines, got:\n%s", content)
	}
}

func TestApplyOmitsDefaultPortButKeepsCustomPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")

	def := config.Record{Name: "x", Port: 3306, User: "u"}
	if _, err := Apply(path, "x", def, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "port") {
		t.Errorf("default port 3306 should be omitted, got:\n%s", data)
	}

	custom := config.Record{Name: "x", Port: 3307, User: "u"}
	if _, err := Apply(path, "x", custom, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "port     = 3307\n") {
		t.Errorf("non-default port should be written, got:\n%s", data)
	}
}

func TestApplyQuotesSpecialCharacterPasswords(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{
		Name: "x", User: "u",
		Password: `p@ss "w"ord \with# stuff`,
	}
	if _, err := Apply(path, "x", r, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	want := `password = "p@ss \"w\"ord \\with# stuff"` + "\n"
	if !strings.Contains(content, want) {
		t.Fatalf("expected escaped/quoted password line %q, got:\n%s", want, content)
	}
}

func TestApplyWritesEnabledGlobalOptionsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	defs := []config.OptionDef{
		{Key: "default-character-set", Value: "utf8mb3"},
		{Key: "compress"},
		{Key: "ssl-mode", Value: "DISABLED"},
	}
	r := config.Record{
		Name: "x", User: "u",
		GlobalOptions: map[string]bool{
			"default-character-set": true,
			"compress":              true,
			"ssl-mode":              false,
		},
	}
	if _, err := Apply(path, "x", r, defs); err != nil {
		t.Fatalf("apply: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "default-character-set = utf8mb3\n") {
		t.Errorf("missing enabled key=value option, got:\n%s", content)
	}
	if !strings.Contains(content, "compress\n") {
		t.Errorf("missing enabled bare-flag option, got:\n%s", content)
	}
	if strings.Contains(content, "ssl-mode") {
		t.Errorf("disabled option should not be written, got:\n%s", content)
	}
}

func TestApplyWritesEnabledCustomOptionsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{
		Name: "x", User: "u",
		CustomOptions: []config.Option{
			{Key: "connect-timeout", Value: "5", Enabled: true},
			{Key: "reconnect", Enabled: false},
		},
	}
	if _, err := Apply(path, "x", r, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "connect-timeout = 5\n") {
		t.Errorf("missing enabled custom option, got:\n%s", content)
	}
	if strings.Contains(content, "reconnect") {
		t.Errorf("disabled custom option should not be written, got:\n%s", content)
	}
}

func TestCleanRemovesCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{Name: "x", User: "u", Password: "secret-value"}
	if _, err := Apply(path, "x", r, nil); err != nil {
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

func TestApplyHashMatchesCurrentHashUntilFileChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{Name: "x", User: "u", Password: "s3cret"}

	hash, err := Apply(path, "x", r, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if hash == "" {
		t.Fatal("expected a non-empty hash")
	}

	current, ok, err := CurrentHash(path)
	if err != nil {
		t.Fatalf("current hash: %v", err)
	}
	if !ok {
		t.Fatal("expected file to exist")
	}
	if current != hash {
		t.Fatalf("hash right after apply should match: got %q want %q", current, hash)
	}

	if err := os.WriteFile(path, []byte("tampered\n"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	current, ok, err = CurrentHash(path)
	if err != nil {
		t.Fatalf("current hash: %v", err)
	}
	if !ok {
		t.Fatal("expected file to still exist")
	}
	if current == hash {
		t.Fatal("expected hash to change after the file was edited externally")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	_, ok, err = CurrentHash(path)
	if err != nil {
		t.Fatalf("current hash after removal: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a missing file")
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
