package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mycnfm/internal/config"
)

const (
	oFldKey = iota
	oFldValue
	oFldEnabled
	oFldCount
)

// optionEditorModel edits one checkbox-style [client] option (key, optional
// value, enabled toggle).
type optionEditorModel struct {
	key     textinput.Model
	value   textinput.Model
	enabled bool
	focused int
	isNew   bool
	editIdx int
	err     string
}

func newOptionEditor(o config.Option, isNew bool, idx int) optionEditorModel {
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
	k.SetValue(o.Key)
	v.SetValue(o.Value)

	enabled := o.Enabled
	if isNew {
		enabled = true
	}

	m := optionEditorModel{key: k, value: v, enabled: enabled, isNew: isNew, editIdx: idx}
	m.applyFocus()
	return m
}

func (m *optionEditorModel) activeInput() *textinput.Model {
	switch m.focused {
	case oFldKey:
		return &m.key
	case oFldValue:
		return &m.value
	}
	return nil
}

func (m *optionEditorModel) applyFocus() {
	m.key.Blur()
	m.value.Blur()
	if inp := m.activeInput(); inp != nil {
		inp.Focus()
	}
}

func (m optionEditorModel) update(key tea.KeyMsg) (optionEditorModel, tea.Cmd) {
	m.err = ""

	switch key.String() {
	case "tab", "down":
		m.focused = (m.focused + 1) % oFldCount
		m.applyFocus()
		return m, nil

	case "shift+tab", "up":
		m.focused = (m.focused - 1 + oFldCount) % oFldCount
		m.applyFocus()
		return m, nil

	case "left", "right", "enter", " ":
		if m.focused == oFldEnabled {
			m.enabled = !m.enabled
			return m, nil
		}
	}

	if inp := m.activeInput(); inp != nil {
		var cmd tea.Cmd
		*inp, cmd = inp.Update(key)
		return m, cmd
	}
	return m, nil
}

func (m optionEditorModel) getResult() (config.Option, bool) {
	key := strings.TrimSpace(m.key.Value())
	if key == "" {
		return config.Option{}, false
	}
	return config.Option{
		Key:     key,
		Value:   strings.TrimSpace(m.value.Value()),
		Enabled: m.enabled,
	}, true
}

func (m optionEditorModel) view(width int) string {
	innerW := width - 10
	if innerW < 44 {
		innerW = 44
	}
	if innerW > 72 {
		innerW = 72
	}
	inputW := innerW - frmLabelW - 8

	title := "Add Option"
	if !m.isNew {
		title = "Edit Option"
	}

	var sb strings.Builder

	for _, f := range []struct {
		label string
		fld   int
		inp   *textinput.Model
	}{
		{"Key", oFldKey, &m.key},
		{"Value", oFldValue, &m.value},
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

	lbl := frmLabelStyle.Render("Enabled")
	bar := frmDimStyle.Render("│")
	if m.focused == oFldEnabled {
		lbl = frmFocusLabelStyle.Render("Enabled")
		bar = frmBarStyle.Render("│")
	}
	box := "[ ]"
	if m.enabled {
		box = lipgloss.NewStyle().Foreground(clrSuccess).Bold(true).Render("[x]")
	}
	sb.WriteString(fmt.Sprintf("  %s %s %s\n", lbl, bar, box))

	if m.err != "" {
		sb.WriteString("\n" + frmErrStyle.Render("  ✗ "+m.err) + "\n")
	}

	sb.WriteString("\n" + frmSepStyle.Render(strings.Repeat("─", innerW-6)) + "\n")
	sb.WriteString(frmHelpStyle.Render("  tab navigate   space/enter toggle   ctrl+s save   esc cancel"))

	return renderModal("☑", title, innerW, sb.String())
}
