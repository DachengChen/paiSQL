package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Simple Palette inspired by standard terminal dark themes
var (
	// Colors
	ColorPrimary   = lipgloss.Color("255") // White
	ColorSecondary = lipgloss.Color("240") // Dark Gray
	ColorAccent    = lipgloss.Color("39")  // Blue / Cyan
	ColorSuccess   = lipgloss.Color("42")  // Green
	ColorError     = lipgloss.Color("196") // Red
	ColorWarning   = lipgloss.Color("214") // Orange
	ColorDim       = lipgloss.Color("240") // Dimmed text

	// Backgrounds (only used for highlighting lines or headers)
	ColorHighlightBg = lipgloss.Color("236") // Very dark gray background for active items

	// Legacy aliases for compatibility
	ColorBgAlt = ColorHighlightBg
	ColorFgDim = ColorDim
)

// Shared styles - minimal and clean
var (
	// Standard Text
	StyleNormal = lipgloss.NewStyle().Foreground(ColorPrimary)
	StyleDimmed = lipgloss.NewStyle().Foreground(ColorDim)
	StyleBold   = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)

	// Status & Feedback
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning)

	// UI Elements
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary)

	StyleTitle  = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).MarginBottom(1)
	StylePrompt = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

	// Tab Bar
	StyleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent).
			Padding(0, 1)

	StyleTabInactive = lipgloss.NewStyle().
				Foreground(ColorDim).
				Padding(0, 1)

	// Connection List Item (Active)
	StyleListItemActive = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	// Form Focus
	StyleInputFocused = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	// Bottom Bar
	StyleStatusBar = lipgloss.NewStyle().
			Foreground(ColorSecondary)

	// Help Keys
	StyleHelpKey = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleHelpDesc = lipgloss.NewStyle().
			Foreground(ColorDim)
)
