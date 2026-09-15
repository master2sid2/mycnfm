package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mycnfm/internal/config"
)

// optionCatalogDoneMsg closes the catalog and returns to the list.
type optionCatalogDoneMsg struct{}

// optionCatalogModel manages the global option definitions (press O on the
// list). Every host sees these as checkboxes; this screen only edits the
// catalog itself, not any one host's enabled state.
type optionCatalogModel struct {
	store         *config.Store
	defs          []config.OptionDef
	cursor        int
	editor        *optionDefEditorModel
	confirmDelete bool
	err           string
	width         int
	height        int
}

func newOptionCatalog(store *config.Store) optionCatalogModel {
	m := optionCatalogModel{store: store}
	m.reload()
	return m
}

func (m *optionCatalogModel) reload() {
	m.defs = m.store.OptionDefs()
	if m.cursor > len(m.defs) {
		m.cursor = len(m.defs)
	}
}

func (m optionCatalogModel) Init() tea.Cmd { return nil }

func (m optionCatalogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = wsm.Width, wsm.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.editor != nil {
		return m.updateEditor(key)
	}

	if m.confirmDelete {
		switch key.String() {
		case "y", "Y":
			if m.cursor < len(m.defs) {
				if err := m.store.DeleteOptionDef(m.defs[m.cursor].Key); err != nil {
					m.err = err.Error()
				}
				m.reload()
			}
			m.confirmDelete = false
		case "n", "N", "esc":
			m.confirmDelete = false
		}
		return m, nil
	}

	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return optionCatalogDoneMsg{} }

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.defs) {
			m.cursor++
		}

	case "n":
		ed := newOptionDefEditor(config.OptionDef{}, true, "")
		m.editor = &ed
		return m, textinput.Blink

	case "enter", "e":
		if m.cursor < len(m.defs) {
			d := m.defs[m.cursor]
			ed := newOptionDefEditor(d, false, d.Key)
			m.editor = &ed
		} else {
			ed := newOptionDefEditor(config.OptionDef{}, true, "")
			m.editor = &ed
		}
		return m, textinput.Blink

	case "d":
		if m.cursor < len(m.defs) {
			m.confirmDelete = true
		}
	}

	return m, nil
}

func (m optionCatalogModel) updateEditor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.editor = nil
		return m, nil

	case "ctrl+s":
		d, ok := m.editor.getResult()
		if !ok {
			m.editor.err = "key is required"
			return m, nil
		}
		var err error
		if m.editor.isNew {
			err = m.store.AddOptionDef(d)
		} else {
			err = m.store.UpdateOptionDef(m.editor.origKey, d)
		}
		if err != nil {
			m.editor.err = err.Error()
			return m, nil
		}
		m.editor = nil
		m.reload()
		return m, nil
	}

	updated, cmd := m.editor.update(key)
	m.editor = &updated
	return m, cmd
}

func (m optionCatalogModel) View() string {
	if m.editor != nil {
		return m.editor.view(m.width)
	}

	innerW := m.width - 10
	if innerW < 46 {
		innerW = 46
	}
	if innerW > 72 {
		innerW = 72
	}

	var sb strings.Builder
	if len(m.defs) == 0 {
		sb.WriteString(frmDimStyle.Italic(true).Render("  No global options yet") + "\n")
	}
	for i, d := range m.defs {
		sel := i == m.cursor
		label := d.Key
		if d.Value != "" {
			label += " = " + d.Value
		}
		if sel {
			sb.WriteString(lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("  ❯ "+label) + "\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(clrText).Render("    "+label) + "\n")
		}
	}

	if m.cursor == len(m.defs) {
		sb.WriteString(lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("  ❯ ") +
			lipgloss.NewStyle().Foreground(clrSuccess).Bold(true).Render("[ + New option ]") + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(clrSuccess).Render("    [ + New option ]") + "\n")
	}

	if m.confirmDelete && m.cursor < len(m.defs) {
		sb.WriteString("\n" + frmErrStyle.Render(fmt.Sprintf("  Delete %q? This unchecks it on every host.  [y]es / [n]o", m.defs[m.cursor].Key)) + "\n")
	}
	if m.err != "" {
		sb.WriteString("\n" + frmErrStyle.Render("  ✗ "+m.err) + "\n")
	}

	sb.WriteString("\n" + frmDimStyle.Render("  These appear as checkboxes on every host — see them there to enable.") + "\n")
	sb.WriteString("\n" + frmSepStyle.Render(strings.Repeat("─", innerW-6)) + "\n")
	sb.WriteString(frmHelpStyle.Render("  ↑↓ navigate   enter/e edit   n new   d delete   esc close"))

	return renderModal("⚙", "Options Catalog", innerW, sb.String())
}

// ── optionDefEditorModel ──────────────────────────────────────────────────────

const (
	odFldKey = iota
	odFldValue
	odFldCount
)

// optionDefEditorModel edits one catalog definition's key/value. Unlike
// optionEditorModel (per-host custom options), there's no enabled toggle
// here — enabling happens per host, not in the catalog.
type optionDefEditorModel struct {
	key     textinput.Model
	value   textinput.Model
	focused int
	isNew   bool
	origKey string
	err     string
}

func newOptionDefEditor(d config.OptionDef, isNew bool, origKey string) optionDefEditorModel {
	mk := func(ph string) textinput.Model {
		t := textinput.New()
		t.Placeholder = ph
		t.CharLimit = 128
		t.PromptStyle = lipgloss.NewStyle().Foreground(clrAccent)
		t.TextStyle = lipgloss.NewStyle().Foreground(clrText)
		return t
	}
	k := mk("e.g. default-character-set")
	v := mk("e.g. utf8mb3 (leave blank for a bare flag)")
	k.SetValue(d.Key)
	v.SetValue(d.Value)

	m := optionDefEditorModel{key: k, value: v, isNew: isNew, origKey: origKey}
	m.applyFocus()
	return m
}

func (m *optionDefEditorModel) activeInput() *textinput.Model {
	if m.focused == odFldKey {
		return &m.key
	}
	return &m.value
}

func (m *optionDefEditorModel) applyFocus() {
	m.key.Blur()
	m.value.Blur()
	m.activeInput().Focus()
}

func (m optionDefEditorModel) update(key tea.KeyMsg) (optionDefEditorModel, tea.Cmd) {
	m.err = ""
	switch key.String() {
	case "tab", "down", "shift+tab", "up":
		m.focused = (m.focused + 1) % odFldCount
		m.applyFocus()
		return m, nil
	}
	inp := m.activeInput()
	var cmd tea.Cmd
	*inp, cmd = inp.Update(key)
	return m, cmd
}

func (m optionDefEditorModel) getResult() (config.OptionDef, bool) {
	key := strings.TrimSpace(m.key.Value())
	if key == "" {
		return config.OptionDef{}, false
	}
	return config.OptionDef{Key: key, Value: strings.TrimSpace(m.value.Value())}, true
}

func (m optionDefEditorModel) view(width int) string {
	innerW := width - 10
	if innerW < 44 {
		innerW = 44
	}
	if innerW > 68 {
		innerW = 68
	}
	inputW := innerW - frmLabelW - 8

	title := "New Option"
	if !m.isNew {
		title = "Edit Option"
	}

	var sb strings.Builder
	for _, f := range []struct {
		label string
		fld   int
		inp   *textinput.Model
	}{
		{"Key", odFldKey, &m.key},
		{"Value", odFldValue, &m.value},
	} {
		f.inp.Width = inputW
		lbl := frmLabelStyle.Render(f.label)
		bar := frmDimStyle.Render("│")
		if m.focused == f.fld {
			lbl = frmFocusLabelStyle.Render(f.label)
			bar = frmBarStyle.Render("│")
		}
		sb.WriteString(fmt.Sprintf("  %s %s %s\n", lbl, bar, f.inp.View()))
	}

	if m.err != "" {
		sb.WriteString("\n" + frmErrStyle.Render("  ✗ "+m.err) + "\n")
	}

	sb.WriteString("\n" + frmSepStyle.Render(strings.Repeat("─", innerW-6)) + "\n")
	sb.WriteString(frmHelpStyle.Render("  tab switch field   ctrl+s save   esc cancel"))

	return renderModal("⚙", title, innerW, sb.String())
}
