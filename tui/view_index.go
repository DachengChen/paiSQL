// view_index.go — Index suggestions view.
//
// Combines EXPLAIN output with AI analysis to suggest indexes.
// The user enters a query, we run EXPLAIN, then ask the AI provider
// for optimization suggestions.
package tui

import (
	"context"
	"strings"

	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type IndexView struct {
	db         *db.DB
	aiProvider ai.Provider
	viewport   *Viewport
	input      string
	loading    bool
	err        error
	width      int
	height     int
}

func NewIndexView(database *db.DB, provider ai.Provider) *IndexView {
	return &IndexView{
		db:         database,
		aiProvider: provider,
		viewport:   NewViewport(80, 20),
	}
}

func (v *IndexView) Name() string         { return "Index" }
func (v *IndexView) WantsTextInput() bool { return false }

func (v *IndexView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.SetSize(width-2, height-4)
}

func (v *IndexView) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "Enter", Desc: "analyze"},
		{Key: "↑/↓", Desc: "scroll"},
	}
}

func (v *IndexView) Init() tea.Cmd {
	v.viewport.SetContent(StyleDimmed.Render(
		"Enter a SQL query and press Enter to get index suggestions.\n\n" +
			"The query will be analyzed with EXPLAIN and then sent to the\n" +
			"AI provider for optimization recommendations."))
	return nil
}

func (v *IndexView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case IndexSuggestionMsg:
		v.loading = false
		v.err = msg.Err
		if msg.Err != nil {
			v.viewport.SetContent(StyleError.Render("ERROR: " + msg.Err.Error()))
		} else {
			v.viewport.SetContent(msg.Suggestion)
		}
		return v, nil
	}
	return v, nil
}

func (v *IndexView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return v, v.analyze()
	case "ctrl+k":
		v.viewport.ScrollUp(1)
	case "ctrl+j":
		v.viewport.ScrollDown(1)
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
		if msg.Type == tea.KeyRunes {
			v.input += string(msg.Runes)
		} else if msg.Type == tea.KeySpace {
			v.input += " "
		}
	}
	return v, nil
}

func (v *IndexView) analyze() tea.Cmd {
	sql := strings.TrimSpace(v.input)
	if sql == "" {
		return nil
	}

	v.loading = true

	providerName := v.aiProvider.Name()
	return func() tea.Msg {
		ctx := context.Background()

		// First, get the explain plan
		explain, err := v.db.Explain(ctx, sql, false)
		if err != nil {
			return IndexSuggestionMsg{Err: err}
		}

		// Log and ask AI for suggestions
		ai.LogAIRequest("SuggestIndexes", providerName, map[string]string{
			"SQL":     sql,
			"Explain": explain.JSON,
		})
		suggestion, err := v.aiProvider.SuggestIndexes(ctx, sql, explain.JSON)
		ai.LogAIResponse("SuggestIndexes", suggestion, err)
		return IndexSuggestionMsg{Suggestion: suggestion, Err: err}
	}
}

func (v *IndexView) View() string {
	prompt := StylePrompt.Render("Index> ") + v.input + "█"
	if v.loading {
		prompt = StylePrompt.Render("Index> ") + StyleDimmed.Render("analyzing query plan...")
	}

	content := v.viewport.Render()

	return lipgloss.JoinVertical(lipgloss.Left, prompt, "", content)
}
