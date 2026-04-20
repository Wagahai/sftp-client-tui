package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	sftpclient "github.com/aaronkiula/sftp-tui/internal/sftp"
	"github.com/aaronkiula/sftp-tui/internal/ui"
)

type ConnectingMsg struct {
	Opts sftpclient.ConnectOpts
}

type ConnectedMsg struct {
	Client *sftpclient.Client
	Label  string
}

type ConnectErrMsg struct {
	Err  error
	Opts sftpclient.ConnectOpts
}

type Model struct {
	tabs            []ui.TabModel
	activeTab       int
	showConnect     bool
	showAddressBook bool
	showHelp        bool
	connectModel    ui.ConnectModel
	addressBook     ui.AddressBookModel
	statusBar       ui.StatusBar
	width           int
	height          int
	localDir        string

	// Host key TOFU confirmation state.
	showHostKeyConfirm bool
	pendingHostKeyErr  *sftpclient.UnknownHostError
	pendingConnectOpts sftpclient.ConnectOpts
}

func New() Model {
	dir, _ := os.Getwd()
	return Model{
		connectModel: ui.NewConnectModel(),
		addressBook:  ui.NewAddressBookModel(),
		statusBar:    ui.NewStatusBar(),
		localDir:     dir,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case ui.StatusMsg:
		m.statusBar.SetMessage(msg.Text)
		return m, nil

	case ConnectingMsg:
		opts := msg.Opts
		return m, func() tea.Msg {
			client, err := sftpclient.Connect(opts)
			if err != nil {
				return ConnectErrMsg{Err: err, Opts: opts}
			}
			return ConnectedMsg{Client: client, Label: client.ConnLabel}
		}

	case ConnectedMsg:
		tab := ui.NewTab(msg.Label, msg.Client, m.localDir)
		tab = tab.SetSize(m.width, m.height-3) // 3 = tabbar + statusbar
		m.tabs = append(m.tabs, tab)
		m.activeTab = len(m.tabs) - 1
		m.showConnect = false
		m.statusBar.SetMessage(fmt.Sprintf("Connected to %s", msg.Label))
		return m, m.tabs[m.activeTab].Init()

	case ConnectErrMsg:
		var uhe *sftpclient.UnknownHostError
		if errors.As(msg.Err, &uhe) {
			m.showHostKeyConfirm = true
			m.pendingHostKeyErr = uhe
			m.pendingConnectOpts = msg.Opts
			m.statusBar.SetMessage("Unknown host — press y to trust, any other key to cancel")
			return m, nil
		}
		m.statusBar.SetMessage("Connection failed: " + msg.Err.Error())
		return m, nil

	case ui.ConnectSubmitMsg:
		m.showConnect = false
		opts := sftpclient.ConnectOpts{
			Host:          msg.Host,
			Port:          msg.Port,
			User:          msg.User,
			Password:      msg.Password,
			KeyPath:       msg.KeyPath,
			KeyPassphrase: msg.KeyPassphrase,
		}
		return m, func() tea.Msg { return ConnectingMsg{Opts: opts} }

	case ui.ConnectCancelMsg:
		m.showConnect = false
		return m, nil

	case ui.ABConnectMsg:
		m.showAddressBook = false
		e := msg.Entry
		opts := sftpclient.ConnectOpts{
			Host:          e.Host,
			Port:          e.Port,
			User:          e.User,
			Password:      e.Password,
			KeyPath:       e.KeyPath,
			KeyPassphrase: e.KeyPassphrase,
		}
		m.statusBar.SetMessage(fmt.Sprintf("Connecting to %s...", e.Label))
		return m, func() tea.Msg { return ConnectingMsg{Opts: opts} }

	case ui.ABCloseMsg:
		m.showAddressBook = false
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Route message to active tab or modals.
	return m.routeMsg(msg)
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Host key TOFU confirmation takes priority over all other key handling.
	if m.showHostKeyConfirm {
		switch msg.String() {
		case "y", "Y":
			if err := sftpclient.AddKnownHost(m.pendingHostKeyErr.Host, m.pendingHostKeyErr.Key); err != nil {
				m.statusBar.SetMessage("Failed to save host key: " + err.Error())
				m.showHostKeyConfirm = false
				m.pendingHostKeyErr = nil
				return m, nil
			}
			opts := m.pendingConnectOpts
			m.showHostKeyConfirm = false
			m.pendingHostKeyErr = nil
			m.pendingConnectOpts = sftpclient.ConnectOpts{}
			return m, func() tea.Msg { return ConnectingMsg{Opts: opts} }
		default:
			m.showHostKeyConfirm = false
			m.pendingHostKeyErr = nil
			m.pendingConnectOpts = sftpclient.ConnectOpts{}
			m.statusBar.SetMessage("Connection cancelled")
			return m, nil
		}
	}

	// Global shortcuts always active.
	switch msg.String() {
	case "ctrl+c":
		if m.addressBook.IsUnlocked() {
			m.addressBook.Zero()
		}
		for i := range m.tabs {
			if m.tabs[i].Label != "" {
				// Close clients (best effort).
			}
		}
		return m, tea.Quit

	case "ctrl+n":
		m.showConnect = true
		m.showAddressBook = false
		m.showHelp = false
		m.connectModel = ui.NewConnectModel()
		m.connectModel.SetSize(m.width, m.height)
		return m, nil

	case "ctrl+b":
		m.showConnect = false
		m.showAddressBook = !m.showAddressBook
		m.showHelp = false
		if m.showAddressBook {
			m.addressBook.SetSize(m.width, m.height)
		}
		return m, nil

	case "ctrl+w":
		if len(m.tabs) > 0 {
			m.tabs = append(m.tabs[:m.activeTab], m.tabs[m.activeTab+1:]...)
			if m.activeTab >= len(m.tabs) && m.activeTab > 0 {
				m.activeTab--
			}
		}
		return m, nil

	case "?":
		if !m.showConnect && !m.showAddressBook {
			m.showHelp = !m.showHelp
		}
		return m, nil
	}

	// Alt+1..9 tab switching.
	if strings.HasPrefix(msg.String(), "alt+") {
		digit := msg.String()[4:]
		if len(digit) == 1 && digit[0] >= '1' && digit[0] <= '9' {
			idx := int(digit[0]-'1')
			if idx < len(m.tabs) {
				m.activeTab = idx
			}
			return m, nil
		}
	}

	// Route key to active modal or tab.
	return m.routeKey(msg)
}

func (m Model) routeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.showConnect {
		var cmd tea.Cmd
		m.connectModel, cmd = m.connectModel.Update(msg)
		return m, cmd
	}
	if m.showAddressBook {
		var cmd tea.Cmd
		m.addressBook, cmd = m.addressBook.Update(msg)
		return m, cmd
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	return m.routeToActiveTab(msg)
}

func (m Model) routeMsg(msg tea.Msg) (Model, tea.Cmd) {
	if m.showConnect {
		var cmd tea.Cmd
		m.connectModel, cmd = m.connectModel.Update(msg)
		return m, cmd
	}
	if m.showAddressBook {
		var cmd1, cmd2 tea.Cmd
		m.addressBook, cmd1 = m.addressBook.Update(msg)
		m.addressBook, cmd2 = m.addressBook.HandleMsg(msg)
		return m, tea.Batch(cmd1, cmd2)
	}
	return m.routeToActiveTab(msg)
}

func (m Model) routeToActiveTab(msg tea.Msg) (Model, tea.Cmd) {
	if len(m.tabs) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.tabs[m.activeTab], cmd = m.tabs[m.activeTab].Update(msg)
	return m, cmd
}

func (m *Model) resize() {
	contentH := m.height - 2 // tabbar + statusbar
	for i := range m.tabs {
		m.tabs[i] = m.tabs[i].SetSize(m.width, contentH)
	}
	m.connectModel.SetSize(m.width, m.height)
	m.addressBook.SetSize(m.width, m.height)
	m.statusBar.SetWidth(m.width)
}

func (m Model) View() string {
	tabBar := m.renderTabBar()
	statusLine := m.statusBar.View()

	var content string
	if len(m.tabs) == 0 {
		content = m.renderWelcome()
	} else {
		content = m.tabs[m.activeTab].View()
	}

	base := lipgloss.JoinVertical(lipgloss.Left, tabBar, content, statusLine)

	// Layer modals on top.
	if m.showHelp {
		overlay := m.renderHelp()
		base = placeOverlay(m.width, m.height, overlay, base)
	}
	if m.showConnect {
		overlay := m.connectModel.View()
		base = placeOverlay(m.width, m.height, overlay, base)
	}
	if m.showAddressBook {
		overlay := m.addressBook.View()
		base = placeOverlay(m.width, m.height, overlay, base)
	}
	if m.showHostKeyConfirm && m.pendingHostKeyErr != nil {
		base = placeOverlay(m.width, m.height, m.renderHostKeyConfirm(), base)
	}

	return base
}

func (m Model) renderHostKeyConfirm() string {
	e := m.pendingHostKeyErr
	lines := []string{
		ui.StyleLabel.Render(" Unknown Host Key "),
		"",
		fmt.Sprintf("  Host:        %s", e.Host),
		fmt.Sprintf("  Algorithm:   %s", e.Key.Type()),
		fmt.Sprintf("  Fingerprint: %s", e.Fingerprint),
		"",
		"  Trust this host and connect? (y/N)",
	}
	return ui.StyleHelp.Render(strings.Join(lines, "\n"))
}

func (m Model) renderTabBar() string {
	if len(m.tabs) == 0 {
		hint := ui.StyleSubtle("  Ctrl+N new connection  Ctrl+B address book  ? help")
		return ui.StyleTabBar.Width(m.width).Render(hint)
	}
	var parts []string
	for i, t := range m.tabs {
		label := fmt.Sprintf(" %d:%s ", i+1, t.Label)
		if i == m.activeTab {
			parts = append(parts, ui.StyleTabActive.Render(label))
		} else {
			parts = append(parts, ui.StyleTabInactive.Render(label))
		}
	}
	return ui.StyleTabBar.Width(m.width).Render(strings.Join(parts, ""))
}

func (m Model) renderWelcome() string {
	w := m.width
	h := m.height - 2
	msg := lipgloss.NewStyle().
		Foreground(ui.ColorAccent).
		Bold(true).
		Render("sftp-tui")
	sub := ui.StyleSubtle("Press Ctrl+N to connect to a server, or Ctrl+B to open the address book.")
	keys := strings.Join([]string{
		ui.StyleSubtle("Ctrl+N") + "  new connection",
		ui.StyleSubtle("Ctrl+B") + "  address book",
		ui.StyleSubtle("?     ") + "  help",
		ui.StyleSubtle("Ctrl+C") + "  quit",
	}, "\n")
	body := lipgloss.JoinVertical(lipgloss.Center, msg, sub, "", keys)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) renderHelp() string {
	lines := []string{
		ui.StyleLabel.Render("Keyboard Shortcuts"),
		"",
		"Global",
		"  Ctrl+N    new connection",
		"  Ctrl+B    address book",
		"  Ctrl+W    close tab",
		"  Alt+1..9  switch tab",
		"  ?         toggle help",
		"  Ctrl+C    quit",
		"",
		"File Pane",
		"  ↑/↓  k/j  navigate",
		"  Enter       enter directory",
		"  Backspace   go up",
		"  Space       multi-select",
		"  *           select all/none",
		"  /           filter",
		"  s           cycle sort",
		"  h           toggle hidden",
		"  r / F10     refresh",
		"",
		"Tab (connection)",
		"  Tab         switch pane focus",
		"  F5          copy (upload/download)",
		"  F6          move (copy + delete)",
		"  F7          make directory",
		"  F8          delete selected",
		"  F2          rename (coming soon)",
		"",
		ui.StyleSubtle("Press any key to close"),
	}
	return ui.StyleHelp.Render(strings.Join(lines, "\n"))
}

// placeOverlay centers the overlay string on top of the base string.
func placeOverlay(w, h int, overlay, base string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, overlay,
		lipgloss.WithWhitespaceBackground(ui.ColorBg),
	)
}
