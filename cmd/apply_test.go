package cmd

import (
	"strings"
	"testing"

	"mycnfm/internal/config"
)

func newTestStore(t *testing.T) *config.Store {
	t.Helper()
	s, err := config.Open(t.TempDir()+"/vault.enc", []byte("pw"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func TestFindRecordByFullPath(t *testing.T) {
	s := newTestStore(t)
	env := config.NewFolder("")
	env.Name = "stage"
	if err := s.AddFolder(env); err != nil {
		t.Fatalf("add folder: %v", err)
	}
	r := config.NewRecord()
	r.FolderID = env.ID
	r.Name = "auth-01"
	r.User = "root"
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}

	got, err := findRecord(s, "stage/auth-01")
	if err != nil {
		t.Fatalf("findRecord: %v", err)
	}
	if got.ID != r.ID {
		t.Fatalf("expected %s, got %s", r.ID, got.ID)
	}
}

func TestFindRecordByUniqueName(t *testing.T) {
	s := newTestStore(t)
	env := config.NewFolder("")
	env.Name = "stage"
	_ = s.AddFolder(env)
	r := config.NewRecord()
	r.FolderID = env.ID
	r.Name = "auth-01"
	r.User = "root"
	if err := s.Add(r); err != nil {
		t.Fatalf("add record: %v", err)
	}

	got, err := findRecord(s, "auth-01")
	if err != nil {
		t.Fatalf("findRecord: %v", err)
	}
	if got.ID != r.ID {
		t.Fatalf("expected %s, got %s", r.ID, got.ID)
	}
}

func TestFindRecordAmbiguousNameRequiresFullPath(t *testing.T) {
	s := newTestStore(t)
	stage := config.NewFolder("")
	stage.Name = "stage"
	_ = s.AddFolder(stage)
	prod := config.NewFolder("")
	prod.Name = "prod"
	_ = s.AddFolder(prod)

	r1 := config.NewRecord()
	r1.FolderID = stage.ID
	r1.Name = "auth-01"
	r1.User = "u"
	_ = s.Add(r1)

	r2 := config.NewRecord()
	r2.FolderID = prod.ID
	r2.Name = "auth-01"
	r2.User = "u"
	_ = s.Add(r2)

	_, err := findRecord(s, "auth-01")
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	if !strings.Contains(err.Error(), "stage/auth-01") || !strings.Contains(err.Error(), "prod/auth-01") {
		t.Fatalf("expected both candidate paths in the error, got: %v", err)
	}

	// The full path still resolves unambiguously.
	got, err := findRecord(s, "prod/auth-01")
	if err != nil {
		t.Fatalf("findRecord by full path: %v", err)
	}
	if got.ID != r2.ID {
		t.Fatalf("expected %s, got %s", r2.ID, got.ID)
	}
}

func TestFindRecordNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := findRecord(s, "nope"); err == nil {
		t.Fatal("expected a not-found error")
	}
}
