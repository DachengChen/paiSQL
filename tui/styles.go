package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette — dark theme with accent colors.
var (
	ColorPrimary     = lipgloss.Color("#7C3AED") // Violet
	ColorSecondary   = lipgloss.Color("#06B6D4") // Cyan
	ColorAccent      = lipgloss.Color("#F59E0B") // Amber
	ColorSuccess     = lipgloss.Color("#10B981") // Emerald
	ColorError       = lipgloss.Color("#EF4444") // Red
	ColorWarning     = lipgloss.Color("#F59E0B") // Amber
	ColorMuted       = lipgloss.Color("#6B7280") // Gray
	ColorBg          = lipgloss.Color("#1E1E2E") // Dark bg
	ColorBgAlt       = lipgloss.Color("#2D2D3F") // Slightly lighter
	ColorFg          = lipgloss.Color("#CDD6F4") // Light text
	ColorFgDim       = lipgloss.Color("#6C7086") // Dimmed text
	ColorBorder      = lipgloss.Color("#45475A") // Border
	ColorTabActive   = lipgloss.Color("#CBA6F7") // Active tab
	ColorTabInactive = lipgloss.Color("#585B70") // Inactive tab
)

// Shared styles used across views.
var (
	StyleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBg).
			Background(ColorTabActive).
			Padding(0, 2)

	StyleTabInactive = lipgloss.NewStyle().
				Foreground(ColorFgDim).
				Background(ColorBgAlt).
				Padding(0, 2)

	StyleStatusBar = lipgloss.NewStyle().
			Foreground(ColorFg).
			Background(ColorBgAlt).
			Padding(0, 1).
			Width(80) // overridden at render time

	StyleHelpKey = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleHelpDesc = lipgloss.NewStyle().
			Foreground(ColorFgDim)

	StyleTitle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			MarginBottom(1)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	StylePrompt = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	StyleDimmed = lipgloss.NewStyle().
			Foreground(ColorFgDim)

	StyleWarning = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)
)
