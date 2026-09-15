package tui

import (
	"fmt"
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
	fldHost
	fldPort
	fldSocket
	fldUser
	fldPassword
	fldDatabase
	fldOptions
	fldTags
	fldNotes
	fldCount
)

var fieldLabels = [fldCount]string{
	fldName:     "Name",
	fldGroup:    "Group",
	fldHost:     "Host",
	fldPort:     "Port",
	fldSocket:   "Socket",
	fldUser:     "User",
	fldPassword: "Password",
	fldDatabase: "Database",
	fldOptions:  "Options",
	fldTags:     "Tags",
	fldNotes:    "Notes",
}

type recordFormModel struct {
	orig         config.Record
	isEdit       bool
	existingTags []string

	inputs     [fldCount]textinput.Model
	focusField int

	options   []config.Option
	optCursor int
	optEditor *optionEditorModel

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

	options := make([]config.Option, len(r.Options))
	copy(options, r.Options)

	m := recordFormModel{
		orig:         r,
		isEdit:       isEdit,
		existingTags: store.AllTagNames(),
		options:      options,
	}
	m.inputs[fldName] = mkInput("db-01", false)
	m.inputs[fldGroup] = mkInput("optional, e.g. auth / mail", false)
	m.inputs[fldHost] = mkInput("hostname or IP", false)
	m.inputs[fldPort] = mkInput("3306", false)
	m.inputs[fldSocket] = mkInput("/var/run/mysqld/mysqld.sock", false)
	m.inputs[fldUser] = mkInput("username", false)
	m.inputs[fldPassword] = mkInput("password", true)
	m.inputs[fldDatabase] = mkInput("optional default schema", false)
	m.inputs[fldTags] = mkInput("comma,separated,tags", false)
	m.inputs[fldNotes] = mkInput("optional", false)

	port := r.Port
	portStr := ""
	if port != 0 {
		portStr = strconv.Itoa(port)
	}

	m.inputs[fldName].SetValue(r.Name)
	m.inputs[fldGroup].SetValue(r.Group)
	m.inputs[fldHost].SetValue(r.Host)
	m.inputs[fldPort].SetValue(portStr)
	m.inputs[fldSocket].SetValue(r.Socket)
	m.inputs[fldUser].SetValue(r.User)
	m.inputs[fldPassword].SetValue(r.Password)
	m.inputs[fldDatabase].SetValue(r.Database)
	m.inputs[fldTags].SetValue(strings.Join(r.Tags, ", "))
	m.inputs[fldNotes].SetValue(r.Notes)

	m.applyFocus()
	return m
}

func (m *recordFormModel) applyFocus() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	if m.focusField != fldOptions {
		m.inputs[m.focusField].Focus()
	}
}

func (m *recordFormModel) nextField() {
	m.focusField = (m.focusField + 1) % fldCount
	m.applyFocus()
}

func (m *recordFormModel) prevField() {
	m.focusField = (m.focusField - 1 + fldCount) % fldCount
	m.applyFocus()
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
		if m.focusField == fldOptions {
			total := len(m.options) + 1
			if m.optCursor < total-1 {
				m.optCursor++
			} else {
				m.nextField()
			}
		} else {
			m.nextField()
		}
		return m, nil

	case "shift+tab", "up":
		if m.focusField == fldOptions {
			if m.optCursor > 0 {
				m.optCursor--
			} else {
				m.prevField()
			}
		} else {
			m.prevField()
		}
		return m, nil

	case "enter":
		if m.focusField == fldOptions {
			if m.optCursor < len(m.options) {
				return m.openOptionEditor(m.optCursor)
			}
			return m.openOptionEditor(-1)
		}
		m.nextField()
		return m, nil

	case " ":
		if m.focusField == fldOptions && m.optCursor < len(m.options) {
			m.options[m.optCursor].Enabled = !m.options[m.optCursor].Enabled
			return m, nil
		}
		return m.updateInput(key)

	case "d", "delete":
		if m.focusField == fldOptions {
			if m.optCursor < len(m.options) {
				m.options = append(m.options[:m.optCursor], m.options[m.optCursor+1:]...)
				if m.optCursor > 0 && m.optCursor >= len(m.options) {
					m.optCursor--
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
	if m.focusField == fldOptions {
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
		o = m.options[idx]
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
			m.options = append(m.options, o)
			m.optCursor = len(m.options) - 1
		} else {
			m.options[m.optEditor.editIdx] = o
		}
		m.optEditor = nil
		m.focusField = fldOptions
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
	host := strings.TrimSpace(m.inputs[fldHost].Value())
	socket := strings.TrimSpace(m.inputs[fldSocket].Value())

	if name == "" {
		m.err = "name is required"
		m.focusField = fldName
		m.applyFocus()
		return m, nil
	}
	if host == "" && socket == "" {
		m.err = "host or socket is required"
		m.focusField = fldHost
		m.applyFocus()
		return m, nil
	}

	port := 0
	if host != "" {
		portStr := strings.TrimSpace(m.inputs[fldPort].Value())
		if portStr == "" {
			portStr = "3306"
		}
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			m.err = "port must be 1–65535"
			m.focusField = fldPort
			m.applyFocus()
			return m, nil
		}
		port = p
	}

	var tags []string
	seen := make(map[string]struct{})
	for _, t := range strings.Split(m.inputs[fldTags].Value(), ",") {
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
	r.Host = host
	r.Port = port
	r.Socket = socket
	r.User = strings.TrimSpace(m.inputs[fldUser].Value())
	r.Password = m.inputs[fldPassword].Value()
	r.Database = strings.TrimSpace(m.inputs[fldDatabase].Value())
	r.Options = m.options
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
	for _, f := range []int{fldHost, fldPort, fldSocket, fldUser, fldPassword, fldDatabase} {
		m.inputs[f].Width = inputW
		sb.WriteString(m.renderField(f, inputW))
	}

	sb.WriteString("\n" + frmSectionStyle.Render("Options") + "\n")
	sb.WriteString(m.renderOptions())

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
	if m.focusField == fldOptions {
		sb.WriteString(frmHelpStyle.Render("  tab/↑↓ navigate   space toggle   enter edit   d delete   ctrl+s save   esc cancel"))
	} else {
		sb.WriteString(frmHelpStyle.Render("  tab/↑↓ navigate   ctrl+s save   esc cancel"))
	}

	return renderModal("◆", title, innerW, sb.String())
}

// renderOptions draws the checkbox-style options list plus a trailing
// "+ Add option" row, mirroring the focus/cursor conventions of the rest of
// the form.
func (m recordFormModel) renderOptions() string {
	inOpts := m.focusField == fldOptions
	var sb strings.Builder

	if len(m.options) == 0 {
		sb.WriteString(frmDimStyle.Italic(true).Render("  No options configured") + "\n")
	}
	for i, o := range m.options {
		selected := inOpts && m.optCursor == i
		box := "[ ]"
		if o.Enabled {
			box = "[x]"
		}
		label := o.Key
		if o.Value != "" {
			label += " = " + o.Value
		}
		row := fmt.Sprintf("%s %s", box, label)
		if selected {
			sb.WriteString(lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("  ❯ "+row) +
				frmDimStyle.Render("   space toggle  enter edit  d delete") + "\n")
		} else if o.Enabled {
			sb.WriteString(lipgloss.NewStyle().Foreground(clrText).Render("    "+row) + "\n")
		} else {
			sb.WriteString(frmDimStyle.Render("    "+row) + "\n")
		}
	}

	addSelected := inOpts && m.optCursor == len(m.options)
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
