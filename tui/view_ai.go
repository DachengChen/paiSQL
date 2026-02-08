// view_ai.go — AI assistant view.
//
// Provides a chat interface to the configured AI provider.
// Messages are sent asynchronously; the UI remains responsive
// while waiting for AI responses.
package tui

import (
	"context"
	"strings"

	"github.com/DachengChen/paiSQL/ai"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AIView struct {
	provider ai.Provider
	viewport *Viewport
	input    string
	messages []ai.Message
	loading  bool
	err      error
	width    int
	height   int
}

func NewAIView(provider ai.Provider) *AIView {
	return &AIView{
		provider: provider,
		viewport: NewViewport(80, 20),
	}
}

func (v *AIView) Name() string { return "AI" }

func (v *AIView) WantsTextInput() bool { return true }

func (v *AIView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.SetSize(width-2, height-4)
}

func (v *AIView) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "Enter", Desc: "send"},
		{Key: "Ctrl+L", Desc: "clear"},
		{Key: "↑/↓", Desc: "scroll"},
	}
}

func (v *AIView) Init() tea.Cmd {
	welcome := []string{
		StyleTitle.Render("🤖 AI Assistant") + StyleDimmed.Render(" ("+v.provider.Name()+")"),
		"",
		"Ask me anything about your database:",
		"  • Query optimization tips",
		"  • Index recommendations",
		"  • Schema design advice",
		"  • SQL syntax help",
		"",
		StyleDimmed.Render("Type your question and press Enter."),
	}
	v.viewport.SetContentLines(welcome)
	return nil
}

func (v *AIView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case AIResponseMsg:
		v.loading = false
		if msg.Err != nil {
			v.err = msg.Err
			v.messages = append(v.messages, ai.Message{
				Role:    "assistant",
				Content: "Error: " + msg.Err.Error(),
			})
		} else {
			v.messages = append(v.messages, ai.Message{
				Role:    "assistant",
				Content: msg.Response,
			})
		}
		v.viewport.SetContentLines(v.renderChat())
		v.viewport.End()
		return v, nil
	}

	return v, nil
}

func (v *AIView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return v, v.sendMessage()
	case "ctrl+l":
		v.messages = nil
		v.viewport.SetContent("")
		return v, v.Init()
	case "ctrl+k":
		v.viewport.ScrollUp(1)
	case "ctrl+j":
		v.viewport.ScrollDown(1)
	case "pgup":
		v.viewport.PageUp()
	case "pgdown":
		v.viewport.PageDown()
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

func (v *AIView) sendMessage() tea.Cmd {
	text := strings.TrimSpace(v.input)
	if text == "" {
		return nil
	}

	v.messages = append(v.messages, ai.Message{
		Role:    "user",
		Content: text,
	})
	v.input = ""
	v.loading = true
	v.viewport.SetContentLines(v.renderChat())
	v.viewport.End()

	// Copy messages for the goroutine
	msgs := make([]ai.Message, len(v.messages))
	copy(msgs, v.messages)

	providerName := v.provider.Name()
	return func() tea.Msg {
		// Build input summary for logging
		var inputSummary string
		for _, m := range msgs {
			inputSummary += m.Role + ": " + m.Content + "\n"
		}
		ai.LogAIRequest("Chat", providerName, map[string]string{
			"Messages": inputSummary,
		})

		resp, err := v.provider.Chat(context.Background(), msgs)
		ai.LogAIResponse("Chat", resp, err)
		return AIResponseMsg{Response: resp, Err: err}
	}
}

func (v *AIView) renderChat() []string {
	var lines []string

	lines = append(lines, StyleTitle.Render("🤖 AI Assistant")+" "+
		StyleDimmed.Render("("+v.provider.Name()+")"))
	lines = append(lines, "")

	userStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true)

	assistantStyle := lipgloss.NewStyle().
		Foreground(ColorSuccess)

	for _, msg := range v.messages {
		switch msg.Role {
		case "user":
			lines = append(lines, userStyle.Render("You: ")+msg.Content)
			lines = append(lines, "")
		case "assistant":
			lines = append(lines, assistantStyle.Render("AI: "))
			for _, line := range strings.Split(msg.Content, "\n") {
				lines = append(lines, "  "+line)
			}
			lines = append(lines, "")
		}
	}

	if v.loading {
		lines = append(lines, StyleDimmed.Render("  ⏳ Thinking..."))
	}

	return lines
}

func (v *AIView) View() string {
	prompt := StylePrompt.Render("Ask> ") + v.input + "█"
	if v.loading {
		prompt = StylePrompt.Render("Ask> ") + StyleDimmed.Render("waiting for response...")
	}

	content := v.viewport.Render()

	return lipgloss.JoinVertical(lipgloss.Left, prompt, "", content)
}
