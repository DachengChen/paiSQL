// view_connect.go — Connection setup screen.
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
	fieldConnect:    "[ Connect ]",
	fieldSave:       "[ Save ]",
	fieldDelete:     "[ Delete ]",
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

	// Load defaults
	def := config.DefaultConnection()
	v.fields[fieldHost] = def.Host
	v.fields[fieldPort] = def.Port
	v.fields[fieldUser] = def.User
	v.fields[fieldPassword] = def.Password
	v.fields[fieldDatabase] = def.Database
	v.fields[fieldSSLMode] = def.SSLMode
	v.fields[fieldSSHEnabled] = "no"
	v.fields[fieldSSHPort] = def.SSH.Port

	// If there are saved connections, select the first one
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
		}
	}
	return []KeyBinding{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "Enter", Desc: "edit/action"},
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
		// Handled by App
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
		// Skip SSH fields if SSH is disabled
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			v.focusField = fieldSSHEnabled - 1
		}
		// Skip saved if no saved connections
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField = fieldCount - 1
		}

	case "down", "j":
		v.focusField++
		// Skip SSH fields if SSH is disabled
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			v.focusField = fieldConnect
		}
		if v.focusField >= fieldCount {
			v.focusField = 0
		}
		// Skip saved if no saved connections
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField = fieldName
		}

	case "enter":
		return v.handleAction()

	case "tab":
		// Quick jump to Connect button
		v.focusField = fieldConnect
		return v, nil

	case "left", "h":
		if v.focusField == fieldSaved && len(v.store.Connections) > 0 {
			v.savedIdx--
			if v.savedIdx < 0 {
				v.savedIdx = len(v.store.Connections) - 1
			}
			v.loadSavedConnection(v.savedIdx)
		}
		if v.focusField == fieldSSLMode {
			v.cycleSSLMode(-1)
		}

	case "right", "l":
		if v.focusField == fieldSaved && len(v.store.Connections) > 0 {
			v.savedIdx++
			if v.savedIdx >= len(v.store.Connections) {
				v.savedIdx = 0
			}
			v.loadSavedConnection(v.savedIdx)
		}
		if v.focusField == fieldSSLMode {
			v.cycleSSLMode(1)
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
		// Already handled by left/right
		return v, nil

	case fieldSSHEnabled:
		// Toggle SSH
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
		// Enter editing mode for text fields
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

	// Update saved index
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

// View renders the connection form.
func (v *ConnectView) View() string {
	// Layout styles
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		MarginBottom(1)

	sectionStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true)

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("  🐘 paiSQL — Connect to PostgreSQL"))
	lines = append(lines, "")

	// Saved connections
	if len(v.store.Connections) > 0 {
		lines = append(lines, sectionStyle.Render("  Saved Connections"))
		savedLine := "  "
		for i, c := range v.store.Connections {
			label := c.Name
			if i == v.savedIdx {
				if v.focusField == fieldSaved {
					savedLine += StyleTabActive.Render(" ► " + label + " ")
				} else {
					savedLine += lipgloss.NewStyle().Foreground(ColorTabActive).Render(" ► " + label + " ")
				}
			} else {
				savedLine += StyleDimmed.Render("   " + label + " ")
			}
		}
		lines = append(lines, savedLine)
		lines = append(lines, "")
	}

	// Connection settings
	lines = append(lines, sectionStyle.Render("  Connection"))
	lines = append(lines, v.renderField(fieldName, 14))
	lines = append(lines, v.renderField(fieldHost, 14))
	lines = append(lines, v.renderField(fieldPort, 14))
	lines = append(lines, v.renderField(fieldUser, 14))
	lines = append(lines, v.renderPasswordField(14))
	lines = append(lines, v.renderField(fieldDatabase, 14))
	lines = append(lines, v.renderSelectField(fieldSSLMode, 14))
	lines = append(lines, "")

	// SSH tunnel section
	lines = append(lines, sectionStyle.Render("  SSH Tunnel"))
	lines = append(lines, v.renderToggleField(fieldSSHEnabled, 14))

	if v.sshEnabled() {
		lines = append(lines, v.renderField(fieldSSHHost, 14))
		lines = append(lines, v.renderField(fieldSSHPort, 14))
		lines = append(lines, v.renderField(fieldSSHUser, 14))
		lines = append(lines, v.renderField(fieldSSHKey, 14))
	}

	lines = append(lines, "")

	// Action buttons
	btnLine := "  "
	btnLine += v.renderButton(fieldConnect)
	btnLine += "  "
	btnLine += v.renderButton(fieldSave)
	btnLine += "  "
	btnLine += v.renderButton(fieldDelete)
	lines = append(lines, btnLine)

	lines = append(lines, "")

	// Status / error
	if v.connecting {
		lines = append(lines, "  "+StyleDimmed.Render("⏳ "+v.statusMsg))
	} else if v.err != nil {
		lines = append(lines, "  "+StyleError.Render("✗ "+v.err.Error()))
	} else if v.statusMsg != "" {
		lines = append(lines, "  "+StyleSuccess.Render("✓ "+v.statusMsg))
	}

	return strings.Join(lines, "\n")
}

func (v *ConnectView) renderField(id, labelWidth int) string {
	label := fieldLabels[id]
	value := v.fields[id]
	focused := v.focusField == id

	labelStr := fmt.Sprintf("  %-*s ", labelWidth, label+":")

	if focused && v.editing {
		return StylePrompt.Render(labelStr) +
			lipgloss.NewStyle().Foreground(ColorFg).Background(ColorBgAlt).Render(" "+value+"█ ")
	} else if focused {
		return StylePrompt.Render(labelStr) +
			lipgloss.NewStyle().Foreground(ColorFg).Background(ColorBgAlt).Render(" "+value+" ") +
			StyleDimmed.Render(" ← Enter to edit")
	}
	return StyleDimmed.Render(labelStr) + value
}

func (v *ConnectView) renderPasswordField(labelWidth int) string {
	label := "Password:"
	value := v.fields[fieldPassword]
	focused := v.focusField == fieldPassword

	masked := strings.Repeat("•", len(value))
	labelStr := fmt.Sprintf("  %-*s ", labelWidth, label)

	if focused && v.editing {
		return StylePrompt.Render(labelStr) +
			lipgloss.NewStyle().Foreground(ColorFg).Background(ColorBgAlt).Render(" "+masked+"█ ")
	} else if focused {
		return StylePrompt.Render(labelStr) +
			lipgloss.NewStyle().Foreground(ColorFg).Background(ColorBgAlt).Render(" "+masked+" ") +
			StyleDimmed.Render(" ← Enter to edit")
	}
	return StyleDimmed.Render(labelStr) + masked
}

func (v *ConnectView) renderSelectField(id, labelWidth int) string {
	label := fieldLabels[id]
	value := v.fields[id]
	focused := v.focusField == id

	labelStr := fmt.Sprintf("  %-*s ", labelWidth, label+":")

	if focused {
		return StylePrompt.Render(labelStr) +
			lipgloss.NewStyle().Foreground(ColorAccent).Background(ColorBgAlt).Render(" ◄ "+value+" ► ") +
			StyleDimmed.Render(" ← / → to change")
	}
	return StyleDimmed.Render(labelStr) + value
}

func (v *ConnectView) renderToggleField(id, labelWidth int) string {
	label := fieldLabels[id]
	enabled := v.fields[id] == "yes"
	focused := v.focusField == id

	labelStr := fmt.Sprintf("  %-*s ", labelWidth, label+":")

	indicator := "○ No"
	style := StyleDimmed
	if enabled {
		indicator = "● Yes"
		style = lipgloss.NewStyle().Foreground(ColorSuccess)
	}

	if focused {
		return StylePrompt.Render(labelStr) + style.Bold(true).Render(indicator) +
			StyleDimmed.Render(" ← Enter to toggle")
	}
	return StyleDimmed.Render(labelStr) + style.Render(indicator)
}

func (v *ConnectView) renderButton(id int) string {
	label := fieldLabels[id]
	focused := v.focusField == id

	if focused {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBg).
			Background(ColorPrimary).
			Padding(0, 1).
			Render(label)
	}
	return lipgloss.NewStyle().
		Foreground(ColorFgDim).
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Render(label)
}
