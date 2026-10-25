package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	// 24-bit Truecolor Palette
	ColorPurple   = lipgloss.Color("#7D56F4")
	ColorMagenta  = lipgloss.Color("#D946EF")
	ColorCyan     = lipgloss.Color("#00F5D4")
	ColorBlue     = lipgloss.Color("#00BBF9")
	ColorGreen    = lipgloss.Color("#04B575")
	ColorYellow   = lipgloss.Color("#FEE440")
	ColorPink     = lipgloss.Color("#FF007F")
	ColorRed      = lipgloss.Color("#FF5555")
	ColorBgDark   = lipgloss.Color("#181825")
	ColorBgCard   = lipgloss.Color("#1E1E2E")
	ColorFgMuted  = lipgloss.Color("#6C7086")
	ColorFgLight  = lipgloss.Color("#CDD6F4")
	ColorFgBright = lipgloss.Color("#FFFFFF")

	// Base Typography
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorFgMuted)

	// Cards and Boxes
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPurple).
			Padding(1, 2).
			Background(ColorBgCard)

	FocusCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Padding(1, 2).
			Background(ColorBgCard)

	DocBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorYellow).
			Padding(1, 2).
			Background(lipgloss.Color("#252438"))

	// Status Badges
	BadgeConnected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBgDark).
			Background(ColorGreen).
			Padding(0, 1).
			SetString("CONNECTED")

	BadgeSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBgDark).
			Background(ColorCyan).
			Padding(0, 1).
			SetString("TARGET")

	BadgeDisconnected = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorFgBright).
				Background(ColorRed).
				Padding(0, 1).
				SetString("DISCONNECTED")

	BadgeReady = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBgDark).
			Background(ColorCyan).
			Padding(0, 1).
			SetString("READY")

	BadgeWait = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBgDark).
			Background(ColorYellow).
			Padding(0, 1).
			SetString("WAITING")

	BadgeActiveTarget = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorBgDark).
				Background(ColorCyan).
				Padding(0, 1).
				SetString("● ACTIVE TARGET")

	BadgeWindows = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFgBright).
			Background(lipgloss.Color("#0078D4")).
			Padding(0, 1).
			SetString("🪟 Windows")

	BadgeMacOS = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBgDark).
			Background(lipgloss.Color("#E2E8F0")).
			Padding(0, 1).
			SetString("🍏 macOS")

	// Menu Item Styles
	MenuItemStyle = lipgloss.NewStyle().
			Foreground(ColorFgLight).
			PaddingLeft(2)

	MenuSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan).
				Background(lipgloss.Color("#313244")).
				PaddingLeft(1)

	KeyBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorYellow)

	// Helper / Footers
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorFgMuted).
			PaddingTop(1)

	NoticeStyle = lipgloss.NewStyle().
			Foreground(ColorMagenta).
			Bold(true)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)
)

// calculateCardWidth calculates a responsive card width clamped between 74 and 100 columns.
func calculateCardWidth(termWidth int) int {
	w := 80
	if termWidth > 0 {
		w = termWidth - 4
		if w < 74 {
			w = 74
		} else if w > 100 {
			w = 100
		}
	}
	return w
}

func padRight(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncate(s string, maxVisualWidth int) string {
	if lipgloss.Width(s) <= maxVisualWidth {
		return s
	}
	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)
	target := maxVisualWidth - ellipsisWidth
	if target <= 0 {
		return ellipsis
	}
	var res strings.Builder
	curWidth := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if curWidth+rw > target {
			break
		}
		res.WriteRune(r)
		curWidth += rw
	}
	res.WriteString(ellipsis)
	return res.String()
}
