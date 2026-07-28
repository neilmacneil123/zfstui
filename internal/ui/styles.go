package ui

import "github.com/charmbracelet/lipgloss"

// Palette — dark terminal theme with cyan/magenta accents.
var (
	ColorBg       = lipgloss.Color("#0d1117")
	ColorFg       = lipgloss.Color("#e6edf3")
	ColorTitle    = lipgloss.Color("#22d3ee") // cyan
	ColorAccent   = lipgloss.Color("#e879f9") // magenta
	ColorOk       = lipgloss.Color("#4ade80") // green
	ColorDanger   = lipgloss.Color("#f87171") // red
	ColorWarn     = lipgloss.Color("#fbbf24") // amber
	ColorMuted    = lipgloss.Color("#6b7280") // gray
	ColorBorder   = lipgloss.Color("#334155")
	ColorSelectBg = lipgloss.Color("#22d3ee")
	ColorSelectFg = lipgloss.Color("#0d1117")
	ColorDimBg    = lipgloss.Color("#161b22")
)

// Shared styles.
var (
	AppStyle = lipgloss.NewStyle().
			Foreground(ColorFg)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorTitle)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	BannerBorderStyle = lipgloss.NewStyle().
				Foreground(ColorTitle).
				Bold(true)

	BannerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorTitle)

	BannerSubStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	MenuItemStyle = lipgloss.NewStyle().
			Foreground(ColorFg).
			PaddingLeft(2)

	MenuSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorSelectFg).
				Background(ColorSelectBg).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)

	MenuCursorStyle = lipgloss.NewStyle().
			Foreground(ColorTitle).
			Bold(true)

	MenuDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(4)

	OkStyle = lipgloss.NewStyle().
		Foreground(ColorOk).
		Bold(true)

	DangerStyle = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	WarnStyle = lipgloss.NewStyle().
			Foreground(ColorWarn).
			Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorFg).
			Background(ColorDimBg).
			Padding(0, 1)

	HelpBarStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorTitle).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	HelpPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(1, 2).
			Foreground(ColorFg)

	PlaceholderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent)

	PlaceholderBodyStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	SectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorTitle).
			MarginBottom(1)
)

// StatusDryRun returns a styled dry-run indicator.
func StatusDryRun(on bool) string {
	if on {
		return WarnStyle.Render("DRY-RUN")
	}
	return OkStyle.Render("LIVE")
}

// HelpHint formats a key+description pair for the help bar.
func HelpHint(key, desc string) string {
	return HelpKeyStyle.Render(key) + HelpDescStyle.Render(" "+desc)
}
