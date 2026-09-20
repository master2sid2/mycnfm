package tui

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mycnfm/internal/config"
)

type recordSavedMsg struct{ record config.Record }
type recordCancelledMsg struct{}

const (
	fldName = iota
	fldGroup
	fldPort
	fldSocket
	fldUser
	fldPassword
	fldDatabase
	fldGlobalOptions // catalog checkboxes — hidden when the catalog is empty
	fldCustomOptions // per-host key/value options
	fldTags
	fldNotes
	fldCount
)

var fieldLabels = [fldCount]string{
	fldName:     "Name",
	fldGroup:    "Group",
	fldPort:     "Port",
	fldSocket:   "Socket",
	fldUser:     "User",
	fldPassword: "Password",
	fldDatabase: "Database",
	fldTags:     "Tags",
	fldNotes:    "Notes",
}

type recordFormModel struct {
	orig         config.Record
	isEdit       bool
	existingTags []string

	inputs     [fldCount]textinput.Model
	focusField int

	// Global (catalog) options — enabled state lives on the record, the
	// key/value definitions live in the catalog (edited via O on the list).
	globalDefs    []config.OptionDef
	globalEnabled map[string]bool
	globalCursor  int

	// Custom (this-host-only) options.
	customOptions []config.Option
	customCursor  int
	optEditor     *optionEditorModel

	err   string
	width int
}

func newRecordFormModel(r config.Record, isEdit bool, store *config.Store) recordFormModel {
	mkInput := func(placeholder string, pw bool) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = 256
		t.PromptStyle = lipgloss.NewStyle().Foreground(clrAccent)
		t.TextStyle = lipgloss.NewStyle().Foreground(clrText)
		if pw {
			t.EchoMode = textinput.EchoPassword
		}
		return t
	}

	customOptions := slices.Clone(r.CustomOptions)

	// Must stay non-nil: toggling a checkbox writes into this map directly.
	globalEnabled := maps.Clone(r.GlobalOptions)
	if globalEnabled == nil {
		globalEnabled = make(map[string]bool)
	}

	m := recordFormModel{
		orig:          r,
		isEdit:        isEdit,
		existingTags:  store.AllTagNames(),
		globalDefs:    store.OptionDefs(),
		globalEnabled: globalEnabled,
		customOptions: customOptions,
	}
	m.inputs[fldName] = mkInput("db-01", false)
	m.inputs[fldGroup] = mkInput("optional, e.g. auth / mail", false)
	m.inputs[fldPort] = mkInput("blank = 3306, only set if different", false)
	m.inputs[fldSocket] = mkInput("/var/run/mysqld/mysqld.sock", false)
	m.inputs[fldUser] = mkInput("username", false)
	m.inputs[fldPassword] = mkInput("password", true)
	m.inputs[fldDatabase] = mkInput("optional default schema", false)
	m.inputs[fldTags] = mkInput("comma,separated,tags", false)
	m.inputs[fldNotes] = mkInput("optional", false)

	portStr := ""
	if r.Port != 0 {
		portStr = strconv.Itoa(r.Port)
	}

	m.inputs[fldName].SetValue(r.Name)
	m.inputs[fldGroup].SetValue(r.Group)
	m.inputs[fldPort].SetValue(portStr)
	m.inputs[fldSocket].SetValue(r.Socket)
	m.inputs[fldUser].SetValue(r.User)
	m.inputs[fldPassword].SetValue(r.Password)
	m.inputs[fldDatabase].SetValue(r.Database)
	m.inputs[fldTags].SetValue(strings.Join(r.Tags, ", "))
	m.inputs[fldNotes].SetValue(r.Notes)

	m.focusField = fldName
	m.applyFocus()
	return m
}

// visibleFields lists focus stops in order, skipping the global-options
// section entirely when the catalog is empty.
func (m *recordFormModel) visibleFields() []int {
	fields := []int{fldName, fldGroup, fldPort, fldSocket, fldUser, fldPassword, fldDatabase}
	if len(m.globalDefs) > 0 {
		fields = append(fields, fldGlobalOptions)
	}
	return append(fields, fldCustomOptions, fldTags, fldNotes)
}

func (m *recordFormModel) applyFocus() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	if m.focusField != fldGlobalOptions && m.focusField != fldCustomOptions {
		m.inputs[m.focusField].Focus()
	}
}

func (m *recordFormModel) setFocus(field int) {
	m.focusField = field
	m.applyFocus()
}

func (m *recordFormModel) nextField() {
	vf := m.visibleFields()
	for i, f := range vf {
		if f == m.focusField {
			m.setFocus(vf[(i+1)%len(vf)])
			return
		}
	}
	m.setFocus(vf[0])
}

func (m *recordFormModel) prevField() {
	vf := m.visibleFields()
	for i, f := range vf {
		if f == m.focusField {
			m.setFocus(vf[(i-1+len(vf))%len(vf)])
			return
		}
	}
	m.setFocus(vf[0])
}

func (m recordFormModel) Init() tea.Cmd { return textinput.Blink }

func (m recordFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.optEditor != nil {
		return m.updateOptionEditor(key)
	}
	return m.handleKey(key)
}

func (m recordFormModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return recordCancelledMsg{} }

	case "ctrl+s":
		return m.submit()

	case "tab", "down":
		switch m.focusField {
		case fldGlobalOptions:
			if m.globalCursor < len(m.globalDefs)-1 {
				m.globalCursor++
			} else {
				m.nextField()
			}
		case fldCustomOptions:
			total := len(m.customOptions) + 1
			if m.customCursor < total-1 {
				m.customCursor++
			} else {
				m.nextField()
			}
		default:
			m.nextField()
		}
		return m, nil

	case "shift+tab", "up":
		switch m.focusField {
		case fldGlobalOptions:
			if m.globalCursor > 0 {
				m.globalCursor--
			} else {
				m.prevField()
			}
		case fldCustomOptions:
			if m.customCursor > 0 {
				m.customCursor--
			} else {
				m.prevField()
			}
		default:
			m.prevField()
		}
		return m, nil

	case "enter":
		if m.focusField == fldCustomOptions {
			if m.customCursor < len(m.customOptions) {
				return m.openOptionEditor(m.customCursor)
			}
			return m.openOptionEditor(-1)
		}
		m.nextField()
		return m, nil

	case " ":
		switch m.focusField {
		case fldGlobalOptions:
			if m.globalCursor < len(m.globalDefs) {
				key := m.globalDefs[m.globalCursor].ID()
				m.globalEnabled[key] = !m.globalEnabled[key]
			}
			return m, nil
		case fldCustomOptions:
			if m.customCursor < len(m.customOptions) {
				m.customOptions[m.customCursor].Enabled = !m.customOptions[m.customCursor].Enabled
			}
			return m, nil
		}
		return m.updateInput(key)

	case "d", "delete":
		if m.focusField == fldCustomOptions {
			if m.customCursor < len(m.customOptions) {
				m.customOptions = append(m.customOptions[:m.customCursor], m.customOptions[m.customCursor+1:]...)
				if m.customCursor > 0 && m.customCursor >= len(m.customOptions) {
					m.customCursor--
				}
			}
			return m, nil
		}
		return m.updateInput(key)
	}

	return m.updateInput(key)
}

// updateInput forwards a key event to whichever text input currently has focus.
func (m recordFormModel) updateInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focusField == fldGlobalOptions || m.focusField == fldCustomOptions {
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focusField], cmd = m.inputs[m.focusField].Update(key)
	return m, cmd
}

func (m recordFormModel) openOptionEditor(idx int) (tea.Model, tea.Cmd) {
	var o config.Option
	isNew := idx < 0
	if !isNew {
		o = m.customOptions[idx]
	}
	ed := newOptionEditor(o, isNew, idx)
	m.optEditor = &ed
	return m, textinput.Blink
}

func (m recordFormModel) updateOptionEditor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.optEditor = nil
		m.applyFocus()
		return m, textinput.Blink

	case "ctrl+s":
		o, ok := m.optEditor.getResult()
		if !ok {
			m.optEditor.err = "key is required"
			return m, nil
		}
		if m.optEditor.isNew {
			m.customOptions = append(m.customOptions, o)
			m.customCursor = len(m.customOptions) - 1
		} else {
			m.customOptions[m.optEditor.editIdx] = o
		}
		m.optEditor = nil
		m.focusField = fldCustomOptions
		m.applyFocus()
		return m, textinput.Blink
	}

	updated, cmd := m.optEditor.update(key)
	m.optEditor = &updated
	return m, cmd
}

func (m recordFormModel) submit() (tea.Model, tea.Cmd) {
	m.err = ""
	name := strings.TrimSpace(m.inputs[fldName].Value())
	if name == "" {
		m.err = "name is required"
		m.setFocus(fldName)
		return m, nil
	}

	port := 0
	portStr := strings.TrimSpace(m.inputs[fldPort].Value())
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			m.err = "port must be 1–65535 (or left blank)"
			m.setFocus(fldPort)
			return m, nil
		}
		port = p
	}

	var tags []string
	seen := make(map[string]struct{})
	for t := range strings.SplitSeq(m.inputs[fldTags].Value(), ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		tags = append(tags, t)
	}

	r := m.orig
	r.Name = name
	r.Group = strings.TrimSpace(m.inputs[fldGroup].Value())
	r.Port = port
	r.Socket = strings.TrimSpace(m.inputs[fldSocket].Value())
	r.User = strings.TrimSpace(m.inputs[fldUser].Value())
	r.Password = m.inputs[fldPassword].Value()
	r.Database = strings.TrimSpace(m.inputs[fldDatabase].Value())
	r.GlobalOptions = m.globalEnabled
	r.CustomOptions = m.customOptions
	r.Tags = tags
	r.Notes = strings.TrimSpace(m.inputs[fldNotes].Value())

	return m, func() tea.Msg { return recordSavedMsg{r} }
}

// ── View styles (shared by every form/modal in the package) ─────────────────

const frmLabelW = 10

var (
	frmLabelStyle = lipgloss.NewStyle().Width(frmLabelW).Foreground(clrDim)

	frmFocusLabelStyle = lipgloss.NewStyle().Width(frmLabelW).Foreground(clrFocus).Bold(true)

	frmSectionStyle = lipgloss.NewStyle().Foreground(clrAccent).Bold(true)

	frmSepStyle = lipgloss.NewStyle().Foreground(clrMuted)

	frmErrStyle = lipgloss.NewStyle().Foreground(clrError).Bold(true)

	frmDimStyle = lipgloss.NewStyle().Foreground(clrDim)

	frmHelpStyle = lipgloss.NewStyle().Foreground(clrDim)

	frmBarStyle = lipgloss.NewStyle().Foreground(clrFocus).Bold(true)
)

func (m recordFormModel) View() string {
	if m.optEditor != nil {
		return m.optEditor.view(m.width)
	}

	innerW := m.width - 10
	if innerW < 46 {
		innerW = 46
	}
	if innerW > 84 {
		innerW = 84
	}
	inputW := innerW - frmLabelW - 8

	var sb strings.Builder
	title := "New Host"
	if m.isEdit {
		title = "Edit Host"
	}

	sb.WriteString(frmSectionStyle.Render("Identity") + "\n")
	for _, f := range []int{fldName, fldGroup} {
		m.inputs[f].Width = inputW
		sb.WriteString(m.renderField(f, inputW))
	}

	sb.WriteString("\n" + frmSectionStyle.Render("Connection") + "\n")
	sb.WriteString(frmDimStyle.Render("  Leave everything but Name blank if you only need user+password.") + "\n")
	for _, f := range []int{fldPort, fldSocket, fldUser, fldPassword, fldDatabase} {
		m.inputs[f].Width = inputW
		sb.WriteString(m.renderField(f, inputW))
	}

	if len(m.globalDefs) > 0 {
		sb.WriteString("\n" + frmSectionStyle.Render("Options") + "\n")
		sb.WriteString(m.renderGlobalOptions())
	}

	sb.WriteString("\n" + frmSectionStyle.Render("Custom options (this host only)") + "\n")
	sb.WriteString(m.renderCustomOptions())

	sb.WriteString("\n" + frmSectionStyle.Render("Metadata") + "\n")
	m.inputs[fldTags].Width = inputW
	sb.WriteString(m.renderField(fldTags, inputW))
	if len(m.existingTags) > 0 {
		sb.WriteString(frmDimStyle.Render("             existing: "+strings.Join(m.existingTags, ", ")) + "\n")
	}
	m.inputs[fldNotes].Width = inputW
	sb.WriteString(m.renderField(fldNotes, inputW))

	if m.err != "" {
		sb.WriteString("\n" + frmErrStyle.Render("  ✗ "+m.err) + "\n")
	}

	sb.WriteString("\n" + frmSepStyle.Render(strings.Repeat("─", innerW-6)) + "\n")
	switch m.focusField {
	case fldGlobalOptions:
		sb.WriteString(frmHelpStyle.Render("  tab/↑↓ navigate   space toggle   ctrl+s save   esc cancel"))
	case fldCustomOptions:
		sb.WriteString(frmHelpStyle.Render("  tab/↑↓ navigate   space toggle   enter edit   d delete   ctrl+s save   esc cancel"))
	default:
		sb.WriteString(frmHelpStyle.Render("  tab/↑↓ navigate   ctrl+s save   esc cancel"))
	}

	return renderModal("◆", title, innerW, sb.String())
}

// renderGlobalOptions draws the catalog checkboxes. The list itself (add /
// rename / delete a definition) is managed from the Options catalog (O on
// the list), not here — a host only flips enabled/disabled.
func (m recordFormModel) renderGlobalOptions() string {
	inGlobal := m.focusField == fldGlobalOptions
	var sb strings.Builder
	for i, def := range m.globalDefs {
		selected := inGlobal && m.globalCursor == i
		box := "[ ]"
		if m.globalEnabled[def.ID()] {
			box = "[x]"
		}
		label := def.Key
		if def.Value != "" {
			label += " = " + def.Value
		}
		if def.Section != "" && def.Section != config.DefaultSection {
			label = "[" + def.Section + "] " + label
		}
		row := fmt.Sprintf("%s %s", box, label)
		switch {
		case selected:
			sb.WriteString(lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("  ❯ "+row) +
				frmDimStyle.Render("   space toggle") + "\n")
		case m.globalEnabled[def.ID()]:
			sb.WriteString(lipgloss.NewStyle().Foreground(clrText).Render("    "+row) + "\n")
		default:
			sb.WriteString(frmDimStyle.Render("    " + row + "\n"))
		}
	}
	return sb.String()
}

// renderCustomOptions draws the per-host checkbox list plus a trailing
// "+ Add option" row.
func (m recordFormModel) renderCustomOptions() string {
	inCustom := m.focusField == fldCustomOptions
	var sb strings.Builder

	if len(m.customOptions) == 0 {
		sb.WriteString(frmDimStyle.Italic(true).Render("  No custom options on this host") + "\n")
	}
	for i, o := range m.customOptions {
		selected := inCustom && m.customCursor == i
		box := "[ ]"
		if o.Enabled {
			box = "[x]"
		}
		label := o.Key
		if o.Value != "" {
			label += " = " + o.Value
		}
		if o.Section != "" && o.Section != config.DefaultSection {
			label = "[" + o.Section + "] " + label
		}
		row := fmt.Sprintf("%s %s", box, label)
		if selected {
			sb.WriteString(lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("  ❯ "+row) +
				frmDimStyle.Render("   space toggle  enter edit  d delete") + "\n")
		} else if o.Enabled {
			sb.WriteString(lipgloss.NewStyle().Foreground(clrText).Render("    "+row) + "\n")
		} else {
			sb.WriteString(frmDimStyle.Render("    " + row + "\n"))
		}
	}

	addSelected := inCustom && m.customCursor == len(m.customOptions)
	if addSelected {
		sb.WriteString(lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("  ❯ ") +
			lipgloss.NewStyle().Foreground(clrSuccess).Bold(true).Render("[ + Add option ]") + "\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(clrSuccess).Render("    [ + Add option ]") + "\n")
	}
	return sb.String()
}

func (m recordFormModel) renderField(field int, _ int) string {
	focused := m.focusField == field
	lbl := frmLabelStyle.Render(fieldLabels[field])
	bar := frmDimStyle.Render("│")
	if focused {
		lbl = frmFocusLabelStyle.Render(fieldLabels[field])
		bar = frmBarStyle.Render("│")
	}
	return fmt.Sprintf("  %s %s %s\n", lbl, bar, m.inputs[field].View())
}
