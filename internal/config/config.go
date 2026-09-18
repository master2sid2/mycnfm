package config

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// OptionDef is a globally defined optional [client] setting that any host can
// switch on or off. Defined once from the Options catalog (press O on the
// list); a host only stores whether it's enabled, so adding, renaming or
// deleting a definition affects what every host sees immediately — nothing
// is baked into a host at creation time.
type OptionDef struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"` // "" = bare flag (no "= value")
}

// defaultOptionDefs seeds a brand-new vault with the one knob most setups
// care about — the client character set, defaulting to utf8mb3. It starts
// unchecked on every host; nothing is written until a host enables it.
func defaultOptionDefs() []OptionDef {
	return []OptionDef{
		{Key: "default-character-set", Value: "utf8mb3"},
	}
}

// Option is a custom [client] entry that belongs to one host only — unlike a
// catalog OptionDef, it's not shared or visible anywhere else.
type Option struct {
	Key     string `json:"key"`
	Value   string `json:"value,omitempty"`
	Enabled bool   `json:"enabled"`
}

// Record holds one set of MySQL credentials to write into ~/.my.cnf.
// Deliberately no remote host field: the same user/password commonly serves
// several hosts within an environment, and the actual target is normally
// given on the command line (mysql -h ...), which overrides the option file
// anyway. Socket is kept because it's local-machine config, not per-target.
type Record struct {
	ID       string `json:"id"`
	FolderID string `json:"folder_id,omitempty"` // "" = root level
	Group    string `json:"group,omitempty"`     // free-text display cluster, e.g. "auth", "mail"
	Name     string `json:"name"`
	Port     int    `json:"port,omitzero"`
	Socket   string `json:"socket,omitempty"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"` // stored encrypted at rest
	Database string `json:"database,omitempty"`

	// GlobalOptions is this host's enabled/disabled state for each catalog
	// OptionDef, keyed by OptionDef.Key. Absent = disabled.
	GlobalOptions map[string]bool `json:"global_options,omitempty"`
	// CustomOptions are extra [client] entries that exist only on this host.
	CustomOptions []Option `json:"custom_options,omitempty"`

	Tags      []string  `json:"tags,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewRecord returns a Record with generated ID and timestamps. Every field
// beyond Name/ID starts blank/disabled — including Port, so a host that only
// needs user+password doesn't get an unwanted "port = 3306" written for it.
func NewRecord() Record {
	now := time.Now()
	return Record{
		ID:        uuid.New().String(),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Clone returns a deep copy of the record.
func (r Record) Clone() Record {
	cp := r
	cp.Tags = slices.Clone(r.Tags)
	cp.CustomOptions = slices.Clone(r.CustomOptions)
	cp.GlobalOptions = maps.Clone(r.GlobalOptions)
	return cp
}

// vault is the plaintext structure stored (encrypted) on disk.
type vault struct {
	Version    int         `json:"version"`
	Folders    []Folder    `json:"folders,omitempty"`
	Records    []Record    `json:"records"`
	OptionDefs []OptionDef `json:"option_defs,omitempty"`
	ActiveID   string      `json:"active_id,omitempty"`   // last record ID applied to ~/.my.cnf
	ActiveHash string      `json:"active_hash,omitempty"` // sha256 of the exact content last written, to detect drift
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
		s.vault = vault{Version: 1, Records: []Record{}, OptionDefs: defaultOptionDefs()}
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

// ExportJSON returns the vault's contents as indented, human-readable JSON.
// The result is plaintext — every stored password is in the clear — so
// callers must treat it with the same care as the master password itself.
// Meant for one-off maintenance: changing the master password or handing a
// config to a colleague to re-encrypt under their own password.
func (s *Store) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(s.vault, "", "  ")
}

// ImportJSON builds a brand-new vault from previously exported JSON,
// encrypted under masterPass with a freshly generated salt. It does not read
// or touch any existing file at path — the caller must call Save to write
// it, and is responsible for warning about overwriting an existing vault.
func ImportJSON(path string, masterPass []byte, data []byte) (*Store, error) {
	var v vault
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if v.Version == 0 {
		v.Version = 1
	}
	salt, err := generateSalt()
	if err != nil {
		return nil, err
	}
	return &Store{
		path:  path,
		salt:  salt,
		key:   deriveKey(masterPass, salt),
		vault: v,
	}, nil
}

// Rekey re-encrypts the vault under a new master password — a fresh salt is
// generated and the file is rewritten in place. The caller must have already
// unlocked the store with the current password via Open.
func (s *Store) Rekey(newMasterPass []byte) error {
	salt, err := generateSalt()
	if err != nil {
		return err
	}
	s.salt = salt
	s.key = deriveKey(newMasterPass, salt)
	return s.Save()
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
	slices.SortFunc(out, func(a, b Folder) int { return cmp.Compare(a.Name, b.Name) })
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
	slices.Reverse(rev)
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
			s.clearActiveLocked()
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
		s.clearActiveLocked()
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

// ActiveHash returns the sha256 (hex) of the exact content last written for
// ActiveID, used to detect whether ~/.my.cnf has since been changed or
// deleted outside mycnfm.
func (s *Store) ActiveHash() string { return s.vault.ActiveHash }

// SetActive records which record was last written to ~/.my.cnf, along with a
// hash of the exact content written, and saves.
func (s *Store) SetActive(id, hash string) error {
	s.vault.ActiveID = id
	s.vault.ActiveHash = hash
	return s.Save()
}

// ClearActive forgets the active profile (used after `clean`) and saves.
func (s *Store) ClearActive() error {
	s.clearActiveLocked()
	return s.Save()
}

// clearActiveLocked resets active-record state without saving; callers save
// as part of a larger mutation (e.g. deleting the active record's folder).
func (s *Store) clearActiveLocked() {
	s.vault.ActiveID = ""
	s.vault.ActiveHash = ""
}

// ── Option catalog ────────────────────────────────────────────────────────────

// OptionDefs returns the global option catalog, sorted by key.
func (s *Store) OptionDefs() []OptionDef {
	out := slices.Clone(s.vault.OptionDefs)
	slices.SortFunc(out, func(a, b OptionDef) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

// AddOptionDef appends a new catalog entry and saves.
func (s *Store) AddOptionDef(d OptionDef) error {
	for _, existing := range s.vault.OptionDefs {
		if existing.Key == d.Key {
			return fmt.Errorf("option %q already exists", d.Key)
		}
	}
	s.vault.OptionDefs = append(s.vault.OptionDefs, d)
	return s.Save()
}

// UpdateOptionDef replaces the catalog entry at oldKey with d — including a
// key rename — migrating every record's enabled-state to the new key, and saves.
func (s *Store) UpdateOptionDef(oldKey string, d OptionDef) error {
	found := false
	for i, existing := range s.vault.OptionDefs {
		if existing.Key == oldKey {
			s.vault.OptionDefs[i] = d
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("option %q not found", oldKey)
	}
	if oldKey != d.Key {
		for i, r := range s.vault.Records {
			enabled, ok := r.GlobalOptions[oldKey]
			if !ok {
				continue
			}
			delete(s.vault.Records[i].GlobalOptions, oldKey)
			s.vault.Records[i].GlobalOptions[d.Key] = enabled
		}
	}
	return s.Save()
}

// DeleteOptionDef removes a catalog entry and strips it from every record.
func (s *Store) DeleteOptionDef(key string) error {
	defs := s.vault.OptionDefs[:0]
	for _, d := range s.vault.OptionDefs {
		if d.Key != key {
			defs = append(defs, d)
		}
	}
	s.vault.OptionDefs = defs
	for i := range s.vault.Records {
		delete(s.vault.Records[i].GlobalOptions, key)
	}
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
	return slices.Sorted(maps.Keys(seen))
}
