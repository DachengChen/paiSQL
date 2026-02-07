// view_explain.go — EXPLAIN / EXPLAIN ANALYZE view.
//
// Shows the JSON query plan with syntax highlighting and scrolling.
// The user can paste a query and run EXPLAIN or EXPLAIN ANALYZE.
package tui

import (
	"context"
	"strings"

	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ExplainView struct {
	db       *db.DB
	viewport *Viewport
	input    string
	analyze  bool
	loading  bool
	err      error
	width    int
	height   int
}

func NewExplainView(database *db.DB) *ExplainView {
	return &ExplainView{
		db:       database,
		viewport: NewViewport(80, 20),
	}
}

func (v *ExplainView) Name() string         { return "Explain" }
func (v *ExplainView) WantsTextInput() bool { return false }

func (v *ExplainView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.SetSize(width-2, height-4)
}

func (v *ExplainView) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "Enter", Desc: "explain"},
		{Key: "Ctrl+A", Desc: "analyze"},
		{Key: "w", Desc: "wrap"},
	}
}

func (v *ExplainView) Init() tea.Cmd { return nil }

func (v *ExplainView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case ExplainResultMsg:
		v.loading = false
		v.err = msg.Err
		if msg.Result != nil {
			v.viewport.SetContent(v.formatJSON(msg.Result.JSON))
		} else if msg.Err != nil {
			v.viewport.SetContent(StyleError.Render("ERROR: " + msg.Err.Error()))
		}
		return v, nil
	}

	return v, nil
}

func (v *ExplainView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return v, v.runExplain(false)

	case "ctrl+a":
		return v, v.runExplain(true)

	case "ctrl+k":
		v.viewport.ScrollUp(1)
	case "ctrl+j":
		v.viewport.ScrollDown(1)
	case "ctrl+h":
		v.viewport.ScrollLeft(4)
	case "ctrl+l":
		v.viewport.ScrollRight(4)
	case "pgup":
		v.viewport.PageUp()
	case "pgdown":
		v.viewport.PageDown()
	case "ctrl+w":
		v.viewport.ToggleWrap()
	case "backspace":
		if len(v.input) > 0 {
			v.input = v.input[:len(v.input)-1]
		}
	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			v.input += msg.String()
		}
	}
	return v, nil
}

func (v *ExplainView) runExplain(analyze bool) tea.Cmd {
	sql := strings.TrimSpace(v.input)
	if sql == "" {
		return nil
	}

	v.loading = true
	v.analyze = analyze

	return func() tea.Msg {
		result, err := v.db.Explain(context.Background(), sql, analyze)
		return ExplainResultMsg{Result: result, Err: err}
	}
}

// formatJSON adds basic colorization to JSON output.
func (v *ExplainView) formatJSON(json string) string {
	// Simple colorization: keys in cyan, numbers in amber, strings in green
	var lines []string
	for _, line := range strings.Split(json, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, ":") {
			// Rough key highlighting
			parts := strings.SplitN(trimmed, ":", 2)
			colored := lipgloss.NewStyle().Foreground(ColorSecondary).Render(parts[0]) + ":" + parts[1]
			// Preserve indentation
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines = append(lines, indent+colored)
		} else {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func (v *ExplainView) View() string {
	mode := "EXPLAIN"
	if v.analyze {
		mode = "EXPLAIN ANALYZE"
	}

	prompt := StylePrompt.Render(mode+"> ") + v.input + "█"
	if v.loading {
		prompt = StylePrompt.Render(mode+"> ") + StyleDimmed.Render("analyzing...")
	}

	content := v.viewport.Render()

	return lipgloss.JoinVertical(lipgloss.Left, prompt, "", content)
}
