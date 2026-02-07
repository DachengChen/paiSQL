// viewport.go provides a reusable scrollable viewport component
// with both vertical and horizontal scrolling, pagination, and text wrapping.
//
// This is used by all views that need to display scrollable content.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Viewport is a scrollable text area with pagination.
type Viewport struct {
	width    int
	height   int
	content  []string // lines of content
	scrollY  int      // vertical scroll offset (line index)
	scrollX  int      // horizontal scroll offset (column index)
	wrapText bool     // whether to wrap text instead of horizontal scroll
}

// NewViewport creates a viewport with the given dimensions.
func NewViewport(width, height int) *Viewport {
	return &Viewport{
		width:  width,
		height: height,
	}
}

// SetContent replaces the viewport content.
func (v *Viewport) SetContent(content string) {
	v.content = strings.Split(content, "\n")
	v.clampScroll()
}

// SetContentLines replaces the viewport content with pre-split lines.
func (v *Viewport) SetContentLines(lines []string) {
	v.content = lines
	v.clampScroll()
}

// SetSize updates viewport dimensions.
func (v *Viewport) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampScroll()
}

// ToggleWrap toggles text wrapping.
func (v *Viewport) ToggleWrap() {
	v.wrapText = !v.wrapText
	v.scrollX = 0
	v.clampScroll()
}

// ScrollUp moves the viewport up by n lines.
func (v *Viewport) ScrollUp(n int) {
	v.scrollY -= n
	v.clampScroll()
}

// ScrollDown moves the viewport down by n lines.
func (v *Viewport) ScrollDown(n int) {
	v.scrollY += n
	v.clampScroll()
}

// ScrollLeft moves the viewport left.
func (v *Viewport) ScrollLeft(n int) {
	if !v.wrapText {
		v.scrollX -= n
		if v.scrollX < 0 {
			v.scrollX = 0
		}
	}
}

// ScrollRight moves the viewport right.
func (v *Viewport) ScrollRight(n int) {
	if !v.wrapText {
		v.scrollX += n
	}
}

// PageUp scrolls up by one page.
func (v *Viewport) PageUp() {
	v.ScrollUp(v.height)
}

// PageDown scrolls down by one page.
func (v *Viewport) PageDown() {
	v.ScrollDown(v.height)
}

// Home scrolls to the top.
func (v *Viewport) Home() {
	v.scrollY = 0
	v.scrollX = 0
}

// End scrolls to the bottom.
func (v *Viewport) End() {
	v.scrollY = v.maxScrollY()
}

// Render returns the visible portion of the content.
func (v *Viewport) Render() string {
	if len(v.content) == 0 {
		return ""
	}

	var visibleLines []string

	if v.wrapText {
		visibleLines = v.renderWrapped()
	} else {
		visibleLines = v.renderScrolled()
	}

	// Pad to fill viewport height
	for len(visibleLines) < v.height {
		visibleLines = append(visibleLines, "")
	}

	// Add scroll indicator
	indicator := v.scrollIndicator()

	content := strings.Join(visibleLines, "\n")
	return lipgloss.JoinVertical(lipgloss.Left, content, indicator)
}

// renderScrolled returns lines with horizontal offset applied.
func (v *Viewport) renderScrolled() []string {
	end := v.scrollY + v.height
	if end > len(v.content) {
		end = len(v.content)
	}

	var lines []string
	for i := v.scrollY; i < end; i++ {
		line := v.content[i]
		// Apply horizontal scroll
		if v.scrollX > 0 && v.scrollX < len(line) {
			line = line[v.scrollX:]
		} else if v.scrollX >= len(line) {
			line = ""
		}
		// Truncate to width
		if len(line) > v.width {
			line = line[:v.width]
		}
		lines = append(lines, line)
	}
	return lines
}

// renderWrapped returns word-wrapped lines.
func (v *Viewport) renderWrapped() []string {
	// First, wrap all content lines
	var wrapped []string
	for _, line := range v.content {
		if len(line) <= v.width || v.width <= 0 {
			wrapped = append(wrapped, line)
		} else {
			for len(line) > v.width {
				wrapped = append(wrapped, line[:v.width])
				line = line[v.width:]
			}
			wrapped = append(wrapped, line)
		}
	}

	// Apply vertical scroll
	end := v.scrollY + v.height
	if v.scrollY >= len(wrapped) {
		return nil
	}
	if end > len(wrapped) {
		end = len(wrapped)
	}
	return wrapped[v.scrollY:end]
}

func (v *Viewport) clampScroll() {
	maxY := v.maxScrollY()
	if v.scrollY > maxY {
		v.scrollY = maxY
	}
	if v.scrollY < 0 {
		v.scrollY = 0
	}
}

func (v *Viewport) maxScrollY() int {
	total := len(v.content)
	if v.wrapText && v.width > 0 {
		total = 0
		for _, line := range v.content {
			if len(line) <= v.width {
				total++
			} else {
				total += (len(line) + v.width - 1) / v.width
			}
		}
	}
	max := total - v.height
	if max < 0 {
		return 0
	}
	return max
}

func (v *Viewport) scrollIndicator() string {
	if len(v.content) <= v.height {
		return ""
	}

	total := len(v.content)
	pos := v.scrollY
	pct := 0
	if total > 0 {
		pct = (pos * 100) / total
	}

	indicator := StyleDimmed.Render(
		strings.Repeat("─", v.width-20) +
			" " + itoa2(pct) + "% " +
			"(" + itoa2(pos+1) + "/" + itoa2(total) + ")")

	return indicator
}

func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa2(-n)
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
