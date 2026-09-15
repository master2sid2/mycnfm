package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mycnfm/internal/config"
	"mycnfm/internal/mycnf"
	"mycnfm/internal/version"
)

type view int

const (
	viewList view = iota
	viewRecordForm
	viewFolderForm
	viewMove
	viewConfirmDelete
	viewConfirmClean
)

type modalKind int

const (
	noModal modalKind = iota
	modalHelp
)

type App struct {
	store *config.Store
	list  listModel

	recordForm recordFormModel
	folderForm folderFormModel
	movePicker movePickerModel

	currentView view
	activeModal modalKind

	// Navigation stack — each entry is the folder we stepped into. Empty = root (ENV list).
	folderStack []config.Folder

	groupView bool // false = browse by folder (ENV) tree; true = flatten & cluster by Group label
	sortMode  sortMode
	sortDesc  bool

	filtering   bool
	filterInput textinput.Model
	filterQuery string

	revealPassword bool

	// Delete state.
	deleteTarget     string
	deleteIsFolder   bool
	deleteTargetName string

	myCnfPath string

	width       int
	height      int
	statusMsg   string
	statusIsErr bool
}

func New(store *config.Store, myCnfPath string, statusMsg string, statusIsErr bool) *App {
	fi := textinput.New()
	fi.Placeholder = "search this env…"
	fi.Prompt = "🔍 "
	fi.PromptStyle = lipgloss.NewStyle().Foreground(clrFocus)
	fi.TextStyle = lipgloss.NewStyle().Foreground(clrText)
	fi.CharLimit = 128

	a := &App{
		store:       store,
		myCnfPath:   myCnfPath,
		statusMsg:   statusMsg,
		statusIsErr: statusIsErr,
		filterInput: fi,
	}
	a.list = newListModel()
	a.refreshList()
	return a
}

// currentFolderID returns the ID of the folder we are currently inside ("" = root).
func (a *App) currentFolderID() string {
	if len(a.folderStack) == 0 {
		return ""
	}
	return a.folderStack[len(a.folderStack)-1].ID
}

// rootFolderID returns the ID of the top-level ENV we are currently under
// ("" if we're browsing at the root, meaning "search everything").
func (a *App) rootFolderID() string {
	if len(a.folderStack) == 0 {
		return ""
	}
	return a.folderStack[0].ID
}

func (a *App) Init() tea.Cmd { return nil }

// ── Entry assembly ────────────────────────────────────────────────────────────

// refreshList rebuilds the visible entries for the current mode (folder / group / filter).
func (a *App) refreshList() {
	var entries []listEntry
	switch {
	case a.filterQuery != "":
		entries = a.buildFilteredEntries()
	case a.groupView:
		entries = a.buildGroupViewEntries()
	default:
		entries = a.buildFolderViewEntries()
	}
	a.list.setEntries(entries)
}

func (a *App) buildFolderViewEntries() []listEntry {
	fid := a.currentFolderID()
	folders := a.store.FoldersInFolder(fid)
	records := a.store.RecordsInFolder(fid)

	var entries []listEntry
	for _, f := range folders {
		entries = append(entries, listEntry{kind: entryFolder, folder: f})
	}

	ungrouped, grouped, groupNames := clusterByGroup(records)

	sortRecords(ungrouped, a.sortMode, a.sortDesc)
	for _, r := range ungrouped {
		entries = append(entries, a.recordEntry(r, ""))
	}

	for _, gname := range groupNames {
		grp := grouped[gname]
		sortRecords(grp, a.sortMode, a.sortDesc)
		entries = append(entries, listEntry{kind: entryHeader, header: gname, count: len(grp)})
		for _, r := range grp {
			entries = append(entries, a.recordEntry(r, ""))
		}
	}
	return entries
}

func (a *App) buildGroupViewEntries() []listEntry {
	records := a.store.RecordsUnder(a.currentFolderID())
	ungrouped, grouped, groupNames := clusterByGroup(records)

	var entries []listEntry
	for _, gname := range groupNames {
		grp := grouped[gname]
		sortRecords(grp, a.sortMode, a.sortDesc)
		entries = append(entries, listEntry{kind: entryHeader, header: gname, count: len(grp)})
		for _, r := range grp {
			entries = append(entries, a.recordEntry(r, a.pathLabel(r.FolderID)))
		}
	}

	if len(ungrouped) > 0 {
		sortRecords(ungrouped, a.sortMode, a.sortDesc)
		entries = append(entries, listEntry{kind: entryHeader, header: "(ungrouped)", count: len(ungrouped)})
		for _, r := range ungrouped {
			entries = append(entries, a.recordEntry(r, a.pathLabel(r.FolderID)))
		}
	}
	return entries
}

func (a *App) buildFilteredEntries() []listEntry {
	records := a.store.RecordsUnder(a.rootFolderID())
	q := strings.ToLower(a.filterQuery)

	var matched []config.Record
	for _, r := range records {
		hay := strings.ToLower(strings.Join([]string{
			r.Name, r.Host, r.Socket, r.User, r.Database, r.Group,
			strings.Join(r.Tags, " "), a.pathLabel(r.FolderID),
		}, " "))
		if strings.Contains(hay, q) {
			matched = append(matched, r)
		}
	}
	sortRecords(matched, a.sortMode, a.sortDesc)

	entries := make([]listEntry, 0, len(matched))
	for _, r := range matched {
		entries = append(entries, a.recordEntry(r, a.pathLabel(r.FolderID)))
	}
	return entries
}

func (a *App) recordEntry(r config.Record, path string) listEntry {
	return listEntry{
		kind:     entryRecord,
		record:   r,
		path:     path,
		isActive: r.ID == a.store.ActiveID(),
	}
}

// pathLabel renders a folder's ancestry as "env / sub / sub2" for display in
// flattened (group / filtered) views where the tree isn't otherwise visible.
func (a *App) pathLabel(folderID string) string {
	path := a.store.FolderPath(folderID)
	if len(path) == 0 {
		return ""
	}
	names := make([]string, len(path))
	for i, f := range path {
		names[i] = f.Name
	}
	return strings.Join(names, "/")
}

// clusterByGroup splits records into an ungrouped slice and a map of
// group-name → records, plus the group names in sorted order.
func clusterByGroup(records []config.Record) (ungrouped []config.Record, grouped map[string][]config.Record, groupNames []string) {
	grouped = make(map[string][]config.Record)
	for _, r := range records {
		if r.Group == "" {
			ungrouped = append(ungrouped, r)
		} else {
			grouped[r.Group] = append(grouped[r.Group], r)
		}
	}
	for g := range grouped {
		groupNames = append(groupNames, g)
	}
	sortStrings(groupNames)
	return
}

// ── Update ────────────────────────────────────────────────────────────────────

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		a.width, a.height = wsm.Width, wsm.Height
		a.recomputeListSize()
		a.recordForm.width = wsm.Width
		a.folderForm.width = wsm.Width
		a.movePicker.width = wsm.Width
		a.movePicker.height = wsm.Height
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return a, tea.Quit
	}

	switch msg := msg.(type) {
	case recordSavedMsg:
		var err error
		if a.recordForm.isEdit {
			err = a.store.Update(msg.record)
		} else {
			err = a.store.Add(msg.record)
		}
		if err != nil {
			a.setErr("Save error: " + err.Error())
		} else {
			a.setOK("Saved: " + msg.record.Name)
		}
		a.refreshList()
		a.currentView = viewList
		return a, nil

	case recordCancelledMsg:
		a.currentView = viewList
		return a, nil

	case folderSavedMsg:
		var err error
		if a.folderForm.isEdit {
			err = a.store.UpdateFolder(msg.folder)
		} else {
			err = a.store.AddFolder(msg.folder)
		}
		if err != nil {
			a.setErr("Save error: " + err.Error())
		} else {
			a.setOK("Saved: " + msg.folder.Name)
		}
		a.refreshList()
		a.currentView = viewList
		return a, nil

	case folderCancelledMsg:
		a.currentView = viewList
		return a, nil

	case moveDoneMsg:
		var err error
		if msg.isFolder {
			err = a.store.MoveFolder(msg.id, msg.targetFolderID)
		} else {
			err = a.store.MoveRecord(msg.id, msg.targetFolderID)
		}
		if err != nil {
			a.setErr("Move error: " + err.Error())
		} else {
			dest := "root"
			if msg.targetFolderID != "" {
				if f, ok := a.store.GetFolder(msg.targetFolderID); ok {
					dest = f.Name
				}
			}
			a.setOK(fmt.Sprintf("Moved to %s", dest))
		}
		a.refreshList()
		a.currentView = viewList
		return a, nil

	case moveCancelledMsg:
		a.currentView = viewList
		return a, nil
	}

	if a.activeModal != noModal {
		return a.updateModal(msg)
	}

	switch a.currentView {
	case viewList:
		if key, ok := msg.(tea.KeyMsg); ok {
			return a.updateList(key)
		}
		return a, nil
	case viewRecordForm:
		newModel, cmd := a.recordForm.Update(msg)
		if fm, ok := newModel.(recordFormModel); ok {
			a.recordForm = fm
		}
		return a, cmd
	case viewFolderForm:
		newModel, cmd := a.folderForm.Update(msg)
		if ff, ok := newModel.(folderFormModel); ok {
			a.folderForm = ff
		}
		return a, cmd
	case viewMove:
		newModel, cmd := a.movePicker.Update(msg)
		if mp, ok := newModel.(movePickerModel); ok {
			a.movePicker = mp
		}
		return a, cmd
	case viewConfirmDelete:
		if key, ok := msg.(tea.KeyMsg); ok {
			return a.updateConfirmDelete(key)
		}
	case viewConfirmClean:
		if key, ok := msg.(tea.KeyMsg); ok {
			return a.updateConfirmClean(key)
		}
	}

	return a, nil
}

func (a *App) setOK(msg string)  { a.statusMsg, a.statusIsErr = msg, false }
func (a *App) setErr(msg string) { a.statusMsg, a.statusIsErr = msg, true }

func (a *App) recomputeListSize() {
	leftW := a.width/2 - 2
	h := a.height - headerH - footerH - 2
	if a.filtering {
		h--
	}
	a.list.setSize(leftW, h)
}

// ── List view key handling ───────────────────────────────────────────────────

func (a *App) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.filtering {
		return a.updateFiltering(msg)
	}

	switch msg.String() {
	case "q":
		return a, tea.Quit

	case "ctrl+r":
		a.revealPassword = !a.revealPassword
		return a, nil

	case "esc":
		switch {
		case a.groupView:
			a.groupView = false
			a.refreshList()
		case len(a.folderStack) > 0:
			a.folderStack = a.folderStack[:len(a.folderStack)-1]
			a.refreshList()
		}
		return a, nil

	case "enter":
		e, ok := a.list.selectedEntry()
		if !ok {
			return a, nil
		}
		if e.kind == entryFolder {
			a.folderStack = append(a.folderStack, e.folder)
			a.refreshList()
			return a, nil
		}
		if e.kind == entryRecord {
			a.applyRecord(e.record)
		}
		return a, nil

	case "n":
		r := config.NewRecord()
		r.FolderID = a.currentFolderID()
		a.recordForm = newRecordFormModel(r, false, a.store)
		a.recordForm.width = a.width
		a.currentView = viewRecordForm
		return a, a.recordForm.Init()

	case "g":
		f := config.NewFolder(a.currentFolderID())
		a.folderForm = newFolderFormModel(f, false, len(a.folderStack) == 0)
		a.folderForm.width = a.width
		a.currentView = viewFolderForm
		return a, a.folderForm.Init()

	case "e":
		e, ok := a.list.selectedEntry()
		if !ok {
			return a, nil
		}
		switch e.kind {
		case entryFolder:
			a.folderForm = newFolderFormModel(e.folder, true, e.folder.ParentID == "")
			a.folderForm.width = a.width
			a.currentView = viewFolderForm
			return a, a.folderForm.Init()
		case entryRecord:
			a.recordForm = newRecordFormModel(e.record, true, a.store)
			a.recordForm.width = a.width
			a.currentView = viewRecordForm
			return a, a.recordForm.Init()
		}
		return a, nil

	case "m":
		e, ok := a.list.selectedEntry()
		if !ok || e.kind == entryHeader {
			return a, nil
		}
		if e.kind == entryFolder {
			a.movePicker = newMovePicker(a.store, e.folder.ID, true, e.folder.Name)
		} else {
			a.movePicker = newMovePicker(a.store, e.record.ID, false, e.record.Name)
		}
		a.movePicker.width = a.width
		a.movePicker.height = a.height
		a.currentView = viewMove
		return a, nil

	case "d":
		e, ok := a.list.selectedEntry()
		if !ok || e.kind == entryHeader {
			return a, nil
		}
		if e.kind == entryFolder {
			a.deleteTarget, a.deleteTargetName, a.deleteIsFolder = e.folder.ID, e.folder.Name, true
		} else {
			a.deleteTarget, a.deleteTargetName, a.deleteIsFolder = e.record.ID, e.record.Name, false
		}
		a.currentView = viewConfirmDelete
		return a, nil

	case "x":
		a.currentView = viewConfirmClean
		return a, nil

	case "v":
		a.groupView = !a.groupView
		a.refreshList()
		return a, nil

	case "s":
		a.sortMode = (a.sortMode + 1) % sortModeCount
		a.refreshList()
		return a, nil

	case "S":
		a.sortDesc = !a.sortDesc
		a.refreshList()
		return a, nil

	case "/":
		a.filtering = true
		a.filterInput.SetValue(a.filterQuery)
		a.filterInput.Focus()
		a.recomputeListSize()
		return a, textinput.Blink

	case "?":
		a.activeModal = modalHelp
		return a, nil

	default:
		return a, a.list.update(msg)
	}
}

func (a *App) updateFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.filtering = false
		a.filterQuery = ""
		a.filterInput.SetValue("")
		a.recomputeListSize()
		a.refreshList()
		return a, nil

	case "enter":
		e, ok := a.list.selectedEntry()
		if ok && e.kind == entryRecord {
			a.applyRecord(e.record)
		}
		return a, nil

	case "up", "down", "pgup", "pgdown", "home", "end":
		return a, a.list.update(msg)

	default:
		var cmd tea.Cmd
		a.filterInput, cmd = a.filterInput.Update(msg)
		a.filterQuery = a.filterInput.Value()
		a.refreshList()
		return a, cmd
	}
}

func (a *App) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		var err error
		if a.deleteIsFolder {
			err = a.store.DeleteFolder(a.deleteTarget)
		} else {
			err = a.store.Delete(a.deleteTarget)
		}
		if err != nil {
			a.setErr("Delete error: " + err.Error())
		} else {
			a.setOK("Deleted: " + a.deleteTargetName)
		}
		a.refreshList()
		a.currentView = viewList
	case "n", "N", "esc":
		a.currentView = viewList
	}
	return a, nil
}

func (a *App) updateConfirmClean(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		a.cleanMyCnf()
		a.currentView = viewList
	case "n", "N", "esc":
		a.currentView = viewList
	}
	return a, nil
}

func (a *App) updateModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch a.activeModal {
	case modalHelp:
		if _, ok := msg.(tea.KeyMsg); ok {
			a.activeModal = noModal
		}
	}
	return a, nil
}

// ── Actions ───────────────────────────────────────────────────────────────────

func (a *App) applyRecord(r config.Record) {
	label := mycnf.Label(a.store, r)
	if err := mycnf.Apply(a.myCnfPath, label, r); err != nil {
		a.setErr("Apply failed: " + err.Error())
		return
	}
	if err := a.store.SetActiveID(r.ID); err != nil {
		a.setErr("Applied but failed to save state: " + err.Error())
		return
	}
	a.setOK("Applied: " + label)
	a.refreshList()
}

func (a *App) cleanMyCnf() {
	if err := mycnf.Clean(a.myCnfPath); err != nil {
		a.setErr("Clean failed: " + err.Error())
		return
	}
	if err := a.store.ClearActiveID(); err != nil {
		a.setErr("Cleaned but failed to save state: " + err.Error())
		return
	}
	a.setOK("~/.my.cnf cleaned — no credentials stored")
	a.refreshList()
}

// ── Layout / palette ──────────────────────────────────────────────────────────

const (
	headerH = 1
	footerH = 1
)

var (
	clrAccent  = lipgloss.Color("37")
	clrFocus   = lipgloss.Color("51")
	clrSuccess = lipgloss.Color("84")
	clrError   = lipgloss.Color("203")
	clrText    = lipgloss.Color("253")
	clrDim     = lipgloss.Color("245")
	clrMuted   = lipgloss.Color("239")
	clrHdrBg   = lipgloss.Color("234")
)

var (
	hdrAppStyle = lipgloss.NewStyle().
			Background(clrHdrBg).
			Foreground(clrFocus).
			Bold(true).
			Padding(0, 2)

	hdrVerStyle = lipgloss.NewStyle().
			Background(clrHdrBg).
			Foreground(clrDim).
			Padding(0, 2)

	hdrOkStyle = lipgloss.NewStyle().
			Background(clrHdrBg).
			Foreground(clrSuccess).
			Padding(0, 2)

	hdrErrStyle = lipgloss.NewStyle().
			Background(clrHdrBg).
			Foreground(clrError).
			Padding(0, 2)

	hdrActiveStyle = lipgloss.NewStyle().
			Background(clrHdrBg).
			Foreground(clrSuccess).
			Padding(0, 2)

	hdrGapStyle = lipgloss.NewStyle().Background(clrHdrBg)

	hdrBreadStyle = lipgloss.NewStyle().Background(clrHdrBg).Foreground(clrDim)

	hdrBreadSepStyle = lipgloss.NewStyle().Background(clrHdrBg).Foreground(clrMuted)

	hdrBreadActiveStyle = lipgloss.NewStyle().Background(clrHdrBg).Foreground(clrText)

	footerBgStyle  = lipgloss.NewStyle().Background(clrHdrBg).Foreground(clrDim)
	footerKeyStyle = lipgloss.NewStyle().Background(clrHdrBg).Foreground(clrFocus).Bold(true)
)

var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(clrMuted)

	detailSectionStyle = lipgloss.NewStyle().Foreground(clrFocus).Bold(true)
	detailKeyStyle     = lipgloss.NewStyle().Width(12).Foreground(clrDim)
	detailValStyle     = lipgloss.NewStyle().Foreground(clrText)
	detailDimStyle     = lipgloss.NewStyle().Foreground(clrDim)
	detailActiveStyle  = lipgloss.NewStyle().Foreground(clrSuccess).Bold(true)
	emptyHintStyle     = lipgloss.NewStyle().Foreground(clrDim).Italic(true).Padding(2, 2)
)

// ── View ──────────────────────────────────────────────────────────────────────

func (a *App) View() string {
	bg := a.listView()
	modal := a.modalContent()
	if modal == "" {
		return bg
	}
	return centerOverlay(bg, modal)
}

func (a *App) modalContent() string {
	switch a.currentView {
	case viewRecordForm:
		return a.recordForm.View()
	case viewFolderForm:
		return a.folderForm.View()
	case viewMove:
		return a.movePicker.View()
	case viewConfirmDelete:
		return confirmDeleteView(a.deleteTargetName, a.deleteIsFolder)
	case viewConfirmClean:
		return confirmCleanView(a.myCnfPath)
	}
	switch a.activeModal {
	case modalHelp:
		return helpModalView()
	}
	return ""
}

func (a *App) listView() string {
	appS := hdrAppStyle.Render("🗄  mycnfm")
	breadS := a.breadcrumb()

	var rightS string
	switch {
	case a.statusMsg != "" && a.statusIsErr:
		rightS = hdrErrStyle.Render("✗  " + a.statusMsg)
	case a.statusMsg != "":
		rightS = hdrOkStyle.Render("✓  " + a.statusMsg)
	case a.store.ActiveID() != "":
		if r, ok := a.store.Get(a.store.ActiveID()); ok {
			rightS = hdrActiveStyle.Render("●  " + mycnf.Label(a.store, r))
		} else {
			rightS = hdrVerStyle.Render("v" + version.Version)
		}
	default:
		rightS = hdrVerStyle.Render("v" + version.Version)
	}

	leftSide := lipgloss.JoinHorizontal(lipgloss.Top, appS, breadS)
	gapW := max(0, a.width-lipgloss.Width(leftSide)-lipgloss.Width(rightS))
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		leftSide,
		hdrGapStyle.Width(gapW).Render(""),
		rightS,
	)

	leftW := a.width/2 - 2
	rightW := a.width - leftW - 5
	contentH := a.height - headerH - footerH - 2

	leftContent := a.list.view()
	if a.filtering {
		leftContent = a.filterInput.View() + "\n" + leftContent
	}
	left := panelStyle.Width(leftW).Height(contentH).Render(leftContent)
	right := panelStyle.Width(rightW).Height(contentH).Render(a.detailsView(rightW - 4))
	panels := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	footer := a.helpBar()

	return header + "\n" + panels + "\n" + footer
}

func (a *App) breadcrumb() string {
	if a.groupView {
		return hdrBreadSepStyle.Render("  ›  ") + hdrBreadActiveStyle.Render("grouped by category")
	}
	if len(a.folderStack) == 0 {
		return ""
	}
	sep := hdrBreadSepStyle.Render("  ›  ")
	var parts []string
	for i, f := range a.folderStack {
		if i == len(a.folderStack)-1 {
			parts = append(parts, hdrBreadActiveStyle.Render(f.Name))
		} else {
			parts = append(parts, hdrBreadStyle.Render(f.Name))
		}
	}
	return sep + strings.Join(parts, sep)
}

func (a *App) helpBar() string {
	arrow := "↑"
	if a.sortDesc {
		arrow = "↓"
	}
	viewLabel := "by folder"
	if a.groupView {
		viewLabel = "by group"
	}

	pairs := []string{
		"↑↓", "navigate",
		"enter", "apply/open",
		"n", "new host",
		"g", "new env/folder",
		"e", "edit",
		"m", "move",
		"d", "delete",
		"v", viewLabel,
		"s", fmt.Sprintf("sort:%s%s", a.sortMode.label(), arrow),
		"/", "search",
		"x", "clean",
		"?", "help",
	}
	if a.filtering {
		pairs = []string{"type", "search", "↑↓", "navigate", "enter", "apply", "esc", "clear"}
	} else if len(a.folderStack) > 0 || a.groupView {
		pairs = append(pairs, "esc", "back")
	} else {
		pairs = append(pairs, "q", "quit")
	}
	return renderHelpBar(a.width, pairs...)
}

func renderHelpBar(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts,
			footerKeyStyle.Render("  "+pairs[i]),
			footerBgStyle.Render(" "+pairs[i+1]+"  "),
		)
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	padW := max(0, width-lipgloss.Width(bar))
	return bar + footerBgStyle.Width(padW).Render("")
}

func (a *App) detailsView(width int) string {
	e, ok := a.list.selectedEntry()
	if !ok || e.kind == entryHeader {
		return emptyHintStyle.Render("Select an item\nto view details")
	}

	if e.kind == entryFolder {
		f := e.folder
		var sb strings.Builder
		sb.WriteString(detailSectionStyle.Render("▸ "+f.Name) + "\n\n")
		kind := "subfolder"
		if f.ParentID == "" {
			kind = "ENV"
		}
		sb.WriteString(detailKeyStyle.Render("Type") + detailValStyle.Render(kind) + "\n")

		sub := a.store.FoldersInFolder(f.ID)
		rec := a.store.RecordsInFolder(f.ID)
		total := len(sub) + len(rec)
		sb.WriteString(detailKeyStyle.Render("Contents") +
			detailValStyle.Render(fmt.Sprintf("%d item(s)", total)) + "\n")
		if len(sub) > 0 {
			sb.WriteString(detailKeyStyle.Render("Subfolders") +
				detailValStyle.Render(fmt.Sprintf("%d", len(sub))) + "\n")
		}
		if len(rec) > 0 {
			sb.WriteString(detailKeyStyle.Render("Hosts") +
				detailValStyle.Render(fmt.Sprintf("%d", len(rec))) + "\n")
		}
		return sb.String()
	}

	r := e.record
	var sb strings.Builder
	title := r.Name
	if r.ID == a.store.ActiveID() {
		title += "  " + detailActiveStyle.Render("● ACTIVE")
	}
	sb.WriteString(detailSectionStyle.Render(title) + "\n\n")

	rows := [][2]string{}
	if r.Group != "" {
		rows = append(rows, [2]string{"Group", r.Group})
	}
	if path := a.pathLabel(r.FolderID); path != "" {
		rows = append(rows, [2]string{"Path", path})
	}
	if r.Host != "" {
		rows = append(rows, [2]string{"Host", fmt.Sprintf("%s:%d", r.Host, r.Port)})
	}
	if r.Socket != "" {
		rows = append(rows, [2]string{"Socket", r.Socket})
	}
	rows = append(rows, [2]string{"User", r.User})

	pw := "(none)"
	if r.Password != "" {
		if a.revealPassword {
			pw = r.Password
		} else {
			pw = "••••••••"
		}
	}
	rows = append(rows, [2]string{"Password", pw})

	if r.Database != "" {
		rows = append(rows, [2]string{"Database", r.Database})
	}
	for _, o := range r.Options {
		if !o.Enabled || o.Key == "" {
			continue
		}
		v := o.Value
		if v == "" {
			v = "(flag)"
		}
		rows = append(rows, [2]string{o.Key, v})
	}

	for _, row := range rows {
		sb.WriteString(detailKeyStyle.Render(row[0]) + detailValStyle.Render(row[1]) + "\n")
	}

	if len(r.Tags) > 0 {
		sb.WriteString("\n" + detailSectionStyle.Render("Tags") + "\n  " + renderTagChips(r.Tags) + "\n")
	}

	if r.Notes != "" {
		sb.WriteString("\n" + detailSectionStyle.Render("Notes") + "\n")
		sb.WriteString(detailDimStyle.Render(r.Notes) + "\n")
	}

	sb.WriteString("\n" + detailDimStyle.Render("ctrl+r reveal/hide password") + "\n")

	return sb.String()
}

func confirmDeleteView(name string, isFolder bool) string {
	label := "host"
	warning := ""
	if isFolder {
		label = "folder"
		warning = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).
			Render("  All contents will be deleted.")
	}
	const panelW = 40
	var sb strings.Builder
	sb.WriteString(
		lipgloss.NewStyle().Foreground(clrText).
			Render(fmt.Sprintf("Delete %s %q?", label, name)) +
			warning +
			"\n\n" +
			lipgloss.NewStyle().Foreground(clrError).Bold(true).Render("[y]") +
			lipgloss.NewStyle().Foreground(clrText).Render(" yes   ") +
			lipgloss.NewStyle().Foreground(clrDim).Bold(true).Render("[n]") +
			lipgloss.NewStyle().Foreground(clrText).Render(" no"),
	)
	return renderModal("✗", "Confirm Delete", panelW, sb.String())
}

func confirmCleanView(path string) string {
	const panelW = 46
	var sb strings.Builder
	sb.WriteString(
		lipgloss.NewStyle().Foreground(clrText).Render("Wipe "+path+"?") + "\n" +
			lipgloss.NewStyle().Foreground(clrDim).Render("  No credentials will be left on disk.") +
			"\n\n" +
			lipgloss.NewStyle().Foreground(clrError).Bold(true).Render("[y]") +
			lipgloss.NewStyle().Foreground(clrText).Render(" yes   ") +
			lipgloss.NewStyle().Foreground(clrDim).Bold(true).Render("[n]") +
			lipgloss.NewStyle().Foreground(clrText).Render(" no"),
	)
	return renderModal("⌫", "Clean ~/.my.cnf", panelW, sb.String())
}

func helpModalView() string {
	const (
		panelW = 56
		colW   = 24
		kw     = 8
	)

	colGap := "  "
	sep := frmSepStyle.Render(strings.Repeat("─", colW))
	keyStyle := lipgloss.NewStyle().Width(kw).Foreground(clrFocus).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(clrText)

	row := func(k, v string) string {
		return "  " + keyStyle.Render(k) + valStyle.Render(v) + "\n"
	}

	var left strings.Builder
	left.WriteString(frmSectionStyle.Render("Navigation") + "\n")
	left.WriteString(sep + "\n")
	left.WriteString(row("↑/↓", "navigate"))
	left.WriteString(row("enter", "open / apply"))
	left.WriteString(row("esc", "go up / back"))
	left.WriteString(row("/", "search this ENV"))
	left.WriteString(row("v", "toggle folder/group view"))
	left.WriteString(row("s / S", "cycle sort / reverse"))
	left.WriteString(row("q", "quit"))

	var right strings.Builder
	right.WriteString(frmSectionStyle.Render("Records") + "\n")
	right.WriteString(sep + "\n")
	right.WriteString(row("n", "new host"))
	right.WriteString(row("e", "edit"))
	right.WriteString(row("m", "move"))
	right.WriteString(row("d", "delete"))
	right.WriteString(row("ctrl+r", "reveal password"))
	right.WriteString("\n")
	right.WriteString(frmSectionStyle.Render("Folders / cleanup") + "\n")
	right.WriteString(sep + "\n")
	right.WriteString(row("g", "new ENV / subfolder"))
	right.WriteString(row("x", "clean ~/.my.cnf"))

	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(colW).Render(left.String()),
		colGap,
		lipgloss.NewStyle().Width(colW).Render(right.String()),
	)

	var sb strings.Builder
	sb.WriteString(cols + "\n\n")
	sb.WriteString(frmSepStyle.Render(strings.Repeat("─", panelW-6)) + "\n")
	sb.WriteString(frmHelpStyle.Render("  (press any key to close)"))

	return renderModal("?", "Keyboard Shortcuts", panelW, sb.String())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run is the main event loop for the TUI.
func Run(ctx context.Context, store *config.Store) error {
	path, err := mycnf.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve ~/.my.cnf: %w", err)
	}

	app := New(store, path, "", false)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
