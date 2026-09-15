package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Folder is a named node in the tree used to organise records. A root-level
// folder (ParentID == "") represents an ENV (e.g. "stage", "prod"); folders
// can be nested arbitrarily deep beneath that.
type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parent_id,omitempty"` // "" = root (ENV level)
	CreatedAt time.Time `json:"created_at"`
}

// NewFolder returns a Folder with a generated ID placed under parentID.
func NewFolder(parentID string) Folder {
	return Folder{
		ID:        uuid.New().String(),
		ParentID:  parentID,
		CreatedAt: time.Now(),
	}
}

// Record holds everything needed to populate ~/.my.cnf for one database target.
type Record struct {
	ID        string    `json:"id"`
	FolderID  string    `json:"folder_id,omitempty"` // "" = root level
	Group     string    `json:"group,omitempty"`     // free-text display cluster, e.g. "auth", "mail"
	Name      string    `json:"name"`
	Host      string    `json:"host,omitempty"`
	Port      int       `json:"port,omitempty"`
	Socket    string    `json:"socket,omitempty"`
	User      string    `json:"user"`
	Password  string    `json:"password,omitempty"` // stored encrypted at rest
	Database  string    `json:"database,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewRecord returns a Record with generated ID and timestamps.
func NewRecord() Record {
	now := time.Now()
	return Record{
		ID:        uuid.New().String(),
		Port:      3306,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Clone returns a deep copy of the record.
func (r Record) Clone() Record {
	cp := r
	cp.Tags = make([]string, len(r.Tags))
	copy(cp.Tags, r.Tags)
	return cp
}

// vault is the plaintext structure stored (encrypted) on disk.
type vault struct {
	Version  int      `json:"version"`
	Folders  []Folder `json:"folders,omitempty"`
	Records  []Record `json:"records"`
	ActiveID string   `json:"active_id,omitempty"` // last record ID applied to ~/.my.cnf
}

// Store is the in-memory config, backed by an encrypted file.
type Store struct {
	path  string
	key   []byte // 32-byte AES key derived from master password
	salt  []byte // Argon2 salt, persisted in file header
	vault vault
}

// DefaultPath returns ~/.config/mycnfm/vault.enc
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "mycnfm")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "vault.enc"), nil
}

// Open loads (or creates) the encrypted vault at path using the given master password.
func Open(path string, masterPass []byte) (*Store, error) {
	s := &Store{path: path}

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read vault: %w", err)
	}

	if os.IsNotExist(err) || len(raw) == 0 {
		salt, err := generateSalt()
		if err != nil {
			return nil, err
		}
		s.salt = salt
		s.key = deriveKey(masterPass, salt)
		s.vault = vault{Version: 1, Records: []Record{}}
		return s, nil
	}

	salt, ciphertext, err := parseFile(raw)
	if err != nil {
		return nil, fmt.Errorf("parse vault: %w", err)
	}
	s.salt = salt
	s.key = deriveKey(masterPass, salt)

	plaintext, err := decrypt(s.key, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt vault (wrong password?): %w", err)
	}

	if err := json.Unmarshal(plaintext, &s.vault); err != nil {
		return nil, fmt.Errorf("unmarshal vault: %w", err)
	}
	return s, nil
}

// Save encrypts and writes the vault to disk atomically.
func (s *Store) Save() error {
	plaintext, err := json.Marshal(s.vault)
	if err != nil {
		return err
	}
	ciphertext, err := encrypt(s.key, plaintext)
	if err != nil {
		return err
	}
	raw := buildFile(s.salt, ciphertext)

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ── Folders ───────────────────────────────────────────────────────────────────

// FoldersInFolder returns direct child folders of parentID ("" = root), sorted by name.
func (s *Store) FoldersInFolder(parentID string) []Folder {
	var out []Folder
	for _, f := range s.vault.Folders {
		if f.ParentID == parentID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetFolder returns a folder by ID.
func (s *Store) GetFolder(id string) (Folder, bool) {
	for _, f := range s.vault.Folders {
		if f.ID == id {
			return f, true
		}
	}
	return Folder{}, false
}

// FolderPath returns the chain of folders from root to id (inclusive), root first.
func (s *Store) FolderPath(id string) []Folder {
	if id == "" {
		return nil
	}
	var rev []Folder
	cur := id
	for cur != "" {
		f, ok := s.GetFolder(cur)
		if !ok {
			break
		}
		rev = append(rev, f)
		cur = f.ParentID
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// AddFolder appends a new folder and saves.
func (s *Store) AddFolder(f Folder) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	s.vault.Folders = append(s.vault.Folders, f)
	return s.Save()
}

// UpdateFolder replaces a folder by ID and saves.
func (s *Store) UpdateFolder(f Folder) error {
	for i, existing := range s.vault.Folders {
		if existing.ID == f.ID {
			f.CreatedAt = existing.CreatedAt
			s.vault.Folders[i] = f
			return s.Save()
		}
	}
	return fmt.Errorf("folder %s not found", f.ID)
}

// DeleteFolder removes a folder and all its descendants (sub-folders + records) recursively.
func (s *Store) DeleteFolder(id string) error {
	s.deleteFolderTree(id)
	return s.Save()
}

func (s *Store) deleteFolderTree(id string) {
	for _, f := range s.vault.Folders {
		if f.ParentID == id {
			s.deleteFolderTree(f.ID)
		}
	}
	kept := s.vault.Records[:0]
	for _, r := range s.vault.Records {
		if r.FolderID != id {
			kept = append(kept, r)
		} else if s.vault.ActiveID == r.ID {
			s.vault.ActiveID = ""
		}
	}
	s.vault.Records = kept
	folders := s.vault.Folders[:0]
	for _, f := range s.vault.Folders {
		if f.ID != id {
			folders = append(folders, f)
		}
	}
	s.vault.Folders = folders
}

// MoveFolder changes the ParentID of a folder ("" = root).
// Returns an error if the target would create a cycle.
func (s *Store) MoveFolder(id, targetParentID string) error {
	if id == targetParentID {
		return fmt.Errorf("cannot move a folder into itself")
	}
	cur := targetParentID
	for cur != "" {
		f, ok := s.GetFolder(cur)
		if !ok {
			break
		}
		if f.ParentID == id {
			return fmt.Errorf("cannot move a folder into its own descendant")
		}
		cur = f.ParentID
	}
	for i, f := range s.vault.Folders {
		if f.ID == id {
			s.vault.Folders[i].ParentID = targetParentID
			return s.Save()
		}
	}
	return fmt.Errorf("folder %s not found", id)
}

// ── Records ───────────────────────────────────────────────────────────────────

// RecordsInFolder returns records that belong directly to folderID ("" = root).
func (s *Store) RecordsInFolder(folderID string) []Record {
	var out []Record
	for _, r := range s.vault.Records {
		if r.FolderID == folderID {
			out = append(out, r.Clone())
		}
	}
	return out
}

// RecordsUnder returns all records at or beneath folderID, recursively.
// folderID == "" means every record in the vault.
func (s *Store) RecordsUnder(folderID string) []Record {
	if folderID == "" {
		out := make([]Record, len(s.vault.Records))
		for i, r := range s.vault.Records {
			out[i] = r.Clone()
		}
		return out
	}
	var out []Record
	var walk func(id string)
	walk = func(id string) {
		for _, r := range s.vault.Records {
			if r.FolderID == id {
				out = append(out, r.Clone())
			}
		}
		for _, f := range s.vault.Folders {
			if f.ParentID == id {
				walk(f.ID)
			}
		}
	}
	walk(folderID)
	return out
}

// Get returns a record by ID.
func (s *Store) Get(id string) (Record, bool) {
	for _, r := range s.vault.Records {
		if r.ID == id {
			return r.Clone(), true
		}
	}
	return Record{}, false
}

// Add appends a new record and saves.
func (s *Store) Add(r Record) error {
	r.CreatedAt = time.Now()
	r.UpdatedAt = r.CreatedAt
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	s.vault.Records = append(s.vault.Records, r)
	return s.Save()
}

// Update replaces a record by ID and saves.
func (s *Store) Update(r Record) error {
	for i, existing := range s.vault.Records {
		if existing.ID == r.ID {
			r.CreatedAt = existing.CreatedAt
			r.UpdatedAt = time.Now()
			s.vault.Records[i] = r
			return s.Save()
		}
	}
	return fmt.Errorf("record %s not found", r.ID)
}

// Delete removes a record by ID and saves.
func (s *Store) Delete(id string) error {
	records := s.vault.Records[:0]
	found := false
	for _, r := range s.vault.Records {
		if r.ID == id {
			found = true
			continue
		}
		records = append(records, r)
	}
	if !found {
		return fmt.Errorf("record %s not found", id)
	}
	s.vault.Records = records
	if s.vault.ActiveID == id {
		s.vault.ActiveID = ""
	}
	return s.Save()
}

// MoveRecord changes the FolderID of a record ("" = root).
func (s *Store) MoveRecord(id, targetFolderID string) error {
	for i, r := range s.vault.Records {
		if r.ID == id {
			s.vault.Records[i].FolderID = targetFolderID
			s.vault.Records[i].UpdatedAt = time.Now()
			return s.Save()
		}
	}
	return fmt.Errorf("record %s not found", id)
}

// ── Active record tracking ───────────────────────────────────────────────────

// ActiveID returns the ID of the record currently applied to ~/.my.cnf ("" = none).
func (s *Store) ActiveID() string { return s.vault.ActiveID }

// SetActiveID records which profile was last written to ~/.my.cnf and saves.
func (s *Store) SetActiveID(id string) error {
	s.vault.ActiveID = id
	return s.Save()
}

// ClearActiveID forgets the active profile (used after `clean`) and saves.
func (s *Store) ClearActiveID() error {
	s.vault.ActiveID = ""
	return s.Save()
}

// AllTagNames returns the distinct set of tag names currently used across all
// records, sorted alphabetically. There is no separate tag registry — a tag
// is just a string on a Record; its display color is derived deterministically
// from its name (see tui.tagColor).
func (s *Store) AllTagNames() []string {
	seen := make(map[string]struct{})
	for _, r := range s.vault.Records {
		for _, t := range r.Tags {
			seen[t] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
