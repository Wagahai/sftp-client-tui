package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type StatusBar struct {
	message       string
	transferCount int
	width         int
}

func NewStatusBar() StatusBar {
	return StatusBar{}
}

func (s *StatusBar) SetMessage(msg string) {
	s.message = msg
}

func (s *StatusBar) SetTransferCount(n int) {
	s.transferCount = n
}

func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

func (s StatusBar) View() string {
	keys := []struct{ key, desc string }{
		{"^N", "new"},
		{"^B", "book"},
		{"^W", "close"},
		{"Tab", "pane"},
		{"F5", "copy"},
		{"F6", "move"},
		{"F7", "mkdir"},
		{"F8", "del"},
		{"?", "help"},
	}

	keyParts := ""
	for _, k := range keys {
		keyParts += StyleStatusKey.Render(k.key) + " " +
			lipgloss.NewStyle().Foreground(ColorText).Render(k.desc) + "  "
	}

	transferInfo := ""
	if s.transferCount > 0 {
		transferInfo = StyleSuccess.Render(fmt.Sprintf(" [%d transfer(s) active]", s.transferCount))
	}

	msg := ""
	if s.message != "" {
		msg = lipgloss.NewStyle().Foreground(ColorYellow).Render(" | " + s.message)
	}

	content := keyParts + transferInfo + msg
	return StyleStatusBar.Width(s.width).Render(content)
}
