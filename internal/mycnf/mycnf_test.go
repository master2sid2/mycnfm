package mycnf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycnfm/internal/config"
)

func TestRenderMatchesApplyWithoutTouchingDisk(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.my.cnf"
	r := config.Record{Name: "x", User: "root", Password: "s3cret"}

	rendered := Render("x", r, nil)
	if !strings.Contains(rendered, "user     = root") {
		t.Fatalf("expected rendered content to contain the user line, got:\n%s", rendered)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Render must not write to disk")
	}

	hash, err := Apply(path, "x", r, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read applied file: %v", err)
	}
	if Hash(string(data)) != hash {
		t.Fatal("Apply's returned hash should match the file it wrote")
	}
}

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

func TestRenderPutsNonClientOptionsInTheirOwnSection(t *testing.T) {
	defs := []config.OptionDef{
		{Key: "default-character-set", Value: "utf8mb3"}, // no Section -> [client]
		{Key: "quick", Section: "mysqldump"},
	}
	r := config.Record{
		Name: "x", User: "u",
		GlobalOptions: map[string]bool{
			"default-character-set": true,
			"[mysqldump]quick":      true, // sectioned options are tracked by OptionDef.ID()
		},
		CustomOptions: []config.Option{
			{Key: "column-statistics", Enabled: true, Section: "mysqldump"},
			{Key: "compress", Enabled: true}, // no Section -> [client]
		},
	}
	content := Render("x", r, defs)

	clientIdx := strings.Index(content, "[client]")
	dumpIdx := strings.Index(content, "[mysqldump]")
	if clientIdx == -1 || dumpIdx == -1 {
		t.Fatalf("expected both [client] and [mysqldump] sections, got:\n%s", content)
	}
	if clientIdx > dumpIdx {
		t.Fatalf("expected [client] before [mysqldump], got:\n%s", content)
	}

	clientBlock := content[clientIdx:dumpIdx]
	dumpBlock := content[dumpIdx:]

	if !strings.Contains(clientBlock, "user     = u") {
		t.Errorf("expected user in [client], got:\n%s", clientBlock)
	}
	if !strings.Contains(clientBlock, "compress") {
		t.Errorf("expected compress (no Section) in [client], got:\n%s", clientBlock)
	}
	if strings.Contains(clientBlock, "quick") || strings.Contains(clientBlock, "column-statistics") {
		t.Errorf("mysqldump-scoped options must not leak into [client], got:\n%s", clientBlock)
	}

	if !strings.Contains(dumpBlock, "quick") || !strings.Contains(dumpBlock, "column-statistics") {
		t.Errorf("expected quick and column-statistics in [mysqldump], got:\n%s", dumpBlock)
	}
	if strings.Contains(dumpBlock, "user") || strings.Contains(dumpBlock, "compress") {
		t.Errorf("client-scoped entries must not leak into [mysqldump], got:\n%s", dumpBlock)
	}
}

func TestRenderOmitsSectionsWithNothingEnabled(t *testing.T) {
	defs := []config.OptionDef{
		{Key: "quick", Section: "mysqldump"},
	}
	r := config.Record{Name: "x", User: "u"} // quick not enabled
	content := Render("x", r, defs)
	if strings.Contains(content, "[mysqldump]") {
		t.Fatalf("expected no [mysqldump] section when nothing targets it, got:\n%s", content)
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
		"two\nlines": `"two\nlines"`,
		"cr\rhere":   `"cr\rhere"`,
	}
	for in, want := range cases {
		if got := quoteIfNeeded(in); got != want {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderSameKeyInBothSectionsIndependently(t *testing.T) {
	defs := []config.OptionDef{
		{Key: "default-character-set", Value: "utf8mb3"},
		{Key: "default-character-set", Value: "utf8mb4", Section: "mysqldump"},
	}
	both := config.Record{Name: "x", User: "u", GlobalOptions: map[string]bool{
		"default-character-set":            true,
		"[mysqldump]default-character-set": true,
	}}
	out := Render("x", both, defs)
	if !strings.Contains(out, "[client]\nuser     = u\ndefault-character-set = utf8mb3\n") ||
		!strings.Contains(out, "[mysqldump]\ndefault-character-set = utf8mb4\n") {
		t.Fatalf("expected each section to get its own value, got:\n%s", out)
	}

	onlyDump := config.Record{Name: "x", User: "u", GlobalOptions: map[string]bool{"[mysqldump]default-character-set": true}}
	out = Render("x", onlyDump, defs)
	if strings.Contains(out, "utf8mb3") || !strings.Contains(out, "utf8mb4") {
		t.Fatalf("toggling one section must not enable the other, got:\n%s", out)
	}
}

func reconcileFixture(t *testing.T, expires time.Time) (*config.Store, string, string) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "vault.enc"), []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	r := config.NewRecord()
	r.Name, r.User, r.Password = "x", "u", "s3cret"
	if err := store.Add(r); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".my.cnf")
	hash, err := Apply(path, "x", r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetActive(r.ID, hash, expires); err != nil {
		t.Fatal(err)
	}
	return store, path, hash
}

func TestReconcileWipesAnExpiredProfileStillInPlace(t *testing.T) {
	store, path, _ := reconcileFixture(t, time.Now().Add(-time.Minute))
	out, err := Reconcile(store, path, time.Now())
	if err != nil || out != Expired {
		t.Fatalf("expected Expired, got %v, %v", out, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "s3cret") {
		t.Fatalf("expired profile must be wiped, got:\n%s", data)
	}
	if store.ActiveID() != "" || !store.ActiveExpires().IsZero() {
		t.Fatal("expiry must clear the active profile and its deadline")
	}
}

func TestReconcileLeavesUnexpiredAndEditedFilesAlone(t *testing.T) {
	store, path, _ := reconcileFixture(t, time.Now().Add(time.Hour))
	if out, _ := Reconcile(store, path, time.Now()); out != Unchanged {
		t.Fatalf("a future deadline must change nothing, got %v", out)
	}

	// Past the deadline, but the user hand-edited the file: not ours to wipe.
	if err := os.WriteFile(path, []byte("my own config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _ := Reconcile(store, path, time.Now().Add(2*time.Hour)); out != Unchanged {
		t.Fatalf("an edited file must never be wiped, got %v", out)
	}
	if data, _ := os.ReadFile(path); string(data) != "my own config\n" {
		t.Fatalf("edited file was modified: %q", data)
	}
}

func TestReconcileDropsTheActiveProfileWhenFileIsAlreadyWiped(t *testing.T) {
	store, path, _ := reconcileFixture(t, time.Now().Add(time.Hour))
	if err := Clean(path); err != nil { // e.g. the watcher, or `mycnfm clean`
		t.Fatal(err)
	}
	out, err := Reconcile(store, path, time.Now())
	if err != nil || out != FoundCleaned {
		t.Fatalf("expected FoundCleaned, got %v, %v", out, err)
	}
	if store.ActiveID() != "" {
		t.Fatal("a wiped file must clear the stale active profile")
	}
}

func TestReconcileWithNoActiveProfileDoesNothing(t *testing.T) {
	store, _ := config.Open(filepath.Join(t.TempDir(), "vault.enc"), []byte("pw"))
	if out, err := Reconcile(store, "/does/not/matter", time.Now()); out != Unchanged || err != nil {
		t.Fatalf("got %v, %v", out, err)
	}
}

func TestApplyKeepsMultiLineValuesOnOneLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	r := config.Record{Name: "x", User: "u", Password: "line1\nhost = evil", Socket: "/run/my sql/mysqld.sock"}
	if _, err := Apply(path, "x", r, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	for _, want := range []string{
		`password = "line1\nhost = evil"` + "\n",
		`socket   = "/run/my sql/mysqld.sock"` + "\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "\nhost = evil") {
		t.Errorf("a newline in the password produced an option line of its own:\n%s", content)
	}
}

func TestApplyTightensPermissionsOfAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".my.cnf")
	if err := os.WriteFile(path, []byte("[client]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(path, "x", config.Record{Name: "x", User: "u", Password: "p"}, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("~/.my.cnf mode = %o, want 600", mode)
	}
}
