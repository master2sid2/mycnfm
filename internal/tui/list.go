package tui

import (
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mycnfm/internal/config"
)

// ── Sorting ───────────────────────────────────────────────────────────────────

type sortMode int

const (
	sortByName sortMode = iota
	// sortByContext sorts by "group" in folder-view or "env" in flattened
	// (group/filtered) views — whichever secondary dimension the current
	// view doesn't already show as its primary axis. See App.sortContextKey.
	sortByContext
	sortByUpdated
	sortModeCount
)

func (s sortMode) label(contextLabel string) string {
	switch s {
	case sortByContext:
		return contextLabel
	case sortByUpdated:
		return "updated"
	default:
		return "name"
	}
}

// sortRecords sorts recs in place. contextKey supplies the comparison value
// for sortByContext and is unused for the other modes (nil is fine then).
func sortRecords(recs []config.Record, mode sortMode, desc bool, contextKey func(config.Record) string) {
	sort.SliceStable(recs, func(i, j int) bool {
		var less bool
		switch mode {
		case sortByContext:
			less = strings.ToLower(contextKey(recs[i])) < strings.ToLower(contextKey(recs[j]))
		case sortByUpdated:
			less = recs[i].UpdatedAt.Before(recs[j].UpdatedAt)
		default:
			less = strings.ToLower(recs[i].Name) < strings.ToLower(recs[j].Name)
		}
		if desc {
			return !less
		}
		return less
	})
}

func sortStrings(ss []string) { sort.Strings(ss) }

// defaultDisplayPort mirrors mycnf's own default-port constant so the list
// doesn't clutter every row with ":3306" — it's implied when omitted.
const defaultDisplayPort = 3306

// connSummary renders a record's connection details for list/detail display:
// "user@socket", or just "user", plus ":port" when it differs from the
// default. There is deliberately no remote host — see config.Record.
func connSummary(r config.Record) string {
	var s string
	switch {
	case r.Socket != "" && r.User != "":
		s = fmt.Sprintf("%s@%s", r.User, r.Socket)
	case r.Socket != "":
		s = r.Socket
	case r.User != "":
		s = r.User
	}
	if r.Port != 0 && r.Port != defaultDisplayPort {
		s += fmt.Sprintf(":%d", r.Port)
	}
	if r.Database != "" {
		s += "/" + r.Database
	}
	return s
}

// ── Tag chip rendering ────────────────────────────────────────────────────────

// tagPalette is the fixed rotation of colors tags are hashed into — there is
// no persisted tag registry, so the same name always renders the same color.
var tagPalette = []string{"208", "214", "42", "81", "213", "203", "111", "190", "117", "183"}

func tagColor(name string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return lipgloss.Color(tagPalette[h.Sum32()%uint32(len(tagPalette))])
}

func renderTagChips(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var parts []string
	for _, t := range tags {
		parts = append(parts, lipgloss.NewStyle().
			Background(tagColor(t)).
			Foreground(lipgloss.Color("0")).
			Bold(true).
			Padding(0, 1).
			Render(t))
	}
	return strings.Join(parts, " ")
}

// ── Entries ───────────────────────────────────────────────────────────────────

type entryKind int

const (
	entryFolder entryKind = iota
	entryRecord
	entryHeader
)

// listEntry is one row in the main list — a folder, a record, or a
// non-selectable group-header divider.
type listEntry struct {
	kind     entryKind
	folder   config.Folder
	record   config.Record
	header   string // group label, when kind == entryHeader
	count    int    // records under this header
	path     string // breadcrumb shown next to a record in flattened views
	isActive bool   // true if this record is currently applied to ~/.my.cnf
}

func (e listEntry) FilterValue() string { return "" } // filtering is handled by the App, not bubbles/list

func entryKey(e listEntry) string {
	switch e.kind {
	case entryFolder:
		return "f:" + e.folder.ID
	case entryRecord:
		return "r:" + e.record.ID
	default:
		return ""
	}
}

// entryDelegate renders folders, records and group headers with distinct styles.
type entryDelegate struct{}

func (d entryDelegate) Height() int                             { return 2 }
func (d entryDelegate) Spacing() int                            { return 1 }
func (d entryDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d entryDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	e, ok := item.(listEntry)
	if !ok {
		return
	}

	width := m.Width()
	if width < 12 {
		width = 12
	}
	selected := index == m.Index()

	switch e.kind {
	case entryFolder:
		d.renderFolder(w, e.folder, width, selected)
	case entryRecord:
		d.renderRecord(w, e, width, selected)
	case entryHeader:
		d.renderHeader(w, e, width)
	}
}

func (d entryDelegate) renderFolder(w io.Writer, f config.Folder, width int, selected bool) {
	kind := "folder"
	if f.ParentID == "" {
		kind = "ENV"
	}
	if selected {
		line1 := lipgloss.NewStyle().
			Background(lipgloss.Color("37")).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Width(width).
			PaddingLeft(1).
			Render("▶  ▸ " + f.Name)
		line2 := lipgloss.NewStyle().
			Background(lipgloss.Color("23")).
			Foreground(lipgloss.Color("158")).
			Width(width).
			PaddingLeft(1).
			Render("     " + kind + "  ↵ open  esc back")
		fmt.Fprintf(w, "%s\n%s", line1, line2)
		return
	}
	line1 := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true).
		Width(width).
		PaddingLeft(2).
		Render("   ▸ " + f.Name)
	line2 := lipgloss.NewStyle().
		Foreground(lipgloss.Color("101")).
		Width(width).
		PaddingLeft(2).
		Render("     " + kind)
	fmt.Fprintf(w, "%s\n%s", line1, line2)
}

func (d entryDelegate) renderHeader(w io.Writer, e listEntry, width int) {
	label := " " + strings.ToUpper(e.header) + " "
	dashes := width - lipgloss.Width(label)
	if dashes < 0 {
		dashes = 0
	}
	line1 := lipgloss.NewStyle().Foreground(clrAccent).Bold(true).Width(width).
		Render("── " + strings.ToUpper(e.header) + " " + strings.Repeat("─", max(0, dashes-3)))
	line2 := lipgloss.NewStyle().Foreground(clrMuted).Width(width).
		Render(fmt.Sprintf("   %d host(s)", e.count))
	fmt.Fprintf(w, "%s\n%s", line1, line2)
}

func (d entryDelegate) renderRecord(w io.Writer, e listEntry, width int, selected bool) {
	r := e.record
	chips := renderTagChips(r.Tags)

	conn := connSummary(r)
	if e.path != "" {
		if conn != "" {
			conn += "   " + e.path
		} else {
			conn = e.path
		}
	}

	activeBadge := ""
	if e.isActive {
		activeBadge = "  ●"
	}

	if selected {
		line1 := lipgloss.NewStyle().
			Background(lipgloss.Color("37")).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Width(width).
			PaddingLeft(1).
			Render("▶  " + r.Name + activeBadge)
		var l2 strings.Builder
		l2.WriteString("   " + conn)
		if chips != "" {
			l2.WriteString("  " + chips)
		}
		line2 := lipgloss.NewStyle().
			Background(lipgloss.Color("23")).
			Foreground(lipgloss.Color("158")).
			Width(width).
			PaddingLeft(1).
			Render(l2.String())
		fmt.Fprintf(w, "%s\n%s", line1, line2)
		return
	}

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	if e.isActive {
		nameStyle = nameStyle.Foreground(clrSuccess).Bold(true)
	}
	line1 := lipgloss.NewStyle().Width(width).PaddingLeft(2).
		Render(nameStyle.Render("   " + r.Name + activeBadge))
	var l2 strings.Builder
	l2.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("   " + conn))
	if chips != "" {
		l2.WriteString("  " + chips)
	}
	line2 := lipgloss.NewStyle().Width(width).PaddingLeft(2).Render(l2.String())
	fmt.Fprintf(w, "%s\n%s", line1, line2)
}

// ── listModel ─────────────────────────────────────────────────────────────────

// listModel wraps bubbles/list and keeps the cursor off non-selectable
// entryHeader rows.
type listModel struct {
	inner list.Model
}

func newListModel() listModel {
	l := list.New(nil, entryDelegate{}, 0, 0)
	l.Title = ""
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	l.SetShowStatusBar(false)
	return listModel{inner: l}
}

func (m *listModel) setSize(w, h int) { m.inner.SetSize(w, h) }

func (m *listModel) setEntries(entries []listEntry) {
	prevKey, hasPrev := "", false
	if e, ok := m.selectedEntry(); ok {
		prevKey = entryKey(e)
		hasPrev = e.kind != entryHeader
	}

	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = e
	}
	m.inner.SetItems(items)

	idx := 0
	if hasPrev {
		for i, e := range entries {
			if entryKey(e) == prevKey {
				idx = i
				break
			}
		}
	}
	m.inner.Select(idx)
	m.selectNonHeader(idx, 1)
}

func (m *listModel) update(msg tea.KeyMsg) tea.Cmd {
	before := m.inner.Index()
	var cmd tea.Cmd
	m.inner, cmd = m.inner.Update(msg)
	after := m.inner.Index()

	items := m.inner.Items()
	if after >= 0 && after < len(items) {
		if e, ok := items[after].(listEntry); ok && e.kind == entryHeader {
			dir := 1
			switch {
			case after < before:
				dir = -1
			case after == before:
				switch msg.String() {
				case "up", "k", "pgup", "home", "g":
					dir = -1
				}
			}
			m.selectNonHeader(after, dir)
		}
	}
	return cmd
}

// selectNonHeader nudges the cursor from start in dir until it lands on a
// non-header row, falling back to the opposite direction if none is found.
func (m *listModel) selectNonHeader(start, dir int) {
	items := m.inner.Items()
	if len(items) == 0 {
		return
	}
	i := start
	for i >= 0 && i < len(items) {
		if e, ok := items[i].(listEntry); ok && e.kind != entryHeader {
			m.inner.Select(i)
			return
		}
		i += dir
	}
	i, dir = start, -dir
	for i >= 0 && i < len(items) {
		if e, ok := items[i].(listEntry); ok && e.kind != entryHeader {
			m.inner.Select(i)
			return
		}
		i += dir
	}
}

func (m *listModel) selectedEntry() (listEntry, bool) {
	e, ok := m.inner.SelectedItem().(listEntry)
	return e, ok
}

func (m *listModel) view() string { return m.inner.View() }
