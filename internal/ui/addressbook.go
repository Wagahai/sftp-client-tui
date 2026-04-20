package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aaronkiula/sftp-tui/internal/addressbook"
)

type ABConnectMsg struct{ Entry addressbook.Entry }
type ABCloseMsg struct{}

type abState int

const (
	abList abState = iota
	abUnlock
	abEdit
	abConfirmDelete
)

type AddressBookModel struct {
	state   abState
	book    *addressbook.Book
	entries []addressbook.Entry
	cursor  int

	// unlock form
	pwInput textinput.Model
	pwErr   string

	// edit form (reuse ConnectModel fields pattern)
	editInputs  []textinput.Model
	editFocused int
	editAuthIdx int
	editID      string // empty = new entry
	editErr     string

	deleteTarget string

	width  int
	height int
}

const (
	abHost = iota
	abPort
	abUser
	abLabel
	abPassword
	abKeyPath
	abKeyPassphrase
	abFieldCount
)

func NewAddressBookModel() AddressBookModel {
	pw := textinput.New()
	pw.Placeholder = "master password"
	pw.EchoMode = textinput.EchoPassword
	pw.CharLimit = 256
	pw.Focus()

	return AddressBookModel{
		state:   abUnlock,
		pwInput: pw,
	}
}

func (m *AddressBookModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m AddressBookModel) IsUnlocked() bool {
	return m.book != nil
}

func (m *AddressBookModel) Zero() {
	if m.book != nil {
		m.book.Zero()
	}
}

func (m AddressBookModel) Update(msg tea.Msg) (AddressBookModel, tea.Cmd) {
	switch m.state {
	case abUnlock:
		return m.updateUnlock(msg)
	case abList:
		return m.updateList(msg)
	case abEdit:
		return m.updateEdit(msg)
	case abConfirmDelete:
		return m.updateConfirmDelete(msg)
	}
	return m, nil
}

func (m AddressBookModel) updateUnlock(msg tea.Msg) (AddressBookModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ABCloseMsg{} }
		case "enter":
			pw := []byte(m.pwInput.Value())
			book, err := addressbook.Open(pw)
			// Zero the slice we just passed in.
			for i := range pw {
				pw[i] = 0
			}
			if err != nil {
				if err == addressbook.ErrWrongPassword {
					m.pwErr = "Wrong password"
				} else {
					m.pwErr = err.Error()
				}
				m.pwInput.SetValue("")
				return m, textinput.Blink
			}
			m.book = book
			m.entries = book.List()
			m.state = abList
			m.pwErr = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.pwInput, cmd = m.pwInput.Update(msg)
	return m, cmd
}

func (m AddressBookModel) updateList(msg tea.Msg) (AddressBookModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+b":
			return m, func() tea.Msg { return ABCloseMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				return m, func() tea.Msg { return ABConnectMsg{Entry: m.entries[m.cursor]} }
			}
		case "n":
			return m, m.openEdit(addressbook.Entry{})
		case "e":
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				return m, m.openEdit(m.entries[m.cursor])
			}
		case "d", "delete":
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				m.deleteTarget = m.entries[m.cursor].ID
				m.state = abConfirmDelete
			}
		}
	}
	return m, nil
}

func (m AddressBookModel) openEdit(e addressbook.Entry) tea.Cmd {
	inputs := make([]textinput.Model, abFieldCount)
	inputs[abHost] = newABInput("Host", e.Host, false)
	inputs[abPort] = newABInput("Port", e.Port, false)
	inputs[abUser] = newABInput("User", e.User, false)
	inputs[abLabel] = newABInput("Label (display name)", e.Label, false)
	inputs[abPassword] = newABInput("Password", e.Password, true)
	inputs[abKeyPath] = newABInput("SSH Key Path", e.KeyPath, false)
	inputs[abKeyPassphrase] = newABInput("Key Passphrase", e.KeyPassphrase, true)
	inputs[abHost].Focus()

	authIdx := 0
	if e.AuthType == "key" {
		authIdx = 1
	}

	return func() tea.Msg {
		return abOpenEditMsg{
			inputs:  inputs,
			authIdx: authIdx,
			editID:  e.ID,
		}
	}
}

type abOpenEditMsg struct {
	inputs  []textinput.Model
	authIdx int
	editID  string
}

func (m AddressBookModel) updateEdit(msg tea.Msg) (AddressBookModel, tea.Cmd) {
	switch msg := msg.(type) {
	case abOpenEditMsg:
		m.editInputs = msg.inputs
		m.editAuthIdx = msg.authIdx
		m.editID = msg.editID
		m.editFocused = 0
		m.state = abEdit
		return m, textinput.Blink

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = abList
			return m, nil
		case "ctrl+t":
			m.editAuthIdx = 1 - m.editAuthIdx
		case "tab", "down":
			m.editInputs[m.editFocused].Blur()
			m.editFocused = m.editNextField()
			m.editInputs[m.editFocused].Focus()
			return m, textinput.Blink
		case "shift+tab", "up":
			m.editInputs[m.editFocused].Blur()
			m.editFocused = m.editPrevField()
			m.editInputs[m.editFocused].Focus()
			return m, textinput.Blink
		case "enter":
			if m.editFocused == m.editLastField() {
				return m, m.saveEntry()
			}
			m.editInputs[m.editFocused].Blur()
			m.editFocused = m.editNextField()
			m.editInputs[m.editFocused].Focus()
			return m, textinput.Blink
		case "ctrl+s":
			return m, m.saveEntry()
		}
	}

	var cmd tea.Cmd
	m.editInputs[m.editFocused], cmd = m.editInputs[m.editFocused].Update(msg)
	return m, cmd
}

func (m AddressBookModel) editActiveFields() []int {
	base := []int{abLabel, abHost, abPort, abUser}
	if m.editAuthIdx == 0 {
		return append(base, abPassword)
	}
	return append(base, abKeyPath, abKeyPassphrase)
}

func (m AddressBookModel) editNextField() int {
	fields := m.editActiveFields()
	for i, f := range fields {
		if f == m.editFocused {
			return fields[(i+1)%len(fields)]
		}
	}
	return fields[0]
}

func (m AddressBookModel) editPrevField() int {
	fields := m.editActiveFields()
	for i, f := range fields {
		if f == m.editFocused {
			prev := i - 1
			if prev < 0 {
				prev = len(fields) - 1
			}
			return fields[prev]
		}
	}
	return fields[0]
}

func (m AddressBookModel) editLastField() int {
	fields := m.editActiveFields()
	return fields[len(fields)-1]
}

func (m AddressBookModel) saveEntry() tea.Cmd {
	host := strings.TrimSpace(m.editInputs[abHost].Value())
	user := strings.TrimSpace(m.editInputs[abUser].Value())
	if host == "" || user == "" {
		m.editErr = "Host and user are required"
		return nil
	}

	port := strings.TrimSpace(m.editInputs[abPort].Value())
	if port == "" {
		port = "22"
	}
	label := strings.TrimSpace(m.editInputs[abLabel].Value())
	if label == "" {
		label = fmt.Sprintf("%s@%s", user, host)
	}

	e := addressbook.Entry{
		ID:    m.editID,
		Label: label,
		Host:  host,
		Port:  port,
		User:  user,
	}
	if m.editAuthIdx == 0 {
		e.AuthType = "password"
		e.Password = m.editInputs[abPassword].Value()
	} else {
		e.AuthType = "key"
		e.KeyPath = strings.TrimSpace(m.editInputs[abKeyPath].Value())
		e.KeyPassphrase = m.editInputs[abKeyPassphrase].Value()
	}

	return func() tea.Msg { return abSaveEntryMsg{entry: e} }
}

type abSaveEntryMsg struct{ entry addressbook.Entry }

func (m AddressBookModel) updateConfirmDelete(msg tea.Msg) (AddressBookModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			return m, func() tea.Msg { return abDeleteMsg{id: m.deleteTarget} }
		default:
			m.state = abList
			m.deleteTarget = ""
		}
	}
	return m, nil
}

type abDeleteMsg struct{ id string }

// HandleMsg processes internal messages that require book mutation.
// Called from AppModel.Update after routing.
func (m AddressBookModel) HandleMsg(msg tea.Msg) (AddressBookModel, tea.Cmd) {
	switch msg := msg.(type) {
	case abOpenEditMsg:
		return m.updateEdit(msg)
	case abSaveEntryMsg:
		if m.book == nil {
			return m, nil
		}
		var err error
		if msg.entry.ID == "" {
			err = m.book.Add(msg.entry)
		} else {
			err = m.book.Update(msg.entry)
		}
		if err != nil {
			m.editErr = err.Error()
			return m, nil
		}
		m.entries = m.book.List()
		m.state = abList
		m.editErr = ""
	case abDeleteMsg:
		if m.book != nil {
			m.book.Delete(msg.id)
			m.entries = m.book.List()
			if m.cursor >= len(m.entries) && m.cursor > 0 {
				m.cursor--
			}
		}
		m.state = abList
	}
	return m, nil
}

func (m AddressBookModel) View() string {
	w := m.width - 8
	if w < 40 {
		w = 40
	}

	var body string
	switch m.state {
	case abUnlock:
		body = m.viewUnlock(w)
	case abList:
		body = m.viewList(w)
	case abEdit:
		body = m.viewEdit(w)
	case abConfirmDelete:
		body = m.viewConfirmDelete(w)
	}

	return StyleModalOverlay.Width(w + 4).Render(body)
}

func (m AddressBookModel) viewUnlock(w int) string {
	title := StyleLabel.Render("Address Book — Unlock")
	hint := StyleSubtle("Enter master password (or press Enter for empty password)")
	errLine := ""
	if m.pwErr != "" {
		errLine = StyleError.Render(m.pwErr)
	}
	return strings.Join([]string{title, "", hint, m.pwInput.View(), errLine, "", StyleSubtle("Enter unlock  Esc close")}, "\n")
}

func (m AddressBookModel) viewList(w int) string {
	title := StyleLabel.Render("Address Book")

	var rows []string
	for i, e := range m.entries {
		line := fmt.Sprintf("%-30s %s@%s:%s", TruncateStr(e.Label, 30), e.User, e.Host, e.Port)
		line = TruncateStr(line, w)
		if i == m.cursor {
			rows = append(rows, StyleCursorItem.Width(w).Render(line))
		} else {
			rows = append(rows, lipgloss.NewStyle().Width(w).Render(line))
		}
	}

	if len(rows) == 0 {
		rows = append(rows, StyleSubtle("  No entries. Press N to add one."))
	}

	hint := StyleSubtle("Enter connect  N new  E edit  D delete  Esc close")
	return strings.Join(append([]string{title, ""}, append(rows, "", hint)...), "\n")
}

func (m AddressBookModel) viewEdit(w int) string {
	var action string
	if m.editID == "" {
		action = "New Entry"
	} else {
		action = "Edit Entry"
	}
	title := StyleLabel.Render(action)

	authTypes := []string{"[1] Password", "[2] SSH Key"}
	authSel := "Auth: "
	for i, a := range authTypes {
		if i == m.editAuthIdx {
			authSel += StyleTabActive.Render(a) + " "
		} else {
			authSel += StyleTabInactive.Render(a) + " "
		}
	}
	authSel += StyleSubtle("  Ctrl+T toggle")

	var fieldLines []string
	fw := w - 14
	fieldLines = append(fieldLines, renderABField("Label", m.editInputs[abLabel], m.editFocused == abLabel, fw))
	fieldLines = append(fieldLines, renderABField("Host", m.editInputs[abHost], m.editFocused == abHost, fw))
	fieldLines = append(fieldLines, renderABField("Port", m.editInputs[abPort], m.editFocused == abPort, fw))
	fieldLines = append(fieldLines, renderABField("User", m.editInputs[abUser], m.editFocused == abUser, fw))
	if m.editAuthIdx == 0 {
		fieldLines = append(fieldLines, renderABField("Password", m.editInputs[abPassword], m.editFocused == abPassword, fw))
	} else {
		fieldLines = append(fieldLines, renderABField("Key Path", m.editInputs[abKeyPath], m.editFocused == abKeyPath, fw))
		fieldLines = append(fieldLines, renderABField("Passphrase", m.editInputs[abKeyPassphrase], m.editFocused == abKeyPassphrase, fw))
	}

	errLine := ""
	if m.editErr != "" {
		errLine = StyleError.Render(m.editErr)
	}

	hint := StyleSubtle("Tab navigate  Ctrl+S save  Esc cancel")
	parts := []string{title, "", authSel, ""}
	parts = append(parts, fieldLines...)
	parts = append(parts, "", errLine, hint)
	return strings.Join(parts, "\n")
}

func (m AddressBookModel) viewConfirmDelete(w int) string {
	msg := StyleError.Render("Delete this entry? (Y/n)")
	return lipgloss.NewStyle().Width(w).Render(msg)
}

func newABInput(placeholder, value string, password bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.CharLimit = 256
	if password {
		ti.EchoMode = textinput.EchoPassword
	}
	return ti
}

func renderABField(label string, ti textinput.Model, focused bool, w int) string {
	lbl := StyleLabel.Render(fmt.Sprintf("%-12s", label+":"))
	style := lipgloss.NewStyle().Width(w)
	if focused {
		style = style.BorderBottom(true).BorderForeground(ColorAccent)
	}
	return lbl + style.Render(ti.View())
}
