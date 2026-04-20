package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaronkiula/sftp-tui/internal/app"
	sftpclient "github.com/aaronkiula/sftp-tui/internal/sftp"
)

func main() {
	var connectFlag string
	flag.StringVar(&connectFlag, "connect", "", "Connect immediately: user@host[:port]")
	flag.Parse()

	model := app.New()

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if connectFlag != "" {
		opts, err := parseConnectFlag(connectFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --connect value: %v\n", err)
			os.Exit(1)
		}
		// Trigger a connection after startup by sending an initial message.
		go func() {
			p.Send(app.ConnectingMsg{Opts: opts})
		}()
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// parseConnectFlag parses user@host or user@host:port.
func parseConnectFlag(s string) (sftpclient.ConnectOpts, error) {
	atIdx := strings.LastIndex(s, "@")
	if atIdx < 0 {
		return sftpclient.ConnectOpts{}, fmt.Errorf("expected user@host[:port]")
	}
	user := s[:atIdx]
	hostPort := s[atIdx+1:]

	host := hostPort
	port := "22"
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		host = hostPort[:idx]
		port = hostPort[idx+1:]
	}

	if user == "" || host == "" {
		return sftpclient.ConnectOpts{}, fmt.Errorf("user and host are required")
	}

	return sftpclient.ConnectOpts{
		User: user,
		Host: host,
		Port: port,
	}, nil
}
