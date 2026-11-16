package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/master2sid2/mycnfm/internal/config"
)

type folderSavedMsg struct{ folder config.Folder }
type folderCancelledMsg struct{}

type folderFormModel struct {
	orig   config.Folder
	inp    textinput.Model
	isEdit bool
	isRoot bool // true = this is an ENV (root-level folder), false = a subfolder
	err    string
	width  int
}

func newFolderFormModel(f config.Folder, isEdit bool, isRoot bool) folderFormModel {
	t := textinput.New()
	if isRoot {
		t.Placeholder = "env name, e.g. stage"
	} else {
		t.Placeholder = "folder name"
	}
	t.CharLimit = 128
	t.PromptStyle = lipgloss.NewStyle().Foreground(clrAccent)
	t.TextStyle = lipgloss.NewStyle().Foreground(clrText)
	t.SetValue(f.Name)
	t.Focus()
	return folderFormModel{orig: f, inp: t, isEdit: isEdit, isRoot: isRoot}
}

func (m folderFormModel) Init() tea.Cmd { return textinput.Blink }

func (m folderFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return folderCancelledMsg{} }
	case "ctrl+s", "enter":
		name := strings.TrimSpace(m.inp.Value())
		if name == "" {
			m.err = "name is required"
			return m, nil
		}
		f := m.orig
		f.Name = name
		return m, func() tea.Msg { return folderSavedMsg{f} }
	default:
		var cmd tea.Cmd
		m.inp, cmd = m.inp.Update(key)
		return m, cmd
	}
}

func (m folderFormModel) View() string {
	innerW := m.width - 10
	if innerW < 36 {
		innerW = 36
	}
	if innerW > 60 {
		innerW = 60
	}
	m.inp.Width = innerW - frmLabelW - 8

	kind := "Folder"
	if m.isRoot {
		kind = "ENV"
	}
	title := "New " + kind
	if m.isEdit {
		title = "Rename " + kind
	}

	var sb strings.Builder
	lbl := frmLabelStyle.Render("Name")
	bar := frmBarStyle.Render("│")
	sb.WriteString("  " + lbl + " " + bar + " " + m.inp.View() + "\n")

	if m.err != "" {
		sb.WriteString("\n" + frmErrStyle.Render("  ✗ "+m.err) + "\n")
	}

	sb.WriteString("\n" + frmSepStyle.Render(strings.Repeat("─", innerW-6)) + "\n")
	sb.WriteString(frmHelpStyle.Render("  enter / ctrl+s save   esc cancel"))

	icon := "▸"
	if m.isRoot {
		icon = "⛁"
	}
	return renderModal(icon, title, innerW, sb.String())
}
