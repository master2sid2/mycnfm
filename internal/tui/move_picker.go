package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/master2sid2/mycnfm/internal/config"
)

type moveDoneMsg struct {
	id             string
	isFolder       bool
	targetFolderID string // "" = root
}

type moveCancelledMsg struct{}

type pickerRow struct {
	isMoveHere bool
	folder     config.Folder
}

// movePickerModel lets the user navigate the folder tree to choose a move target.
type movePickerModel struct {
	store          *config.Store
	movingID       string
	movingIsFolder bool
	movingName     string
	stack          []config.Folder
	rows           []pickerRow
	cursor         int
	width          int
	height         int
}

func newMovePicker(store *config.Store, id string, isFolder bool, name string) movePickerModel {
	var parentID string
	if isFolder {
		if f, ok := store.GetFolder(id); ok {
			parentID = f.ParentID
		}
	} else {
		if r, ok := store.Get(id); ok {
			parentID = r.FolderID
		}
	}

	m := movePickerModel{
		store:          store,
		movingID:       id,
		movingIsFolder: isFolder,
		movingName:     name,
		stack:          store.FolderPath(parentID),
	}
	m.rebuild()
	return m
}

func (m *movePickerModel) rebuild() {
	parentID := ""
	if len(m.stack) > 0 {
		parentID = m.stack[len(m.stack)-1].ID
	}

	m.rows = []pickerRow{{isMoveHere: true}}

	for _, f := range m.store.FoldersInFolder(parentID) {
		if m.movingIsFolder && m.isAncestorOrSelf(f.ID) {
			continue
		}
		m.rows = append(m.rows, pickerRow{folder: f})
	}

	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

// isAncestorOrSelf returns true if folderID equals movingID or is a descendant of it.
func (m *movePickerModel) isAncestorOrSelf(folderID string) bool {
	if folderID == m.movingID {
		return true
	}
	cur := folderID
	for {
		f, ok := m.store.GetFolder(cur)
		if !ok || f.ParentID == "" {
			return false
		}
		if f.ParentID == m.movingID {
			return true
		}
		cur = f.ParentID
	}
}

func (m movePickerModel) currentTargetID() string {
	if len(m.stack) == 0 {
		return ""
	}
	return m.stack[len(m.stack)-1].ID
}

func (m movePickerModel) Init() tea.Cmd { return nil }

func (m movePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = wsm.Width, wsm.Height
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		if len(m.stack) > 0 {
			m.stack = m.stack[:len(m.stack)-1]
			m.cursor = 0
			m.rebuild()
		} else {
			return m, func() tea.Msg { return moveCancelledMsg{} }
		}

	case "up", "k", "shift+tab":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j", "tab":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}

	case "enter":
		if len(m.rows) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		if row.isMoveHere {
			tid := m.currentTargetID()
			return m, func() tea.Msg {
				return moveDoneMsg{id: m.movingID, isFolder: m.movingIsFolder, targetFolderID: tid}
			}
		}
		m.stack = append(m.stack, row.folder)
		m.cursor = 0
		m.rebuild()
	}

	return m, nil
}

var (
	mpSepStyle = lipgloss.NewStyle().Foreground(clrMuted)
	mpDimStyle = lipgloss.NewStyle().Foreground(clrDim)

	mpMoveHereSelStyle = lipgloss.NewStyle().Foreground(clrSuccess).Bold(true)
	mpMoveHereStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))

	mpFolderSelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("37")).
				Bold(true)
	mpFolderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	mpBreadStyle = lipgloss.NewStyle().Foreground(clrDim)
	mpBreadSep   = lipgloss.NewStyle().Foreground(clrMuted).Render("  ›  ")
)

func (m movePickerModel) View() string {
	innerW := m.width - 10
	if innerW < 36 {
		innerW = 36
	}
	if innerW > 68 {
		innerW = 68
	}

	title := fmt.Sprintf("Move %q to…", m.movingName)
	var sb strings.Builder

	if len(m.stack) > 0 {
		var parts []string
		parts = append(parts, mpBreadStyle.Render("root"))
		for _, f := range m.stack {
			parts = append(parts, mpBreadStyle.Render(f.Name))
		}
		sb.WriteString(mpDimStyle.Render("  ") + strings.Join(parts, mpBreadSep) + "\n")
	} else {
		sb.WriteString(mpDimStyle.Render("  root") + "\n")
	}
	sb.WriteString("\n")

	for i, row := range m.rows {
		sel := i == m.cursor
		cursor := "  "
		if sel {
			cursor = lipgloss.NewStyle().Foreground(clrFocus).Bold(true).Render("❯ ")
		}

		if row.isMoveHere {
			style := mpMoveHereStyle
			if sel {
				style = mpMoveHereSelStyle
			}
			sb.WriteString(cursor + style.Render("[ ✓ Move here ]") + "\n")
			continue
		}

		label := "▸ " + row.folder.Name
		rowW := innerW - 8
		style := mpFolderStyle
		if sel {
			style = mpFolderSelStyle.Width(rowW)
		}
		sb.WriteString(cursor + style.Render(label) + "\n")
	}

	if len(m.rows) == 0 {
		sb.WriteString(mpDimStyle.Render("  (empty folder)") + "\n")
	}

	sb.WriteString("\n" + mpSepStyle.Render(strings.Repeat("─", innerW-6)) + "\n")
	sb.WriteString(mpDimStyle.Render("  ↑↓ navigate   enter open/confirm   esc back / cancel"))

	return renderModal("→", title, innerW, sb.String())
}
