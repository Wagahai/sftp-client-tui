package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// syntheticDir is a fake fs.FileInfo used for virtual nav entries (.. and /).
type syntheticDir struct{ name string }

func (s syntheticDir) Name() string      { return s.name }
func (s syntheticDir) Size() int64       { return 0 }
func (s syntheticDir) Mode() fs.FileMode { return fs.ModeDir | 0755 }
func (s syntheticDir) ModTime() time.Time { return time.Time{} }
func (s syntheticDir) IsDir() bool       { return true }
func (s syntheticDir) Sys() interface{}  { return nil }

type PaneType int

const (
	PaneLocal  PaneType = iota
	PaneRemote PaneType = iota
)

type sortMode int

const (
	sortName sortMode = iota
	sortSize
	sortModified
)

type paneState int

const (
	paneNormal paneState = iota
	paneFilter
)

type FileEntry struct {
	Info     fs.FileInfo
	Selected bool
	nav      bool // true for synthetic .. and / entries
}

type FilePaneModel struct {
	ID         string
	PaneType   PaneType
	Dir        string
	entries    []FileEntry
	cursor     int
	offset     int
	width      int
	height     int
	focused    bool
	sort       sortMode
	showHidden bool
	state      paneState
	filter     textinput.Model
	filterStr  string
}

func NewFilePane(id string, pt PaneType, dir string) FilePaneModel {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.CharLimit = 80

	return FilePaneModel{
		ID:       id,
		PaneType: pt,
		Dir:      dir,
		filter:   ti,
	}
}

func (m *FilePaneModel) SetEntries(entries []fs.FileInfo) {
	m.entries = make([]FileEntry, 0, len(entries)+2)
	m.entries = append(m.entries,
		FileEntry{Info: syntheticDir{".."}, nav: true},
		FileEntry{Info: syntheticDir{"/"}, nav: true},
	)
	for _, e := range entries {
		if !m.showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		m.entries = append(m.entries, FileEntry{Info: e})
	}
	m.sortEntries()
	m.cursor = 0
	m.offset = 0
}

func (m *FilePaneModel) sortEntries() {
	// Nav entries always stay at the top; only sort the real entries.
	navCount := 0
	for _, e := range m.entries {
		if e.nav {
			navCount++
		}
	}
	sort.SliceStable(m.entries[navCount:], func(i, j int) bool {
		a, b := m.entries[navCount+i].Info, m.entries[navCount+j].Info
		// Directories always before files.
		if a.IsDir() != b.IsDir() {
			return a.IsDir()
		}
		switch m.sort {
		case sortSize:
			return a.Size() > b.Size()
		case sortModified:
			return a.ModTime().After(b.ModTime())
		default:
			return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
		}
	})
}

func (m *FilePaneModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *FilePaneModel) SetFocused(f bool) {
	m.focused = f
}

func (m FilePaneModel) CursorEntry() *FileEntry {
	visible := m.visibleEntries()
	if len(visible) == 0 {
		return nil
	}
	if m.cursor >= len(visible) {
		return nil
	}
	idx := visible[m.cursor]
	return &m.entries[idx]
}

func (m FilePaneModel) SelectedEntries() []FileEntry {
	var out []FileEntry
	for _, e := range m.entries {
		if e.Selected && !e.nav {
			out = append(out, e)
		}
	}
	// If nothing selected, return the cursor item (skip nav entries).
	if len(out) == 0 {
		if ce := m.CursorEntry(); ce != nil && !ce.nav {
			out = append(out, *ce)
		}
	}
	return out
}

func (m FilePaneModel) HasSelection() bool {
	for _, e := range m.entries {
		if e.Selected && !e.nav {
			return true
		}
	}
	return false
}

// visibleEntries returns indices into m.entries that pass the current filter.
// Nav entries (.. and /) are always included regardless of filter.
func (m FilePaneModel) visibleEntries() []int {
	var out []int
	f := strings.ToLower(m.filterStr)
	for i, e := range m.entries {
		if e.nav || f == "" || strings.Contains(strings.ToLower(e.Info.Name()), f) {
			out = append(out, i)
		}
	}
	return out
}

func (m *FilePaneModel) clampCursor(visible []int) {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if len(visible) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(visible) {
		m.cursor = len(visible) - 1
	}
}

func (m FilePaneModel) Update(msg tea.Msg) (FilePaneModel, tea.Cmd) {
	if m.state == paneFilter {
		return m.updateFilter(msg)
	}
	return m.updateNormal(msg)
}

func (m FilePaneModel) updateFilter(msg tea.Msg) (FilePaneModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.filterStr = ""
			m.filter.SetValue("")
			m.state = paneNormal
			m.cursor = 0
			return m, nil
		case "enter":
			m.filterStr = m.filter.Value()
			m.state = paneNormal
			m.cursor = 0
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.filterStr = m.filter.Value()
	m.cursor = 0
	return m, cmd
}

func (m FilePaneModel) updateNormal(msg tea.Msg) (FilePaneModel, tea.Cmd) {
	visible := m.visibleEntries()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(visible)-1 {
				m.cursor++
				listH := m.listHeight()
				if m.cursor >= m.offset+listH {
					m.offset = m.cursor - listH + 1
				}
			}
		case "pgup":
			m.cursor -= m.listHeight()
			if m.cursor < 0 {
				m.cursor = 0
			}
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		case "pgdown":
			m.cursor += m.listHeight()
			if m.cursor >= len(visible) {
				m.cursor = len(visible) - 1
			}
		case "home", "g":
			m.cursor = 0
			m.offset = 0
		case "end", "G":
			m.cursor = len(visible) - 1
			m.offset = max(0, m.cursor-m.listHeight()+1)
		case " ":
			if len(visible) > 0 && m.cursor < len(visible) {
				idx := visible[m.cursor]
				if !m.entries[idx].nav {
					m.entries[idx].Selected = !m.entries[idx].Selected
					// Advance cursor after select.
					if m.cursor < len(visible)-1 {
						m.cursor++
					}
				}
			}
		case "*":
			// Select all / deselect all toggle.
			anySelected := m.HasSelection()
			for i := range m.entries {
				m.entries[i].Selected = !anySelected
			}
		case "s":
			m.sort = (m.sort + 1) % 3
			m.sortEntries()
			m.cursor = 0
		case "h":
			m.showHidden = !m.showHidden
			// Re-apply entries without changing dir.
			// Caller must reload.
		case "/":
			m.state = paneFilter
			m.filter.Focus()
			return m, textinput.Blink
		case "esc":
			// Clear filter if active.
			if m.filterStr != "" {
				m.filterStr = ""
				m.filter.SetValue("")
				m.cursor = 0
			}
		}
	}

	m.clampCursor(visible)
	return m, nil
}

func (m FilePaneModel) listHeight() int {
	// height minus: border(2) + header(1) + path(1) + (filter line if active)
	h := m.height - 4
	if m.state == paneFilter {
		h--
	}
	if h < 1 {
		return 1
	}
	return h
}

func (m FilePaneModel) View() string {
	borderStyle := StyleBorderNormal
	if m.focused {
		borderStyle = StyleBorderFocus
	}

	inner := m.renderInner()
	return borderStyle.Width(m.width - 2).Height(m.height - 2).Render(inner)
}

func (m FilePaneModel) renderInner() string {
	w := m.width - 4 // account for border

	// Header line: pane type + path.
	paneLabel := "Local"
	if m.PaneType == PaneRemote {
		paneLabel = "Remote"
	}
	dirDisplay := TruncateStr(m.Dir, w-len(paneLabel)-3)
	header := StylePaneHeader.Width(w).Render(
		fmt.Sprintf("%s: %s", paneLabel, dirDisplay),
	)

	// Sort indicator.
	sortLabels := []string{"name", "size", "date"}
	sortInfo := StyleSubtle(" [sort:" + sortLabels[m.sort] + "]")
	if m.showHidden {
		sortInfo += StyleSubtle(" [hidden]")
	}
	subheader := lipgloss.NewStyle().Foreground(ColorSubtle).Width(w).Render(sortInfo)

	// Filter line.
	filterLine := ""
	if m.state == paneFilter {
		filterLine = StyleFilterPrompt.Render("/") + m.filter.View() + "\n"
	} else if m.filterStr != "" {
		filterLine = StyleFilterPrompt.Render("filter: "+m.filterStr) + "\n"
	}

	// File list.
	visible := m.visibleEntries()
	listH := m.listHeight()

	// Clamp offset.
	if m.offset > len(visible)-listH && len(visible) > listH {
		m.offset = len(visible) - listH
	}
	if m.offset < 0 {
		m.offset = 0
	}

	var rows []string
	end := m.offset + listH
	if end > len(visible) {
		end = len(visible)
	}

	for i := m.offset; i < end; i++ {
		idx := visible[i]
		e := m.entries[idx]
		isCursor := i == m.cursor
		rows = append(rows, m.renderEntry(e, isCursor, w))
	}

	// Pad remaining rows.
	for len(rows) < listH {
		rows = append(rows, strings.Repeat(" ", w))
	}

	// Scroll indicator.
	scrollInfo := ""
	if len(visible) > listH {
		scrollInfo = fmt.Sprintf(" %d/%d", m.cursor+1, len(visible))
	}
	footer := lipgloss.NewStyle().Foreground(ColorSubtle).Width(w).Render(scrollInfo)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		subheader,
		filterLine+strings.Join(rows, "\n"),
		footer,
	)
}

func StyleSubtle(s string) string {
	return lipgloss.NewStyle().Foreground(ColorSubtle).Render(s)
}

func (m FilePaneModel) renderEntry(e FileEntry, isCursor bool, w int) string {
	nameWidth := w - 14 // leave room for size + date
	if nameWidth < 10 {
		nameWidth = 10
	}

	name := e.Info.Name()
	if e.nav {
		if name == ".." {
			name = "../"
		}
		// "/" stays as "/"
	} else if e.Info.IsDir() {
		name += "/"
	}
	name = TruncateStr(name, nameWidth)
	name = PadRight(name, nameWidth)

	size := ""
	if e.nav {
		size = "      "
	} else if !e.Info.IsDir() {
		size = formatSize(e.Info.Size())
	} else {
		size = "   DIR"
	}
	size = fmt.Sprintf("%6s", size)

	prefix := "  "
	if e.Selected {
		prefix = "* "
	}

	line := prefix + name + "  " + size

	navStyle := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)

	switch {
	case isCursor && e.nav:
		return StyleCursorItem.Width(w).Render(line)
	case isCursor && e.Selected:
		return StyleCursorSelectedItem.Width(w).Render(line)
	case isCursor:
		return StyleCursorItem.Width(w).Render(line)
	case e.nav:
		return navStyle.Width(w).Render(line)
	case e.Selected:
		return StyleSelectedItem.Width(w).Render(line)
	case e.Info.IsDir():
		return StyleDir.Width(w).Render(line)
	default:
		return StyleFilename.Width(w).Render(line)
	}
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(b)/float64(div), "KMGTPE"[exp])
}

// LoadLocal reads the local filesystem into the pane.
func (m *FilePaneModel) LoadLocal() error {
	f, err := os.Open(m.Dir)
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.Readdir(-1)
	if err != nil {
		return err
	}
	m.SetEntries(entries)
	return nil
}

// NavigateLocal changes directory and reloads.
func (m *FilePaneModel) NavigateLocal(name string) error {
	var newDir string
	if name == ".." {
		newDir = filepath.Dir(m.Dir)
	} else if name == "/" {
		newDir = "/"
	} else {
		newDir = filepath.Join(m.Dir, name)
	}
	old := m.Dir
	m.Dir = newDir
	if err := m.LoadLocal(); err != nil {
		m.Dir = old
		return err
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
