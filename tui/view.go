package tui

import tea "github.com/charmbracelet/bubbletea"

// View is the interface every TUI panel must implement.
// Each view is a self-contained Bubble Tea sub-model.
type View interface {
	// Init returns an initial command (e.g. load data).
	Init() tea.Cmd

	// Update handles messages and returns updated view + command.
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the view content (without chrome — tabs/status bar
	// are rendered by the App).
	View() string

	// Name returns the tab label.
	Name() string

	// ShortHelp returns key bindings for the bottom help bar.
	ShortHelp() []KeyBinding

	// SetSize is called when the terminal is resized.
	SetSize(width, height int)

	// WantsTextInput returns true when the view is accepting freeform text
	// input (e.g. chat mode, editing a field). When true, the App should
	// NOT intercept single-character shortcuts like q, 1-6, etc.
	WantsTextInput() bool
}

// KeyBinding describes a keyboard shortcut for the help bar.
type KeyBinding struct {
	Key  string
	Desc string
}
