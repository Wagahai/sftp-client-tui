package ui

import "github.com/charmbracelet/lipgloss"

var (
	ColorBg        = lipgloss.Color("#1a1b26")
	ColorBgAlt     = lipgloss.Color("#24283b")
	ColorBorder    = lipgloss.Color("#3b4261")
	ColorBorderFocus = lipgloss.Color("#7aa2f7")
	ColorText      = lipgloss.Color("#c0caf5")
	ColorSubtle    = lipgloss.Color("#565f89")
	ColorAccent    = lipgloss.Color("#7aa2f7")
	ColorGreen     = lipgloss.Color("#9ece6a")
	ColorYellow    = lipgloss.Color("#e0af68")
	ColorRed       = lipgloss.Color("#f7768e")
	ColorCyan      = lipgloss.Color("#7dcfff")
	ColorMagenta   = lipgloss.Color("#bb9af7")
	ColorSelected  = lipgloss.Color("#283457")
	ColorTabActive = lipgloss.Color("#7aa2f7")
	ColorTabInact  = lipgloss.Color("#3b4261")

	StyleBase = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorText)

	StyleBorderNormal = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder)

	StyleBorderFocus = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderFocus)

	StyleTabActive = lipgloss.NewStyle().
			Background(ColorAccent).
			Foreground(ColorBg).
			Padding(0, 1).
			Bold(true)

	StyleTabInactive = lipgloss.NewStyle().
				Background(ColorTabInact).
				Foreground(ColorText).
				Padding(0, 1)

	StyleTabBar = lipgloss.NewStyle().
			Background(ColorBgAlt).
			Padding(0, 0)

	StyleStatusBar = lipgloss.NewStyle().
			Background(ColorBgAlt).
			Foreground(ColorSubtle).
			Padding(0, 1)

	StyleStatusKey = lipgloss.NewStyle().
			Background(ColorAccent).
			Foreground(ColorBg).
			Padding(0, 1).
			Bold(true)

	StylePaneHeader = lipgloss.NewStyle().
			Background(ColorBgAlt).
			Foreground(ColorAccent).
			Bold(true).
			Padding(0, 1)

	StyleFilename = lipgloss.NewStyle().
			Foreground(ColorText)

	StyleDir = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleSelectedItem = lipgloss.NewStyle().
				Background(ColorSelected).
				Foreground(ColorCyan).
				Bold(true)

	StyleCursorItem = lipgloss.NewStyle().
			Background(ColorBgAlt).
			Foreground(ColorYellow).
			Bold(true)

	StyleCursorSelectedItem = lipgloss.NewStyle().
				Background(ColorSelected).
				Foreground(ColorYellow).
				Bold(true)

	StyleFileSize = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	StyleFilterPrompt = lipgloss.NewStyle().
				Foreground(ColorYellow).
				Bold(true)

	StyleHelp = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Background(ColorBgAlt).
			Foreground(ColorText).
			Padding(1, 2)

	StyleModalOverlay = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorMagenta).
				Background(ColorBgAlt).
				Foreground(ColorText).
				Padding(1, 2)

	StyleLabel = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(ColorGreen)

	StyleProgressBar = lipgloss.NewStyle().
				Foreground(ColorCyan)
)

func TruncateStr(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return string(runes[:maxWidth])
	}
	return string(runes[:maxWidth-3]) + "..."
}

func PadRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	for len(runes) < width {
		runes = append(runes, ' ')
	}
	return string(runes)
}
