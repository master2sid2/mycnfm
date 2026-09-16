package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	pass := []byte("correct horse battery staple")

	s, err := Open(path, pass)
	if err != nil {
		t.Fatalf("open new vault: %v", err)
	}

	env := NewFolder("")
	env.Name = "stage"
	if err := s.AddFolder(env); err != nil {
		t.Fatalf("add folder: %v", err)
	}

	r := NewRecord()
	r.FolderID = env.ID
	r.Group = "auth"
	r.Name = "auth-01"
	r.Port = 3307
	r.User = "root"
	r.Password = "s3cret"
	r.Tags = []string{"prod", "ro"}
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}
	if err := s.SetActiveID(r.ID); err != nil {
		t.Fatalf("set active: %v", err)
	}

	s2, err := Open(path, pass)
	if err != nil {
		t.Fatalf("reopen vault: %v", err)
	}

	recs := s2.RecordsInFolder(env.ID)
	if len(recs) != 1 || recs[0].Name != "auth-01" || recs[0].Password != "s3cret" {
		t.Fatalf("unexpected records after reload: %+v", recs)
	}
	if s2.ActiveID() != r.ID {
		t.Fatalf("active id lost across reload: got %q want %q", s2.ActiveID(), r.ID)
	}

	if _, err := Open(path, []byte("wrong password")); err == nil {
		t.Fatal("expected decrypt failure with wrong password")
	}
}

func TestDeleteFolderCascades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	s, err := Open(path, []byte("pw"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	root := NewFolder("")
	root.Name = "stage"
	_ = s.AddFolder(root)

	child := NewFolder(root.ID)
	child.Name = "sub"
	_ = s.AddFolder(child)

	r := NewRecord()
	r.FolderID = child.ID
	r.Name = "leaf"
	r.User = "u"
	_ = s.Add(r)
	_ = s.SetActiveID(r.ID)

	if err := s.DeleteFolder(root.ID); err != nil {
		t.Fatalf("delete folder: %v", err)
	}

	if _, ok := s.GetFolder(child.ID); ok {
		t.Fatal("child folder should have been deleted")
	}
	if _, ok := s.Get(r.ID); ok {
		t.Fatal("record under deleted folder should have been removed")
	}
	if s.ActiveID() != "" {
		t.Fatal("active id should be cleared when its record is deleted")
	}
}

func TestNewVaultSeedsCharsetOptionDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	s, err := Open(path, []byte("pw"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	defs := s.OptionDefs()
	if len(defs) != 1 || defs[0].Key != "default-character-set" || defs[0].Value != "utf8mb3" {
		t.Fatalf("expected seeded default-character-set=utf8mb3 catalog entry, got %+v", defs)
	}

	r := NewRecord()
	r.Name = "x"
	r.User = "u"
	if r.GlobalOptions[defs[0].Key] {
		t.Fatal("a new record must not have catalog options pre-enabled")
	}
}

func TestOptionDefRenameMigratesRecordState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	s, _ := Open(path, []byte("pw"))

	if err := s.AddOptionDef(OptionDef{Key: "compress"}); err != nil {
		t.Fatalf("add option def: %v", err)
	}

	r := NewRecord()
	r.Name = "x"
	r.User = "u"
	r.GlobalOptions = map[string]bool{"compress": true}
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}

	if err := s.UpdateOptionDef("compress", OptionDef{Key: "compression"}); err != nil {
		t.Fatalf("rename option def: %v", err)
	}

	got, _ := s.Get(r.ID)
	if got.GlobalOptions["compress"] {
		t.Error("old key should no longer be enabled after rename")
	}
	if !got.GlobalOptions["compression"] {
		t.Error("enabled state should have migrated to the new key")
	}
}

func TestDeleteOptionDefStripsRecordReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	s, _ := Open(path, []byte("pw"))

	if err := s.AddOptionDef(OptionDef{Key: "compress"}); err != nil {
		t.Fatalf("add option def: %v", err)
	}
	r := NewRecord()
	r.Name = "x"
	r.User = "u"
	r.GlobalOptions = map[string]bool{"compress": true}
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}

	if err := s.DeleteOptionDef("compress"); err != nil {
		t.Fatalf("delete option def: %v", err)
	}

	for _, d := range s.OptionDefs() {
		if d.Key == "compress" {
			t.Fatal("catalog entry should be gone")
		}
	}
	got, _ := s.Get(r.ID)
	if got.GlobalOptions["compress"] {
		t.Error("record should no longer reference the deleted option")
	}
}

func TestExportImportRoundTripsWithNewPassword(t *testing.T) {
	origPath := filepath.Join(t.TempDir(), "vault.enc")
	oldPass := []byte("old password")
	s, err := Open(origPath, oldPass)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	env := NewFolder("")
	env.Name = "stage"
	_ = s.AddFolder(env)

	r := NewRecord()
	r.FolderID = env.ID
	r.Name = "auth-01"
	r.User = "root"
	r.Password = "s3cret"
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}
	_ = s.AddOptionDef(OptionDef{Key: "compress"})

	data, err := s.ExportJSON()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(string(data), "s3cret") {
		t.Fatal("expected exported JSON to contain the plaintext password")
	}

	newPath := filepath.Join(t.TempDir(), "vault2.enc")
	newPass := []byte("brand new password")
	imported, err := ImportJSON(newPath, newPass, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if err := imported.Save(); err != nil {
		t.Fatalf("save imported vault: %v", err)
	}

	if _, err := Open(newPath, oldPass); err == nil {
		t.Fatal("expected the old password to no longer work on the imported vault")
	}

	reopened, err := Open(newPath, newPass)
	if err != nil {
		t.Fatalf("reopen with new password: %v", err)
	}
	recs := reopened.RecordsInFolder(env.ID)
	if len(recs) != 1 || recs[0].Name != "auth-01" || recs[0].Password != "s3cret" {
		t.Fatalf("unexpected records after import: %+v", recs)
	}
	defs := reopened.OptionDefs()
	found := false
	for _, d := range defs {
		if d.Key == "compress" {
			found = true
		}
	}
	if !found {
		t.Fatal("option catalog was not preserved across export/import")
	}
}

func TestRekeyChangesPasswordInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	oldPass := []byte("old password")
	s, err := Open(path, oldPass)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	r := NewRecord()
	r.Name = "auth-01"
	r.User = "root"
	r.Password = "s3cret"
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}

	newPass := []byte("brand new password")
	if err := s.Rekey(newPass); err != nil {
		t.Fatalf("rekey: %v", err)
	}

	if _, err := Open(path, oldPass); err == nil {
		t.Fatal("expected the old password to no longer work after rekey")
	}

	reopened, err := Open(path, newPass)
	if err != nil {
		t.Fatalf("reopen with new password: %v", err)
	}
	recs := reopened.RecordsUnder("")
	if got := len(recs); got != 1 {
		t.Fatalf("expected 1 record to survive rekey, got %d", got)
	}
}

func TestMoveFolderRejectsCycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	s, _ := Open(path, []byte("pw"))

	parent := NewFolder("")
	parent.Name = "parent"
	_ = s.AddFolder(parent)

	child := NewFolder(parent.ID)
	child.Name = "child"
	_ = s.AddFolder(child)

	if err := s.MoveFolder(parent.ID, child.ID); err == nil {
		t.Fatal("expected error moving a folder into its own descendant")
	}
}
