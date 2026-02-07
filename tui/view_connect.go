// view_connect.go — Connection setup screen with k9s-style bordered frame.
//
// This is the first screen shown when paiSQL starts. The user can:
//   - Fill in connection details (host, port, user, password, etc.)
//   - Toggle SSH tunnel and configure SSH settings
//   - Select from saved connections
//   - Save / delete connection profiles
//   - Press Enter on "Connect" to establish the connection
//
// After successful connection, the App switches to the main multi-tab view.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/DachengChen/paiSQL/config"
	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Field IDs for the connection form.
const (
	fieldSaved = iota
	fieldName
	fieldHost
	fieldPort
	fieldUser
	fieldPassword
	fieldDatabase
	fieldSSLMode
	fieldSSHEnabled
	fieldSSHHost
	fieldSSHPort
	fieldSSHUser
	fieldSSHKey
	fieldConnect
	fieldSave
	fieldDelete
	fieldCount // sentinel
)

// fieldLabel maps field IDs to display labels.
var fieldLabels = map[int]string{
	fieldSaved:      "Saved",
	fieldName:       "Name",
	fieldHost:       "Host",
	fieldPort:       "Port",
	fieldUser:       "User",
	fieldPassword:   "Password",
	fieldDatabase:   "Database",
	fieldSSLMode:    "SSL Mode",
	fieldSSHEnabled: "SSH Tunnel",
	fieldSSHHost:    "SSH Host",
	fieldSSHPort:    "SSH Port",
	fieldSSHUser:    "SSH User",
	fieldSSHKey:     "SSH Key",
	fieldConnect:    "Connect",
	fieldSave:       "Save",
	fieldDelete:     "Delete",
}

// SSL mode options for cycling.
var sslModes = []string{"disable", "require", "verify-ca", "verify-full", "prefer"}

// ConnectView is the connection setup form.
type ConnectView struct {
	store      *config.ConnectionStore
	fields     []string // field values indexed by field ID
	focusField int
	savedIdx   int  // selected index in saved connections list
	editing    bool // true when typing in a field
	err        error
	statusMsg  string
	connecting bool
	width      int
	height     int
}

// ConnectedMsg is sent when a DB connection is successfully established.
type ConnectedMsg struct {
	DB   *db.DB
	Cfg  config.Config
	Conn config.Connection
}

// ConnectErrorMsg is sent when connection fails.
type ConnectErrorMsg struct {
	Err error
}

func NewConnectView(store *config.ConnectionStore) *ConnectView {
	v := &ConnectView{
		store:      store,
		fields:     make([]string, fieldCount),
		focusField: fieldHost,
	}

	def := config.DefaultConnection()
	v.fields[fieldHost] = def.Host
	v.fields[fieldPort] = def.Port
	v.fields[fieldUser] = def.User
	v.fields[fieldPassword] = def.Password
	v.fields[fieldDatabase] = def.Database
	v.fields[fieldSSLMode] = def.SSLMode
	v.fields[fieldSSHEnabled] = "no"
	v.fields[fieldSSHPort] = def.SSH.Port

	if len(store.Connections) > 0 {
		v.loadSavedConnection(0)
		v.focusField = fieldSaved
	}

	return v
}

func (v *ConnectView) Name() string { return "Connect" }

func (v *ConnectView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *ConnectView) ShortHelp() []KeyBinding {
	if v.editing {
		return []KeyBinding{
			{Key: "Enter", Desc: "confirm"},
			{Key: "Esc", Desc: "cancel"},
			{Key: "Ctrl+U", Desc: "clear"},
		}
	}
	return []KeyBinding{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "Enter", Desc: "edit/action"},
		{Key: "Tab", Desc: "connect"},
		{Key: "Ctrl+C", Desc: "quit"},
	}
}

func (v *ConnectView) Init() tea.Cmd { return nil }

func (v *ConnectView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if v.editing {
			return v.handleEditing(msg)
		}
		return v.handleNavigation(msg)

	case ConnectedMsg:
		return v, nil

	case ConnectErrorMsg:
		v.connecting = false
		v.err = msg.Err
		v.statusMsg = ""
		return v, nil
	}

	return v, nil
}

func (v *ConnectView) handleNavigation(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		v.focusField--
		if v.focusField < 0 {
			v.focusField = fieldCount - 1
		}
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			v.focusField = fieldSSHEnabled - 1
		}
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField = fieldCount - 1
		}

	case "down", "j":
		v.focusField++
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			v.focusField = fieldConnect
		}
		if v.focusField >= fieldCount {
			v.focusField = 0
		}
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField = fieldName
		}

	case "enter":
		return v.handleAction()

	case "tab":
		v.focusField = fieldConnect
		return v, nil

	case "left", "h":
		if v.focusField == fieldSaved && len(v.store.Connections) > 0 {
			v.savedIdx--
			if v.savedIdx < 0 {
				v.savedIdx = len(v.store.Connections) - 1
			}
			v.loadSavedConnection(v.savedIdx)
			return v, nil
		}
		if v.focusField == fieldSSLMode {
			v.cycleSSLMode(-1)
			return v, nil
		}
		// Default: behave like UP
		v.focusField--
		if v.focusField < 0 {
			v.focusField = fieldCount - 1
		}
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			v.focusField = fieldSSHEnabled - 1
		}
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField = fieldCount - 1
		}

	case "right", "l":
		if v.focusField == fieldSaved && len(v.store.Connections) > 0 {
			v.savedIdx++
			if v.savedIdx >= len(v.store.Connections) {
				v.savedIdx = 0
			}
			v.loadSavedConnection(v.savedIdx)
			return v, nil
		}
		if v.focusField == fieldSSLMode {
			v.cycleSSLMode(1)
			return v, nil
		}
		// Default: behave like DOWN
		v.focusField++
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			v.focusField = fieldConnect
		}
		if v.focusField >= fieldCount {
			v.focusField = 0
		}
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField = fieldName
		}

	case "q", "ctrl+c":
		return v, tea.Quit
	}

	return v, nil
}

func (v *ConnectView) handleEditing(msg tea.KeyMsg) (View, tea.Cmd) {
	field := v.focusField

	switch msg.String() {
	case "enter", "escape":
		v.editing = false
		return v, nil

	case "backspace":
		if len(v.fields[field]) > 0 {
			v.fields[field] = v.fields[field][:len(v.fields[field])-1]
		}

	case "ctrl+u":
		v.fields[field] = ""

	default:
		ch := msg.String()
		if len(ch) == 1 || ch == " " {
			v.fields[field] += ch
		}
	}

	return v, nil
}

func (v *ConnectView) handleAction() (View, tea.Cmd) {
	switch v.focusField {
	case fieldSaved:
		// Directly connect using the selected saved connection
		return v, v.connect()

	case fieldSSHEnabled:
		if v.sshEnabled() {
			v.fields[fieldSSHEnabled] = "no"
		} else {
			v.fields[fieldSSHEnabled] = "yes"
		}
		return v, nil

	case fieldSSLMode:
		v.cycleSSLMode(1)
		return v, nil

	case fieldConnect:
		return v, v.connect()

	case fieldSave:
		return v, v.saveConnection()

	case fieldDelete:
		return v, v.deleteConnection()

	default:
		v.editing = true
		return v, nil
	}
}

func (v *ConnectView) connect() tea.Cmd {
	conn := v.buildConnection()
	cfg := config.FromConnection(conn)

	v.connecting = true
	v.statusMsg = "Connecting..."
	v.err = nil

	return func() tea.Msg {
		database, err := db.Connect(context.Background(), cfg)
		if err != nil {
			return ConnectErrorMsg{Err: err}
		}
		return ConnectedMsg{DB: database, Cfg: cfg, Conn: conn}
	}
}

func (v *ConnectView) saveConnection() tea.Cmd {
	name := strings.TrimSpace(v.fields[fieldName])
	if name == "" {
		v.err = fmt.Errorf("enter a connection name first")
		return nil
	}

	conn := v.buildConnection()
	conn.Name = name
	v.store.Add(conn)

	if err := v.store.Save(); err != nil {
		v.err = err
		return nil
	}

	v.statusMsg = fmt.Sprintf("Connection '%s' saved!", name)
	v.err = nil

	for i, c := range v.store.Connections {
		if c.Name == name {
			v.savedIdx = i
			break
		}
	}

	return nil
}

func (v *ConnectView) deleteConnection() tea.Cmd {
	if len(v.store.Connections) == 0 {
		return nil
	}

	name := v.store.Connections[v.savedIdx].Name
	v.store.Delete(name)

	if err := v.store.Save(); err != nil {
		v.err = err
		return nil
	}

	v.statusMsg = fmt.Sprintf("Connection '%s' deleted.", name)
	v.err = nil

	if v.savedIdx >= len(v.store.Connections) {
		v.savedIdx = 0
	}

	return nil
}

func (v *ConnectView) buildConnection() config.Connection {
	return config.Connection{
		Name:     strings.TrimSpace(v.fields[fieldName]),
		Host:     v.fields[fieldHost],
		Port:     v.fields[fieldPort],
		User:     v.fields[fieldUser],
		Password: v.fields[fieldPassword],
		Database: v.fields[fieldDatabase],
		SSLMode:  v.fields[fieldSSLMode],
		SSH: config.SSHEntry{
			Enabled: v.sshEnabled(),
			Host:    v.fields[fieldSSHHost],
			Port:    v.fields[fieldSSHPort],
			User:    v.fields[fieldSSHUser],
			KeyPath: v.fields[fieldSSHKey],
		},
	}
}

func (v *ConnectView) loadSavedConnection(idx int) {
	if idx < 0 || idx >= len(v.store.Connections) {
		return
	}
	c := v.store.Connections[idx]
	v.fields[fieldName] = c.Name
	v.fields[fieldHost] = c.Host
	v.fields[fieldPort] = c.Port
	v.fields[fieldUser] = c.User
	v.fields[fieldPassword] = c.Password
	v.fields[fieldDatabase] = c.Database
	v.fields[fieldSSLMode] = c.SSLMode
	if c.SSH.Enabled {
		v.fields[fieldSSHEnabled] = "yes"
	} else {
		v.fields[fieldSSHEnabled] = "no"
	}
	v.fields[fieldSSHHost] = c.SSH.Host
	v.fields[fieldSSHPort] = c.SSH.Port
	v.fields[fieldSSHUser] = c.SSH.User
	v.fields[fieldSSHKey] = c.SSH.KeyPath
	v.savedIdx = idx
}

func (v *ConnectView) sshEnabled() bool {
	return v.fields[fieldSSHEnabled] == "yes"
}

func (v *ConnectView) isSSHField(f int) bool {
	return f >= fieldSSHHost && f <= fieldSSHKey
}

func (v *ConnectView) cycleSSLMode(dir int) {
	current := v.fields[fieldSSLMode]
	idx := 0
	for i, m := range sslModes {
		if m == current {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(sslModes)) % len(sslModes)
	v.fields[fieldSSLMode] = sslModes[idx]
}

// ─────────────────────────────────────────────────────────────
// View rendering — k9s-style bordered frame
// ─────────────────────────────────────────────────────────────

func (v *ConnectView) View() string {
	formWidth := 60
	if v.width > 0 && formWidth > v.width-4 {
		formWidth = v.width - 4
	}
	inputWidth := formWidth - 20 // label takes ~18 chars

	var formLines []string

	// ── Saved Connections ──
	if len(v.store.Connections) > 0 {
		formLines = append(formLines, v.sectionHeader("Saved Connections", formWidth-2))
		savedLine := ""
		for i, c := range v.store.Connections {
			label := c.Name
			if i == v.savedIdx {
				if v.focusField == fieldSaved {
					savedLine += StyleListItemActive.Render(" ► " + label + " ")
				} else {
					savedLine += lipgloss.NewStyle().
						Foreground(ColorAccent).
						Render(" ► " + label + " ")
				}
			} else {
				savedLine += StyleDimmed.Render("   " + label + " ")
			}
		}
		formLines = append(formLines, savedLine)
		formLines = append(formLines, "")
	}

	// ── Connection ──
	formLines = append(formLines, v.sectionHeader("Connection", formWidth-2))
	formLines = append(formLines, v.renderField(fieldName, inputWidth))
	formLines = append(formLines, v.renderField(fieldHost, inputWidth))
	formLines = append(formLines, v.renderField(fieldPort, inputWidth))
	formLines = append(formLines, v.renderField(fieldUser, inputWidth))
	formLines = append(formLines, v.renderPasswordField(inputWidth))
	formLines = append(formLines, v.renderField(fieldDatabase, inputWidth))
	formLines = append(formLines, v.renderSelectField(fieldSSLMode, inputWidth))
	formLines = append(formLines, "")

	// ── SSH Tunnel ──
	formLines = append(formLines, v.sectionHeader("SSH Tunnel", formWidth-2))
	formLines = append(formLines, v.renderToggleField(fieldSSHEnabled))

	if v.sshEnabled() {
		formLines = append(formLines, v.renderField(fieldSSHHost, inputWidth))
		formLines = append(formLines, v.renderField(fieldSSHPort, inputWidth))
		formLines = append(formLines, v.renderField(fieldSSHUser, inputWidth))
		formLines = append(formLines, v.renderField(fieldSSHKey, inputWidth))
	}

	formLines = append(formLines, "")

	// ── Action buttons ──
	btnLine := v.renderButton(fieldConnect) + "  " +
		v.renderButton(fieldSave) + "  " +
		v.renderButton(fieldDelete)
	formLines = append(formLines, btnLine)

	// ── Status line ──
	if v.connecting {
		formLines = append(formLines, "")
		formLines = append(formLines, StyleDimmed.Render("⏳ "+v.statusMsg))
	} else if v.err != nil {
		formLines = append(formLines, "")
		formLines = append(formLines, StyleError.Render("✗ "+v.err.Error()))
	} else if v.statusMsg != "" {
		formLines = append(formLines, "")
		formLines = append(formLines, StyleSuccess.Render("✓ "+v.statusMsg))
	}

	// Wrap form content in a bordered box
	formContent := strings.Join(formLines, "\n")

	formBox := StyleBorder.
		Padding(1, 2).
		Width(formWidth).
		Render(formContent)

	// Center the form in the available space
	centered := lipgloss.NewStyle().
		Width(v.width).
		Height(v.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(formBox)

	return centered
}

// sectionHeader renders a k9s-style section divider: ── Label ──
func (v *ConnectView) sectionHeader(label string, width int) string {
	labelRendered := lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true).
		Render(label)

	// Calculate dashes on each side
	labelLen := len(label)
	remaining := width - labelLen - 4 // " label " + some dashes
	if remaining < 4 {
		remaining = 4
	}
	left := 2
	right := remaining - left

	dashes := StyleDimmed.Render(strings.Repeat("─", left)) +
		" " + labelRendered + " " +
		StyleDimmed.Render(strings.Repeat("─", right))

	return dashes
}

// renderField renders a form input field.
func (v *ConnectView) renderField(id, inputWidth int) string {
	label := fieldLabels[id]
	value := v.fields[id]
	focused := v.focusField == id

	labelStr := lipgloss.NewStyle().
		Width(16).
		Foreground(ColorDim).
		Render(label)

	if focused {
		labelStr = lipgloss.NewStyle().
			Width(16).
			Foreground(ColorAccent).
			Bold(true).
			Render("▸ " + label)
	}

	if focused {
		cursor := "█"
		if !v.editing {
			cursor = ""
		}
		// Highlight active input with just a slightly different foreground or bold
		inputBox := lipgloss.NewStyle().
			Width(inputWidth).
			Foreground(ColorPrimary).
			Render(value + cursor)
		return labelStr + " " + inputBox
	}

	return labelStr + " " + StyleDimmed.Render(value)
}

func (v *ConnectView) renderPasswordField(inputWidth int) string {
	value := v.fields[fieldPassword]
	focused := v.focusField == fieldPassword
	masked := strings.Repeat("•", len(value))

	labelStr := lipgloss.NewStyle().
		Width(16).
		Foreground(ColorDim).
		Render("Password")

	if focused {
		labelStr = lipgloss.NewStyle().
			Width(16).
			Foreground(ColorAccent).
			Bold(true).
			Render("▸ Password")
	}

	if focused {
		cursor := "█"
		if !v.editing {
			cursor = ""
		}
		inputBox := lipgloss.NewStyle().
			Width(inputWidth).
			Foreground(ColorPrimary).
			Render(masked + cursor)
		return labelStr + " " + inputBox
	}

	return labelStr + " " + StyleDimmed.Render(masked)
}

func (v *ConnectView) renderSelectField(id, inputWidth int) string {
	label := fieldLabels[id]
	value := v.fields[id]
	focused := v.focusField == id

	labelStr := lipgloss.NewStyle().
		Width(16).
		Foreground(ColorDim).
		Render(label)

	if focused {
		labelStr = lipgloss.NewStyle().
			Width(16).
			Foreground(ColorAccent).
			Bold(true).
			Render("▸ " + label)

		selectBox := lipgloss.NewStyle().
			Foreground(ColorAccent).
			Render(" ◂ " + value + " ▸ ")
		return labelStr + " " + selectBox
	}

	return labelStr + " " + StyleDimmed.Render(value)
}

func (v *ConnectView) renderToggleField(id int) string {
	label := fieldLabels[id]
	enabled := v.fields[id] == "yes"
	focused := v.focusField == id

	labelStr := lipgloss.NewStyle().
		Width(16).
		Foreground(ColorDim).
		Render(label)

	if focused {
		labelStr = lipgloss.NewStyle().
			Width(16).
			Foreground(ColorAccent).
			Bold(true).
			Render("▸ " + label)
	}

	if enabled {
		toggle := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("● Enabled")
		return labelStr + " " + toggle
	}
	toggle := StyleDimmed.Render("○ Disabled")
	return labelStr + " " + toggle
}

func (v *ConnectView) renderButton(id int) string {
	label := fieldLabels[id]
	focused := v.focusField == id

	if focused {
		// Inverted style for active button
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Background(ColorAccent).
			Padding(0, 2).
			Render("⏎ " + label)
	}
	return lipgloss.NewStyle().
		Foreground(ColorDim).
		Padding(0, 2).
		Render("  " + label)
}
