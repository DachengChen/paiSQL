// view_connect.go — Connection setup screen with integrated AI settings.
//
// This is the first screen shown when paiSQL starts. It has two blocks:
//
//	Block 0: Connection — host, port, user, password, SSH, etc.
//	Block 1: AI Settings — provider, API key, model
//
// Press TAB to switch between blocks. Arrow keys navigate within
// the active block. AI config is saved to ~/.paisql/config.json
// when connecting.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DachengChen/paiSQL/config"
	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── Connection block fields ────────────────────────────────
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
	// ─── AI block fields ────────────────────────────────────
	fieldAIProvider
	fieldAIAPIKey
	fieldAIModel
	fieldAIHost // only for Ollama
	fieldAISave
	fieldCount // sentinel
)

// Block boundaries
const (
	blockConn      = 0
	blockAI        = 1
	connFieldFirst = fieldSaved
	connFieldLast  = fieldDelete
	aiFieldFirst   = fieldAIProvider
	aiFieldLast    = fieldAISave
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
	fieldAIProvider: "Provider",
	fieldAIAPIKey:   "API Key",
	fieldAIModel:    "Model",
	fieldAIHost:     "Host",
	fieldAISave:     "Save AI",
}

// SSL mode options for cycling.
var sslModes = []string{"disable", "require", "verify-ca", "verify-full", "prefer"}

// AI provider options for cycling.
var aiProviders = []string{"openai", "anthropic", "gemini", "ollama", "placeholder"}

// aiProviderDesc maps provider name to a short label.
var aiProviderDesc = map[string]string{
	"openai":      "OpenAI (GPT-4o)",
	"anthropic":   "Anthropic (Claude)",
	"gemini":      "Google Gemini",
	"ollama":      "Ollama (local)",
	"placeholder": "None (disabled)",
}

// aiDefaultModels maps provider name to its default model.
var aiDefaultModels = map[string]string{
	"openai":    "gpt-4o",
	"anthropic": "claude-sonnet-4-20250514",
	"gemini":    "gemini-2.0-flash",
	"ollama":    "llama3.2",
}

// ConnectView is the connection + AI setup form.
type ConnectView struct {
	store      *config.ConnectionStore
	appCfg     *config.AppConfig
	fields     []string // field values indexed by field ID
	focusField int
	savedIdx   int  // selected index in saved connections list
	editing    bool // true when typing in a field
	err        error
	statusMsg  string
	connecting bool
	width      int
	height     int
	block      int      // 0=connection, 1=AI
	sshKeys    []string // discovered SSH key paths
	sshKeyIdx  int      // selected index in sshKeys
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

func NewConnectView(store *config.ConnectionStore, appCfg *config.AppConfig) *ConnectView {
	v := &ConnectView{
		store:      store,
		appCfg:     appCfg,
		fields:     make([]string, fieldCount),
		focusField: fieldHost,
		block:      blockConn,
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

	// Load AI settings from config
	v.fields[fieldAIProvider] = appCfg.AI.Provider
	if v.fields[fieldAIProvider] == "" {
		v.fields[fieldAIProvider] = "placeholder"
	}
	v.loadAIFieldsFromConfig()

	// Discover SSH keys
	v.sshKeys = discoverSSHKeys()
	if len(v.sshKeys) > 0 && v.fields[fieldSSHKey] == "" {
		v.fields[fieldSSHKey] = v.sshKeys[0]
	}
	// Set index to match current value
	for i, k := range v.sshKeys {
		if k == v.fields[fieldSSHKey] {
			v.sshKeyIdx = i
			break
		}
	}

	return v
}

// loadAIFieldsFromConfig populates AI fields from the current provider config.
func (v *ConnectView) loadAIFieldsFromConfig() {
	switch v.fields[fieldAIProvider] {
	case "openai":
		v.fields[fieldAIAPIKey] = v.appCfg.AI.OpenAI.APIKey
		v.fields[fieldAIModel] = v.appCfg.AI.OpenAI.Model
	case "anthropic":
		v.fields[fieldAIAPIKey] = v.appCfg.AI.Anthropic.APIKey
		v.fields[fieldAIModel] = v.appCfg.AI.Anthropic.Model
	case "gemini":
		v.fields[fieldAIAPIKey] = v.appCfg.AI.Gemini.APIKey
		v.fields[fieldAIModel] = v.appCfg.AI.Gemini.Model
	case "ollama":
		v.fields[fieldAIAPIKey] = ""
		v.fields[fieldAIModel] = v.appCfg.AI.Ollama.Model
		v.fields[fieldAIHost] = v.appCfg.AI.Ollama.Host
	default:
		v.fields[fieldAIAPIKey] = ""
		v.fields[fieldAIModel] = ""
		v.fields[fieldAIHost] = ""
	}
	if v.fields[fieldAIModel] == "" {
		v.fields[fieldAIModel] = aiDefaultModels[v.fields[fieldAIProvider]]
	}
	if v.fields[fieldAIHost] == "" {
		v.fields[fieldAIHost] = "http://localhost:11434"
	}
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
	blockLabel := "AI"
	if v.block == blockAI {
		blockLabel = "Connection"
	}
	return []KeyBinding{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "Tab", Desc: blockLabel},
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
		return v, nil

	case ConnectErrorMsg:
		v.connecting = false
		v.err = msg.Err
		v.statusMsg = ""
		return v, nil
	}

	return v, nil
}

// ─────────────────────────────────────────────────────────────
// Navigation — TAB switches blocks, arrows within block
// ─────────────────────────────────────────────────────────────

func (v *ConnectView) handleNavigation(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "tab":
		v.switchBlock()

	case "up", "k":
		v.moveWithinBlock(-1)

	case "down", "j":
		v.moveWithinBlock(1)

	case "enter":
		return v.handleAction()

	case "left", "h":
		return v.handleLeft()

	case "right", "l":
		return v.handleRight()

	case "q", "ctrl+c":
		return v, tea.Quit
	}

	return v, nil
}

func (v *ConnectView) switchBlock() {
	if v.block == blockConn {
		v.block = blockAI
		v.focusField = fieldAIProvider
	} else {
		v.block = blockConn
		if len(v.store.Connections) > 0 {
			v.focusField = fieldSaved
		} else {
			v.focusField = fieldHost
		}
	}
}

func (v *ConnectView) moveWithinBlock(dir int) {
	first, last := v.blockRange()
	v.focusField += dir

	// Wrap within block
	if v.focusField < first {
		v.focusField = last
	}
	if v.focusField > last {
		v.focusField = first
	}

	// Skip hidden fields
	v.skipHiddenFields(dir)
}

func (v *ConnectView) blockRange() (int, int) {
	if v.block == blockAI {
		return aiFieldFirst, aiFieldLast
	}
	return connFieldFirst, connFieldLast
}

func (v *ConnectView) skipHiddenFields(dir int) {
	first, last := v.blockRange()

	// Connection block: skip saved if empty, skip SSH fields if disabled
	if v.block == blockConn {
		if v.focusField == fieldSaved && len(v.store.Connections) == 0 {
			v.focusField += dir
		}
		if !v.sshEnabled() && v.isSSHField(v.focusField) {
			if dir > 0 {
				v.focusField = fieldConnect
			} else {
				v.focusField = fieldSSHEnabled - 1
			}
		}
	}

	// AI block: skip API key for ollama/placeholder, skip host for non-ollama
	if v.block == blockAI {
		provider := v.fields[fieldAIProvider]
		if (provider == "ollama" || provider == "placeholder") && v.focusField == fieldAIAPIKey {
			v.focusField += dir
		}
		if provider != "ollama" && v.focusField == fieldAIHost {
			v.focusField += dir
		}
		if provider == "placeholder" && (v.focusField == fieldAIModel || v.focusField == fieldAIHost) {
			v.focusField += dir
		}
	}

	// Final bounds check
	if v.focusField < first {
		v.focusField = last
	}
	if v.focusField > last {
		v.focusField = first
	}
}

func (v *ConnectView) handleLeft() (View, tea.Cmd) {
	switch v.focusField {
	case fieldSaved:
		if len(v.store.Connections) > 0 {
			v.savedIdx--
			if v.savedIdx < 0 {
				v.savedIdx = len(v.store.Connections) - 1
			}
			v.loadSavedConnection(v.savedIdx)
		}
	case fieldSSLMode:
		v.cycleSSLMode(-1)
	case fieldSSHKey:
		v.cycleSSHKey(-1)
	case fieldAIProvider:
		v.cycleAIProvider(-1)
	default:
		v.moveWithinBlock(-1)
	}
	return v, nil
}

func (v *ConnectView) handleRight() (View, tea.Cmd) {
	switch v.focusField {
	case fieldSaved:
		if len(v.store.Connections) > 0 {
			v.savedIdx++
			if v.savedIdx >= len(v.store.Connections) {
				v.savedIdx = 0
			}
			v.loadSavedConnection(v.savedIdx)
		}
	case fieldSSLMode:
		v.cycleSSLMode(1)
	case fieldSSHKey:
		v.cycleSSHKey(1)
	case fieldAIProvider:
		v.cycleAIProvider(1)
	default:
		v.moveWithinBlock(1)
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

	case fieldAIProvider:
		v.cycleAIProvider(1)
		return v, nil

	case fieldSSHKey:
		// If we have discovered keys, cycle; otherwise allow manual edit
		if len(v.sshKeys) > 0 {
			v.cycleSSHKey(1)
		} else {
			v.editing = true
		}
		return v, nil

	case fieldAISave:
		return v, v.saveAIConfig()

	default:
		// Editable text fields
		v.editing = true
		return v, nil
	}
}

// ─────────────────────────────────────────────────────────────
// Connection logic
// ─────────────────────────────────────────────────────────────

func (v *ConnectView) connect() tea.Cmd {
	conn := v.buildConnection()
	cfg := config.FromConnection(conn)

	v.connecting = true
	v.statusMsg = "Connecting..."
	v.err = nil

	// Save AI config before connecting
	v.applyAIConfig()
	_ = config.SaveAppConfig(v.appCfg)

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

	// Sync SSH key index
	for i, k := range v.sshKeys {
		if k == c.SSH.KeyPath {
			v.sshKeyIdx = i
			break
		}
	}
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

// cycleSSHKey cycles through discovered SSH key files.
func (v *ConnectView) cycleSSHKey(dir int) {
	if len(v.sshKeys) == 0 {
		return
	}
	v.sshKeyIdx = (v.sshKeyIdx + dir + len(v.sshKeys)) % len(v.sshKeys)
	v.fields[fieldSSHKey] = v.sshKeys[v.sshKeyIdx]
}

// discoverSSHKeys scans ~/.ssh/ for private key files.
func discoverSSHKeys() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}

	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip public keys, known_hosts, config, etc.
		if strings.HasSuffix(name, ".pub") ||
			name == "known_hosts" ||
			name == "known_hosts.old" ||
			name == "config" ||
			name == "authorized_keys" ||
			name == "environment" {
			continue
		}

		fullPath := filepath.Join(sshDir, name)

		// Quick check: read first bytes to see if it looks like a key
		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}
		buf := make([]byte, 40)
		n, _ := f.Read(buf)
		f.Close()

		header := string(buf[:n])
		if strings.Contains(header, "PRIVATE KEY") ||
			strings.Contains(header, "OPENSSH") {
			keys = append(keys, fullPath)
		}
	}

	return keys
}

// ─────────────────────────────────────────────────────────────
// AI config logic
// ─────────────────────────────────────────────────────────────

func (v *ConnectView) cycleAIProvider(dir int) {
	current := v.fields[fieldAIProvider]
	idx := 0
	for i, p := range aiProviders {
		if p == current {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(aiProviders)) % len(aiProviders)
	v.fields[fieldAIProvider] = aiProviders[idx]

	// Reset model to default for new provider
	v.fields[fieldAIModel] = aiDefaultModels[v.fields[fieldAIProvider]]
	v.fields[fieldAIAPIKey] = ""
	v.fields[fieldAIHost] = "http://localhost:11434"

	// Load existing key if we have one
	v.loadAIKeyFromConfig()

	v.err = nil
}

// loadAIKeyFromConfig loads the saved API key for the current provider.
func (v *ConnectView) loadAIKeyFromConfig() {
	switch v.fields[fieldAIProvider] {
	case "openai":
		v.fields[fieldAIAPIKey] = v.appCfg.AI.OpenAI.APIKey
	case "anthropic":
		v.fields[fieldAIAPIKey] = v.appCfg.AI.Anthropic.APIKey
	case "gemini":
		v.fields[fieldAIAPIKey] = v.appCfg.AI.Gemini.APIKey
	}
}

// applyAIConfig writes the current AI form values back to appConfig.
func (v *ConnectView) applyAIConfig() {
	v.appCfg.AI.Provider = v.fields[fieldAIProvider]
	switch v.fields[fieldAIProvider] {
	case "openai":
		v.appCfg.AI.OpenAI.APIKey = v.fields[fieldAIAPIKey]
		v.appCfg.AI.OpenAI.Model = v.fields[fieldAIModel]
	case "anthropic":
		v.appCfg.AI.Anthropic.APIKey = v.fields[fieldAIAPIKey]
		v.appCfg.AI.Anthropic.Model = v.fields[fieldAIModel]
	case "gemini":
		v.appCfg.AI.Gemini.APIKey = v.fields[fieldAIAPIKey]
		v.appCfg.AI.Gemini.Model = v.fields[fieldAIModel]
	case "ollama":
		v.appCfg.AI.Ollama.Host = v.fields[fieldAIHost]
		v.appCfg.AI.Ollama.Model = v.fields[fieldAIModel]
	}
}

func (v *ConnectView) saveAIConfig() tea.Cmd {
	v.applyAIConfig()
	if err := config.SaveAppConfig(v.appCfg); err != nil {
		v.err = err
		return nil
	}
	v.statusMsg = "AI config saved!"
	v.err = nil
	return nil
}

// ─────────────────────────────────────────────────────────────
// View rendering — two-block layout
// ─────────────────────────────────────────────────────────────

func (v *ConnectView) View() string {
	// Calculate panel widths: left panel gets 60%, right panel 40%
	totalWidth := v.width
	if totalWidth < 40 {
		totalWidth = 40
	}
	leftWidth := (totalWidth * 6) / 10
	rightWidth := totalWidth - leftWidth
	if leftWidth < 30 {
		leftWidth = 30
	}
	if rightWidth < 25 {
		rightWidth = 25
	}

	leftInputW := leftWidth - 24 // label + padding + border
	rightInputW := rightWidth - 24

	if leftInputW < 10 {
		leftInputW = 10
	}
	if rightInputW < 10 {
		rightInputW = 10
	}

	// ══════════════════════════════════════════
	// Left panel: Connection
	// ══════════════════════════════════════════

	var leftLines []string

	// Saved connections
	if len(v.store.Connections) > 0 {
		leftLines = append(leftLines, v.blockHeader("Saved", leftWidth-8, blockConn))
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
		leftLines = append(leftLines, savedLine)
		leftLines = append(leftLines, "")
	}

	// Connection fields
	leftLines = append(leftLines, v.blockHeader("Connection", leftWidth-8, blockConn))
	leftLines = append(leftLines, v.renderField(fieldName, leftInputW))
	leftLines = append(leftLines, v.renderField(fieldHost, leftInputW))
	leftLines = append(leftLines, v.renderField(fieldPort, leftInputW))
	leftLines = append(leftLines, v.renderField(fieldUser, leftInputW))
	leftLines = append(leftLines, v.renderPasswordField(leftInputW))
	leftLines = append(leftLines, v.renderField(fieldDatabase, leftInputW))
	leftLines = append(leftLines, v.renderSelectField(fieldSSLMode, leftInputW))
	leftLines = append(leftLines, "")

	// SSH Tunnel
	leftLines = append(leftLines, v.blockHeader("SSH Tunnel", leftWidth-8, blockConn))
	leftLines = append(leftLines, v.renderToggleField(fieldSSHEnabled))

	if v.sshEnabled() {
		leftLines = append(leftLines, v.renderField(fieldSSHHost, leftInputW))
		leftLines = append(leftLines, v.renderField(fieldSSHPort, leftInputW))
		leftLines = append(leftLines, v.renderField(fieldSSHUser, leftInputW))
		leftLines = append(leftLines, v.renderSSHKeyField(leftInputW))
	}

	leftLines = append(leftLines, "")

	// Connection action buttons
	btnLine := v.renderButton(fieldConnect) + "  " +
		v.renderButton(fieldSave) + "  " +
		v.renderButton(fieldDelete)
	leftLines = append(leftLines, btnLine)

	leftContent := strings.Join(leftLines, "\n")

	// Left panel border style — highlight if active
	leftBorder := StyleBorder.Padding(1, 2).Width(leftWidth - 2)
	if v.block == blockConn {
		leftBorder = leftBorder.BorderForeground(ColorAccent)
	}
	leftPanel := leftBorder.Render(leftContent)

	// ══════════════════════════════════════════
	// Right panel: AI Settings
	// ══════════════════════════════════════════

	var rightLines []string

	rightLines = append(rightLines, v.blockHeader("🤖 AI Settings", rightWidth-8, blockAI))
	rightLines = append(rightLines, v.renderSelectField(fieldAIProvider, rightInputW))

	// Show provider description
	provider := v.fields[fieldAIProvider]
	if desc, ok := aiProviderDesc[provider]; ok {
		rightLines = append(rightLines, StyleDimmed.Render("  "+desc))
	}

	rightLines = append(rightLines, "")

	if provider != "placeholder" {
		if provider != "ollama" {
			rightLines = append(rightLines, v.renderMaskedField(fieldAIAPIKey, rightInputW))
		}
		rightLines = append(rightLines, v.renderField(fieldAIModel, rightInputW))
		if provider == "ollama" {
			rightLines = append(rightLines, v.renderField(fieldAIHost, rightInputW))
		}
	}

	rightLines = append(rightLines, "")
	rightLines = append(rightLines, v.renderButton(fieldAISave))

	rightContent := strings.Join(rightLines, "\n")

	// Right panel border style — highlight if active
	rightBorder := StyleBorder.Padding(1, 2).Width(rightWidth - 2)
	if v.block == blockAI {
		rightBorder = rightBorder.BorderForeground(ColorAccent)
	}
	rightPanel := rightBorder.Render(rightContent)

	// ══════════════════════════════════════════
	// Combine panels side by side
	// ══════════════════════════════════════════

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Status line below panels
	var statusLine string
	if v.connecting {
		statusLine = StyleDimmed.Render("⏳ " + v.statusMsg)
	} else if v.err != nil {
		statusLine = StyleError.Render("✗ " + v.err.Error())
	} else if v.statusMsg != "" {
		statusLine = StyleSuccess.Render("✓ " + v.statusMsg)
	}

	var content string
	if statusLine != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, panels, statusLine)
	} else {
		content = panels
	}

	// Center everything
	centered := lipgloss.NewStyle().
		Width(v.width).
		Height(v.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(content)

	return centered
}

// blockHeader renders a section header, highlighted if the block is active.
func (v *ConnectView) blockHeader(label string, width int, blk int) string {
	active := v.block == blk

	var labelRendered string
	if active {
		labelRendered = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true).
			Render(label)
	} else {
		labelRendered = lipgloss.NewStyle().
			Foreground(ColorDim).
			Render(label)
	}

	// Calculate dashes on each side
	labelLen := len(label)
	remaining := width - labelLen - 4
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

func (v *ConnectView) renderMaskedField(id, inputWidth int) string {
	label := fieldLabels[id]
	value := v.fields[id]
	focused := v.focusField == id
	masked := strings.Repeat("•", len(value))

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

func (v *ConnectView) renderSSHKeyField(inputWidth int) string {
	focused := v.focusField == fieldSSHKey

	labelStr := lipgloss.NewStyle().
		Width(16).
		Foreground(ColorDim).
		Render("SSH Key")

	if focused {
		labelStr = lipgloss.NewStyle().
			Width(16).
			Foreground(ColorAccent).
			Bold(true).
			Render("▸ SSH Key")
	}

	// If we have discovered keys, render as a selector
	if len(v.sshKeys) > 0 {
		keyPath := v.fields[fieldSSHKey]
		keyName := filepath.Base(keyPath)

		if focused {
			selectBox := lipgloss.NewStyle().
				Foreground(ColorAccent).
				Render(" ◂ " + keyName + " ▸ ")
			return labelStr + " " + selectBox
		}
		return labelStr + " " + StyleDimmed.Render(keyName)
	}

	// No keys discovered — fall back to editable text field
	value := v.fields[fieldSSHKey]
	if focused {
		cursor := "█"
		if !v.editing {
			cursor = ""
		}
		inputBox := lipgloss.NewStyle().
			Width(inputWidth).
			Foreground(ColorPrimary).
			Render(value + cursor)
		return labelStr + " " + inputBox
	}
	return labelStr + " " + StyleDimmed.Render(value)
}
