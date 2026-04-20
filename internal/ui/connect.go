package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ConnectSubmitMsg struct {
	Host          string
	Port          string
	User          string
	AuthType      string // "password" | "key"
	Password      string
	KeyPath       string
	KeyPassphrase string
}

type ConnectCancelMsg struct{}

type ConnectModel struct {
	inputs  []textinput.Model
	focused int
	authIdx int // 0=password 1=key
	err     string
	width   int
	height  int
}

const (
	ciHost = iota
	ciPort
	ciUser
	ciPassword
	ciKeyPath
	ciKeyPassphrase
	ciCount
)

func NewConnectModel() ConnectModel {
	inputs := make([]textinput.Model, ciCount)

	inputs[ciHost] = newInput("Host", "", false)
	inputs[ciHost].Focus()

	inputs[ciPort] = newInput("Port", "22", false)
	inputs[ciUser] = newInput("User", "", false)
	inputs[ciPassword] = newInput("Password", "", true)
	inputs[ciKeyPath] = newInput("Key file path", "", false)
	inputs[ciKeyPassphrase] = newInput("Key passphrase (optional)", "", true)

	return ConnectModel{inputs: inputs}
}

func newInput(placeholder string, value string, password bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.CharLimit = 256
	if password {
		ti.EchoMode = textinput.EchoPassword
	}
	return ti
}

func (m *ConnectModel) Prefill(host, port, user, authType, password, keyPath, keyPassphrase string) {
	m.inputs[ciHost].SetValue(host)
	m.inputs[ciPort].SetValue(port)
	m.inputs[ciUser].SetValue(user)
	m.inputs[ciPassword].SetValue(password)
	m.inputs[ciKeyPath].SetValue(keyPath)
	m.inputs[ciKeyPassphrase].SetValue(keyPassphrase)
	if authType == "key" {
		m.authIdx = 1
	} else {
		m.authIdx = 0
	}
}

func (m *ConnectModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m ConnectModel) Update(msg tea.Msg) (ConnectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ConnectCancelMsg{} }
		case "tab", "down":
			m.inputs[m.focused].Blur()
			m.focused = m.nextField()
			m.inputs[m.focused].Focus()
			return m, textinput.Blink
		case "shift+tab", "up":
			m.inputs[m.focused].Blur()
			m.focused = m.prevField()
			m.inputs[m.focused].Focus()
			return m, textinput.Blink
		case "ctrl+t":
			m.authIdx = 1 - m.authIdx
		case "enter":
			if m.focused == m.lastField() {
				return m, m.submit()
			}
			m.inputs[m.focused].Blur()
			m.focused = m.nextField()
			m.inputs[m.focused].Focus()
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

func (m ConnectModel) activeFields() []int {
	base := []int{ciHost, ciPort, ciUser}
	if m.authIdx == 0 {
		return append(base, ciPassword)
	}
	return append(base, ciKeyPath, ciKeyPassphrase)
}

func (m ConnectModel) nextField() int {
	fields := m.activeFields()
	for i, f := range fields {
		if f == m.focused {
			return fields[(i+1)%len(fields)]
		}
	}
	return fields[0]
}

func (m ConnectModel) prevField() int {
	fields := m.activeFields()
	for i, f := range fields {
		if f == m.focused {
			prev := i - 1
			if prev < 0 {
				prev = len(fields) - 1
			}
			return fields[prev]
		}
	}
	return fields[0]
}

func (m ConnectModel) lastField() int {
	fields := m.activeFields()
	return fields[len(fields)-1]
}

func (m ConnectModel) submit() tea.Cmd {
	host := strings.TrimSpace(m.inputs[ciHost].Value())
	port := strings.TrimSpace(m.inputs[ciPort].Value())
	user := strings.TrimSpace(m.inputs[ciUser].Value())

	if host == "" || user == "" {
		return nil
	}
	if port == "" {
		port = "22"
	}

	msg := ConnectSubmitMsg{Host: host, Port: port, User: user}
	if m.authIdx == 0 {
		msg.AuthType = "password"
		msg.Password = m.inputs[ciPassword].Value()
	} else {
		msg.AuthType = "key"
		msg.KeyPath = strings.TrimSpace(m.inputs[ciKeyPath].Value())
		msg.KeyPassphrase = m.inputs[ciKeyPassphrase].Value()
	}
	return func() tea.Msg { return msg }
}

func (m ConnectModel) View() string {
	w := m.width - 8
	if w < 30 {
		w = 30
	}

	title := StyleLabel.Render("New Connection")

	authTypes := []string{"[1] Password", "[2] SSH Key"}
	authLabel := "Auth: "
	authSel := ""
	for i, a := range authTypes {
		if i == m.authIdx {
			authSel += StyleTabActive.Render(a) + " "
		} else {
			authSel += StyleTabInactive.Render(a) + " "
		}
	}
	authLine := lipgloss.NewStyle().Render(authLabel) + authSel + StyleSubtle("  (Ctrl+T to toggle)")

	var fieldLines []string
	fieldLines = append(fieldLines, renderField("Host", m.inputs[ciHost], m.focused == ciHost, w))
	fieldLines = append(fieldLines, renderField("Port", m.inputs[ciPort], m.focused == ciPort, w))
	fieldLines = append(fieldLines, renderField("User", m.inputs[ciUser], m.focused == ciUser, w))

	if m.authIdx == 0 {
		fieldLines = append(fieldLines, renderField("Password", m.inputs[ciPassword], m.focused == ciPassword, w))
	} else {
		fieldLines = append(fieldLines, renderField("Key Path", m.inputs[ciKeyPath], m.focused == ciKeyPath, w))
		fieldLines = append(fieldLines, renderField("Passphrase", m.inputs[ciKeyPassphrase], m.focused == ciKeyPassphrase, w))
	}

	hint := StyleSubtle("Tab/↑↓ navigate  Enter connect  Esc cancel")

	errLine := ""
	if m.err != "" {
		errLine = StyleError.Render(m.err)
	}

	body := strings.Join([]string{
		title,
		"",
		authLine,
		"",
		strings.Join(fieldLines, "\n"),
		"",
		hint,
		errLine,
	}, "\n")

	return StyleModalOverlay.Width(w + 4).Render(body)
}

func renderField(label string, ti textinput.Model, focused bool, w int) string {
	lbl := StyleLabel.Render(label + ": ")
	style := lipgloss.NewStyle().Width(w - len([]rune(label)) - 2)
	if focused {
		style = style.BorderBottom(true).BorderForeground(ColorAccent)
	}
	return lbl + style.Render(ti.View())
}
