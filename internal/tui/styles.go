package tui

import "github.com/charmbracelet/lipgloss"

var (
	// 24-bit Truecolor Palette
	ColorPurple    = lipgloss.Color("#7D56F4")
	ColorMagenta   = lipgloss.Color("#D946EF")
	ColorCyan      = lipgloss.Color("#00F5D4")
	ColorBlue      = lipgloss.Color("#00BBF9")
	ColorGreen     = lipgloss.Color("#04B575")
	ColorYellow    = lipgloss.Color("#FEE440")
	ColorPink      = lipgloss.Color("#FF007F")
	ColorRed       = lipgloss.Color("#FF5555")
	ColorBgDark    = lipgloss.Color("#181825")
	ColorBgCard    = lipgloss.Color("#1E1E2E")
	ColorFgMuted   = lipgloss.Color("#6C7086")
	ColorFgLight   = lipgloss.Color("#CDD6F4")
	ColorFgBright  = lipgloss.Color("#FFFFFF")

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
