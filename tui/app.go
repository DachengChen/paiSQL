// app.go is the top-level Bubble Tea model that orchestrates all views.
//
// Flow:
//  1. Start with ConnectView (connection form)
//  2. On successful connection → switch to main multi-tab view
//  3. User can disconnect and return to connection screen
//
// Key design decisions:
//   - Two phases: "connecting" and "connected"
//   - Tab-based navigation between views (when connected)
//   - Command mode (`:`) for psql-like commands
//   - Jump mode (`/`) for quick view switching
//   - Help overlay (`?`) toggled on/off
package tui

import (
	"fmt"
	"strings"

	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/config"
	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const appVersion = "0.1.0"

// Tab indices for connected mode.
const (
	TabSQL = iota
	TabExplain
	TabIndex
	TabStats
	TabLog
	TabAI
)

// AppPhase tracks whether we're connecting or already connected.
type AppPhase int

const (
	PhaseConnect AppPhase = iota
	PhaseMain
)

// InputMode determines what keystrokes do in main phase.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeJump
)

// App is the root Bubble Tea model.
type App struct {
	// Phase management
	phase       AppPhase
	connectView *ConnectView
	store       *config.ConnectionStore

	// Connected state
	views      []View
	activeTab  int
	db         *db.DB
	aiProvider ai.Provider
	cfg        config.Config
	connName   string // name of active connection

	// UI state
	width     int
	height    int
	mode      InputMode
	cmdInput  string
	showHelp  bool
	statusMsg string
}

// NewApp creates the application starting with the connection screen.
func NewApp(store *config.ConnectionStore) *App {
	return &App{
		phase:       PhaseConnect,
		connectView: NewConnectView(store),
		store:       store,
	}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return a.connectView.Init()
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// header(1) + border(2) + helpbar(1) = 4 lines of chrome
		contentH := a.height - 4
		contentW := a.width - 2 // border left+right
		if a.phase == PhaseConnect {
			a.connectView.SetSize(contentW, contentH)
		} else {
			// tab bar removed, status bar moved outside
			// Header(1) + Status(1) + Borders(2) = 4 lines chrome
			viewH := contentH
			for _, v := range a.views {
				v.SetSize(contentW, viewH)
			}
		}
		return a, nil

	case ConnectedMsg:
		// Transition from connect → main phase
		a.db = msg.DB
		a.cfg = msg.Cfg
		a.connName = msg.Conn.Name
		a.phase = PhaseMain
		a.aiProvider = ai.NewPlaceholder()
		a.initViews()
		// Trigger resize for views
		contentW := a.width - 2
		viewH := a.height - 4 // chrome (header1 + status1 + borders2)
		for _, v := range a.views {
			v.SetSize(contentW, viewH)
		}
		return a, a.views[a.activeTab].Init()

	case ConnectErrorMsg:
		// Stay on connect screen, forward error
		updated, cmd := a.connectView.Update(msg)
		a.connectView = updated.(*ConnectView)
		return a, cmd
	}

	if a.phase == PhaseConnect {
		return a.updateConnect(msg)
	}
	return a.updateMain(msg)
}

func (a *App) updateConnect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	updated, cmd := a.connectView.Update(msg)
	a.connectView = updated.(*ConnectView)
	return a, cmd
}

func (a *App) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKey(msg)
	}

	// Forward other messages to active view
	if a.activeTab < len(a.views) {
		updatedView, cmd := a.views[a.activeTab].Update(msg)
		a.views[a.activeTab] = updatedView
		return a, cmd
	}

	return a, nil
}

// initViews creates all main views after connection is established.
func (a *App) initViews() {
	a.views = []View{
		NewSQLView(a.db),
		NewExplainView(a.db),
		NewIndexView(a.db, a.aiProvider),
		NewStatsView(a.db),
		NewLogView(a.db),
		NewAIView(a.aiProvider),
	}
	a.activeTab = 0
}

// handleKey processes keyboard input in main phase.
func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.mode {
	case ModeCommand:
		return a.handleCommandMode(msg)
	case ModeJump:
		return a.handleJumpMode(msg)
	default:
		return a.handleNormalMode(msg)
	}
}

func (a *App) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return a, tea.Quit

	case ":":
		a.mode = ModeCommand
		a.cmdInput = ""
		return a, nil

	case "/":
		a.mode = ModeJump
		a.cmdInput = ""
		return a, nil

	case "?":
		a.showHelp = !a.showHelp
		return a, nil

	case "1":
		return a.switchTab(0)
	case "2":
		return a.switchTab(1)
	case "3":
		return a.switchTab(2)
	case "4":
		return a.switchTab(3)
	case "5":
		return a.switchTab(4)
	case "6":
		return a.switchTab(5)
	}

	// Forward to active view
	if a.activeTab < len(a.views) {
		updatedView, cmd := a.views[a.activeTab].Update(msg)
		a.views[a.activeTab] = updatedView
		return a, cmd
	}

	return a, nil
}

func (a *App) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		cmd := a.executeCommand(a.cmdInput)
		a.mode = ModeNormal
		a.cmdInput = ""
		return a, cmd

	case "escape":
		a.mode = ModeNormal
		a.cmdInput = ""
		return a, nil

	case "backspace":
		if len(a.cmdInput) > 0 {
			a.cmdInput = a.cmdInput[:len(a.cmdInput)-1]
		}
		return a, nil

	default:
		if len(msg.String()) == 1 {
			a.cmdInput += msg.String()
		}
		return a, nil
	}
}

func (a *App) handleJumpMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		a.jumpToView(a.cmdInput)
		a.mode = ModeNormal
		a.cmdInput = ""
		return a, a.views[a.activeTab].Init()

	case "escape":
		a.mode = ModeNormal
		a.cmdInput = ""
		return a, nil

	case "backspace":
		if len(a.cmdInput) > 0 {
			a.cmdInput = a.cmdInput[:len(a.cmdInput)-1]
		}
		return a, nil

	default:
		if len(msg.String()) == 1 {
			a.cmdInput += msg.String()
		}
		return a, nil
	}
}

func (a *App) switchTab(idx int) (tea.Model, tea.Cmd) {
	if idx >= 0 && idx < len(a.views) {
		a.activeTab = idx
		return a, a.views[a.activeTab].Init()
	}
	return a, nil
}

func (a *App) jumpToView(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, v := range a.views {
		if strings.Contains(strings.ToLower(v.Name()), name) {
			a.activeTab = i
			return
		}
	}
	a.statusMsg = "view not found: " + name
}

func (a *App) executeCommand(input string) tea.Cmd {
	input = strings.TrimSpace(input)
	switch {
	case input == "q" || input == "quit":
		return tea.Quit
	case input == "disconnect":
		a.disconnect()
		return nil
	case strings.HasPrefix(input, "dt"):
		a.activeTab = TabSQL
		a.statusMsg = "listing tables..."
		return a.views[TabSQL].Init()
	default:
		a.statusMsg = "unknown command: " + input
		return nil
	}
}

func (a *App) disconnect() {
	if a.db != nil {
		a.db.Close()
		a.db = nil
	}
	a.phase = PhaseConnect
	a.views = nil
	a.activeTab = 0
	a.statusMsg = ""
}

// View implements tea.Model.
// View implements tea.Model.
func (a *App) View() string {
	if a.width == 0 {
		return "loading..."
	}

	// ── Header bar ──
	header := a.renderHeader()

	// ── Help bar ──
	var helpBar string

	frameBox := StyleBorder.
		Width(a.width - 2).
		Height(a.height - 3) // header + helpbar + border chrome

	if a.phase == PhaseConnect {
		content := a.connectView.View()
		helpBar = a.renderConnectHelpBar()
		frame := frameBox.Render(content)
		return lipgloss.JoinVertical(lipgloss.Left, header, frame, helpBar)
	}

	// Main phase: content inside a border, status bar at bottom outside border
	var innerSections []string
	// innerSections = append(innerSections, a.renderTabBar()) // User requested removal

	if a.showHelp {
		innerSections = append(innerSections, a.renderHelp())
	} else if a.activeTab < len(a.views) {
		innerSections = append(innerSections, a.views[a.activeTab].View())
	}

	innerContent := lipgloss.JoinVertical(lipgloss.Left, innerSections...)

	// Frame height = Total - Header(1) - Status(1)
	frame := StyleBorder.
		Width(a.width - 2).
		Height(a.height - 2).
		Render(innerContent)

	statusBar := a.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, header, frame, statusBar)
}

// renderHeader draws a simple text bar: logo + version + connection info.
func (a *App) renderHeader() string {
	logo := StyleBold.Render("🐘 paiSQL")
	version := StyleDimmed.Render(" v" + appVersion)

	left := logo + version

	// Right side: connection info + screen size
	var right string
	if a.phase == PhaseMain && a.connName != "" {
		right = StyleSuccess.Render("⚡ " + a.connName)
	} else if a.phase == PhaseMain {
		right = StyleSuccess.Render("⚡ connected")
	}

	// Add dimensions dimmed
	right += "  " + StyleDimmed.Render(fmt.Sprintf("%d×%d", a.width, a.height))

	// Fill gap with spaces (transparent)
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	filler := strings.Repeat(" ", gap)

	// No background color set - relies on terminal default
	return lipgloss.NewStyle().
		Width(a.width).
		Render(left + filler + right)
}

func (a *App) renderConnectHelpBar() string {
	help := a.connectView.ShortHelp()
	var parts []string
	for _, h := range help {
		key := StyleHelpKey.Render(h.Key)
		desc := StyleHelpDesc.Render(h.Desc)
		parts = append(parts, key+" "+desc)
	}

	content := strings.Join(parts, StyleDimmed.Render("  │  "))

	return lipgloss.NewStyle().
		Width(a.width).
		Padding(0, 1).
		Render(content)
}

func (a *App) renderTabBar() string {
	var tabs []string
	for i, v := range a.views {
		label := v.Name()
		if i == a.activeTab {
			tabs = append(tabs, StyleTabActive.Render(label))
		} else {
			tabs = append(tabs, StyleTabInactive.Render(label))
		}
	}

	// Connection indicator
	connLabel := " ⚡ " + a.connName
	if a.connName == "" {
		connLabel = " ⚡ connected"
	}
	connIndicator := lipgloss.NewStyle().Foreground(ColorSuccess).Render(connLabel)

	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	tabBar += "  " + connIndicator

	return lipgloss.NewStyle().Width(a.width).Background(ColorBgAlt).Render(tabBar)
}

func (a *App) renderStatusBar() string {
	var content string

	switch a.mode {
	case ModeCommand:
		content = StylePrompt.Render(":") + a.cmdInput + "█"
	case ModeJump:
		content = StylePrompt.Render("/") + a.cmdInput + "█"
	default:
		if a.statusMsg != "" {
			content = a.statusMsg
		} else {
			helpItems := a.getHelpItems()
			var parts []string
			for _, h := range helpItems {
				parts = append(parts,
					StyleHelpKey.Render(h.Key)+" "+StyleHelpDesc.Render(h.Desc))
			}
			content = strings.Join(parts, "  │  ")
		}
	}

	return StyleStatusBar.Width(a.width).Render(content)
}

func (a *App) getHelpItems() []KeyBinding {
	global := []KeyBinding{
		{Key: "tab", Desc: "next view"},
		{Key: "1-6", Desc: "jump tab"},
		{Key: ":", Desc: "command"},
		{Key: "/", Desc: "find view"},
		{Key: "?", Desc: "help"},
		{Key: "q", Desc: "quit"},
	}
	if a.activeTab < len(a.views) {
		return append(a.views[a.activeTab].ShortHelp(), global...)
	}
	return global
}

func (a *App) renderHelp() string {
	help := []string{
		StyleTitle.Render("⌨ paiSQL Keyboard Shortcuts"),
		"",
		StyleHelpKey.Render("Tab / Shift+Tab") + "  Switch between views",
		StyleHelpKey.Render("1-6") + "              Jump to view by number",
		StyleHelpKey.Render(":") + "                Command mode (e.g. :dt, :quit, :disconnect)",
		StyleHelpKey.Render("/") + "                Jump to view by name",
		StyleHelpKey.Render("?") + "                Toggle this help",
		StyleHelpKey.Render("q / Ctrl+C") + "       Quit",
		"",
		StyleTitle.Render("View-specific"),
		"",
		StyleHelpKey.Render("↑/↓ j/k") + "         Vertical scroll",
		StyleHelpKey.Render("←/→ h/l") + "         Horizontal scroll",
		StyleHelpKey.Render("PgUp/PgDn") + "        Page up/down",
		StyleHelpKey.Render("Enter") + "            Execute query (SQL view)",
		StyleHelpKey.Render("Ctrl+E") + "           Explain query",
		"",
		StyleTitle.Render("Commands"),
		"",
		StyleHelpKey.Render(":disconnect") + "      Return to connection screen",
		StyleHelpKey.Render(":dt") + "              List tables",
		StyleHelpKey.Render(":quit") + "            Quit",
		"",
		StyleDimmed.Render("Press ? to close"),
	}

	contentHeight := a.height - 3
	box := lipgloss.NewStyle().
		Width(a.width-4).
		Height(contentHeight).
		Padding(1, 2).
		Render(strings.Join(help, "\n"))

	return box
}
