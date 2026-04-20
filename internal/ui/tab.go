package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	sftpclient "github.com/aaronkiula/sftp-tui/internal/sftp"
)

type TabModel struct {
	Label      string
	client     *sftpclient.Client
	left       FilePaneModel
	right      FilePaneModel
	focusRight bool
	progress   ProgressModel
	width      int
	height     int
	jobCounter int
	confirmMsg string
	confirmCmd tea.Cmd
}

// TransferProgressMsg carries progress from a transfer goroutine.
type TransferProgressMsg struct {
	JobID string
	Sent  int64
	Total int64
	Done  bool
	Err   error
}

// DirLoadedMsg carries async directory listing results.
type DirLoadedMsg struct {
	PaneID  string
	Entries []fs.FileInfo
	Err     error
}

// MkdirDoneMsg signals mkdir completion.
type MkdirDoneMsg struct {
	PaneID string
	Err    error
}

// DeleteDoneMsg signals deletion completion.
type DeleteDoneMsg struct {
	PaneID string
	Err    error
}

func NewTab(label string, client *sftpclient.Client, localDir string) TabModel {
	left := NewFilePane("left", PaneLocal, localDir)
	left.SetFocused(true)
	left.LoadLocal() //nolint:errcheck

	right := NewFilePane("right", PaneRemote, "/")
	right.SetFocused(false)

	t := TabModel{
		Label:    label,
		client:   client,
		left:     left,
		right:    right,
		progress: NewProgressModel(),
	}
	return t
}

func (t TabModel) Init() tea.Cmd {
	return t.loadRemote(t.right.Dir)
}

func (t TabModel) focusedPane() *FilePaneModel {
	if t.focusRight {
		return &t.right
	}
	return &t.left
}

func (t TabModel) Update(msg tea.Msg) (TabModel, tea.Cmd) {
	// Handle internal progress/dir messages first.
	switch msg := msg.(type) {
	case TransferProgressMsg:
		t.progress.UpdateJob(msg.JobID, msg.Sent, msg.Total, msg.Done, msg.Err)
		if msg.Done {
			// Refresh panes after transfer.
			var cmds []tea.Cmd
			cmds = append(cmds, t.loadRemote(t.right.Dir))
			t.left.LoadLocal() //nolint:errcheck
			return t, tea.Batch(cmds...)
		}
		return t, nil

	case DirLoadedMsg:
		if msg.Err != nil {
			return t, statusCmd("Error listing remote directory")
		}
		if msg.PaneID == "right" {
			t.right.SetEntries(msg.Entries)
		}
		return t, nil

	case MkdirDoneMsg:
		if msg.Err != nil {
			if msg.PaneID == "right" {
				return t, statusCmd("Remote mkdir failed")
			}
			return t, statusCmd("mkdir error: " + msg.Err.Error())
		}
		if msg.PaneID == "right" {
			return t, t.loadRemote(t.right.Dir)
		}
		t.left.LoadLocal() //nolint:errcheck
		return t, nil

	case DeleteDoneMsg:
		if msg.Err != nil {
			if msg.PaneID == "right" {
				return t, statusCmd("Remote delete failed")
			}
			return t, statusCmd("delete error: " + msg.Err.Error())
		}
		if msg.PaneID == "right" {
			return t, t.loadRemote(t.right.Dir)
		}
		t.left.LoadLocal() //nolint:errcheck
		return t, nil
	}

	// Key handling.
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Confirmation prompt active.
		if t.confirmMsg != "" {
			switch keyMsg.String() {
			case "y", "Y":
				cmd := t.confirmCmd
				t.confirmMsg = ""
				t.confirmCmd = nil
				return t, cmd
			default:
				t.confirmMsg = ""
				t.confirmCmd = nil
				return t, statusCmd("Cancelled")
			}
		}

		switch keyMsg.String() {
		case "tab":
			t.focusRight = !t.focusRight
			t.left.SetFocused(!t.focusRight)
			t.right.SetFocused(t.focusRight)
			return t, nil

		case "enter":
			return t.handleEnter()

		case "backspace":
			return t.handleBackspace()

		case "f5":
			return t.handleCopy(false)

		case "f6":
			return t.handleCopy(true)

		case "f7":
			return t.handleMkdir()

		case "f8":
			return t.handleDelete()

		case "f2":
			return t.handleRename()

		case "r", "f10":
			return t.handleRefresh()
		}
	}

	// Delegate to focused pane.
	var cmd tea.Cmd
	if t.focusRight {
		t.right, cmd = t.right.Update(msg)
	} else {
		t.left, cmd = t.left.Update(msg)
	}
	return t, cmd
}

func (t TabModel) handleEnter() (TabModel, tea.Cmd) {
	pane := t.focusedPane()
	entry := pane.CursorEntry()
	if entry == nil || !entry.Info.IsDir() {
		return t, nil
	}
	name := entry.Info.Name()

	if t.focusRight {
		newDir := remoteJoin(t.right.Dir, name)
		t.right.Dir = newDir
		return t, t.loadRemote(newDir)
	}
	if err := t.left.NavigateLocal(name); err != nil {
		return t, statusCmd("Navigate error: " + err.Error())
	}
	return t, nil
}

func (t TabModel) handleBackspace() (TabModel, tea.Cmd) {
	if t.focusRight {
		parent := remoteParent(t.right.Dir)
		t.right.Dir = parent
		return t, t.loadRemote(parent)
	}
	if err := t.left.NavigateLocal(".."); err != nil {
		return t, statusCmd("Navigate error: " + err.Error())
	}
	return t, nil
}

func (t TabModel) handleCopy(move bool) (TabModel, tea.Cmd) {
	if t.focusRight {
		// Download: remote → local.
		selected := t.right.SelectedEntries()
		if len(selected) == 0 {
			return t, statusCmd("Nothing selected")
		}
		var cmds []tea.Cmd
		for _, e := range selected {
			if e.Info.IsDir() {
				continue // no recursive transfers
			}
			remotePath := remoteJoin(t.right.Dir, e.Info.Name())
			localPath := filepath.Join(t.left.Dir, e.Info.Name())
			cmds = append(cmds, t.startDownload(remotePath, localPath, e.Info.Name(), e.Info.Size(), move))
		}
		return t, tea.Batch(cmds...)
	}
	// Upload: local → remote.
	selected := t.left.SelectedEntries()
	if len(selected) == 0 {
		return t, statusCmd("Nothing selected")
	}
	var cmds []tea.Cmd
	for _, e := range selected {
		if e.Info.IsDir() {
			continue
		}
		localPath := filepath.Join(t.left.Dir, e.Info.Name())
		remotePath := remoteJoin(t.right.Dir, e.Info.Name())
		cmds = append(cmds, t.startUpload(localPath, remotePath, e.Info.Name(), e.Info.Size(), move))
	}
	return t, tea.Batch(cmds...)
}

func (t *TabModel) startDownload(remotePath, localPath, name string, size int64, deleteAfter bool) tea.Cmd {
	t.jobCounter++
	jobID := fmt.Sprintf("dl-%d", t.jobCounter)
	t.progress.AddJob(jobID, name, "download", size)
	client := t.client
	prog := &t.progress
	return func() tea.Msg {
		err := client.Download(remotePath, localPath, func(sent, total int64) {
			// Note: we can't send tea.Msgs from here directly.
			// Progress is updated via polling cmd below.
			_ = sent
			_ = total
		})
		if err == nil && deleteAfter {
			client.SFTP.Remove(remotePath) //nolint:errcheck
		}
		prog.UpdateJob(jobID, size, size, true, err)
		return TransferProgressMsg{JobID: jobID, Sent: size, Total: size, Done: true, Err: err}
	}
}

func (t *TabModel) startUpload(localPath, remotePath, name string, size int64, deleteAfter bool) tea.Cmd {
	t.jobCounter++
	jobID := fmt.Sprintf("ul-%d", t.jobCounter)
	t.progress.AddJob(jobID, name, "upload", size)
	client := t.client
	prog := &t.progress
	return func() tea.Msg {
		err := client.Upload(localPath, remotePath, func(sent, total int64) {
			_ = sent
			_ = total
		})
		if err == nil && deleteAfter {
			os.Remove(localPath) //nolint:errcheck
		}
		prog.UpdateJob(jobID, size, size, true, err)
		return TransferProgressMsg{JobID: jobID, Sent: size, Total: size, Done: true, Err: err}
	}
}

func (t TabModel) handleMkdir() (TabModel, tea.Cmd) {
	// For simplicity, prompt via status — a real implementation would use a textinput popup.
	// Here we create a default-named directory with a timestamp.
	// TODO: integrate a mini prompt.
	pane := t.focusedPane()
	if t.focusRight {
		client := t.client
		dir := t.right.Dir
		paneID := "right"
		return t, func() tea.Msg {
			err := client.SFTP.MkdirAll(remoteJoin(dir, "new_folder"))
			return MkdirDoneMsg{PaneID: paneID, Err: err}
		}
	}
	newDir := filepath.Join(pane.Dir, "new_folder")
	if err := os.Mkdir(newDir, 0755); err != nil {
		return t, statusCmd("mkdir error: " + err.Error())
	}
	t.left.LoadLocal() //nolint:errcheck
	return t, statusCmd("Directory created")
}

func (t TabModel) handleDelete() (TabModel, tea.Cmd) {
	pane := t.focusedPane()
	selected := pane.SelectedEntries()
	if len(selected) == 0 {
		return t, nil
	}
	names := make([]string, 0, len(selected))
	for _, e := range selected {
		names = append(names, e.Info.Name())
	}
	t.confirmMsg = fmt.Sprintf("Delete %d item(s)? (y/N)", len(names))
	if t.focusRight {
		dir := t.right.Dir
		client := t.client
		t.confirmCmd = func() tea.Msg {
			for _, e := range selected {
				p := remoteJoin(dir, e.Info.Name())
				if e.Info.IsDir() {
					client.SFTP.RemoveDirectory(p) //nolint:errcheck
				} else {
					client.SFTP.Remove(p) //nolint:errcheck
				}
			}
			return DeleteDoneMsg{PaneID: "right"}
		}
	} else {
		dir := t.left.Dir
		t.confirmCmd = func() tea.Msg {
			for _, e := range selected {
				p := filepath.Join(dir, e.Info.Name())
				if e.Info.IsDir() {
					os.RemoveAll(p) //nolint:errcheck
				} else {
					os.Remove(p) //nolint:errcheck
				}
			}
			return DeleteDoneMsg{PaneID: "left"}
		}
	}
	return t, nil
}

func (t TabModel) handleRename() (TabModel, tea.Cmd) {
	// Minimal: not yet implemented — future enhancement.
	return t, statusCmd("Rename: not yet implemented")
}

func (t TabModel) handleRefresh() (TabModel, tea.Cmd) {
	t.left.LoadLocal() //nolint:errcheck
	return t, t.loadRemote(t.right.Dir)
}

func (t TabModel) loadRemote(dir string) tea.Cmd {
	client := t.client
	return func() tea.Msg {
		entries, err := client.SFTP.ReadDir(dir)
		if err != nil {
			return DirLoadedMsg{PaneID: "right", Err: err}
		}
		// Convert []os.FileInfo.
		infos := make([]fs.FileInfo, len(entries))
		for i, e := range entries {
			infos[i] = e
		}
		return DirLoadedMsg{PaneID: "right", Entries: infos}
	}
}

func (t TabModel) SetSize(w, h int) TabModel {
	t.width = w
	t.height = h
	paneW := w / 2
	paneH := h - 3 // leave room for progress + confirm
	t.left.SetSize(paneW, paneH)
	t.right.SetSize(w-paneW, paneH)
	t.progress.SetWidth(w)
	return t
}

func (t TabModel) View() string {
	left := t.left.View()
	right := t.right.View()
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	var extras []string
	if t.confirmMsg != "" {
		extras = append(extras, StyleError.Render("  "+t.confirmMsg))
	}
	progView := t.progress.View()
	if progView != "" {
		extras = append(extras, progView)
	}

	if len(extras) > 0 {
		return lipgloss.JoinVertical(lipgloss.Left, panes, strings.Join(extras, "\n"))
	}
	return panes
}

func remoteJoin(dir, name string) string {
	if name == ".." {
		return remoteParent(dir)
	}
	if name == "/" {
		return "/"
	}
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

func remoteParent(dir string) string {
	if dir == "/" || dir == "" {
		return "/"
	}
	dir = strings.TrimSuffix(dir, "/")
	idx := strings.LastIndex(dir, "/")
	if idx <= 0 {
		return "/"
	}
	return dir[:idx]
}

func statusCmd(msg string) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: msg} }
}

type StatusMsg struct{ Text string }
