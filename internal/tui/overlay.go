package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Modal rendering ───────────────────────────────────────────────────────────

var (
	modalBorderStyle = lipgloss.NewStyle().Foreground(clrFocus)
	modalTitleStyle  = lipgloss.NewStyle().Foreground(clrFocus).Bold(true)
)

// renderModal renders content inside a rounded border with the title embedded
// in the top edge:  ╭───[ icon  title ]───╮
//
// panelW is the total content+padding width (padding is fixed at 1 row / 3 cols).
// The usable text area inside is panelW-6.  All background-coloured or
// width-constrained elements in content must be at most (panelW-6) wide;
// cursor-prefixed rows (cursor = 2 chars) must be at most (panelW-8) wide.
func renderModal(icon, title string, panelW int, content string) string {
	padded := lipgloss.NewStyle().Padding(1, 3).Width(panelW).Render(content)
	lines := strings.Split(padded, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	label := " " + icon + "  " + title + " "
	labelW := ansiWidth(label)
	avail := panelW - labelW
	if avail < 2 {
		avail = 2
	}
	leftD := avail / 2
	rightD := avail - leftD

	top := modalBorderStyle.Render("╭") +
		modalBorderStyle.Render(strings.Repeat("─", leftD)) +
		modalTitleStyle.Render(label) +
		modalBorderStyle.Render(strings.Repeat("─", rightD)) +
		modalBorderStyle.Render("╮")
	bot := modalBorderStyle.Render("╰") +
		modalBorderStyle.Render(strings.Repeat("─", panelW)) +
		modalBorderStyle.Render("╯")

	var sb strings.Builder
	sb.WriteString(top + "\n")
	for _, line := range lines {
		sb.WriteString(modalBorderStyle.Render("│") + line + modalBorderStyle.Render("│") + "\n")
	}
	sb.WriteString(bot)
	return sb.String()
}

// centerOverlay composites fg on top of bg, centering fg both horizontally
// and vertically. ANSI escape sequences in bg lines are preserved correctly.
func centerOverlay(bg, fg string) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	bgH := len(bgLines)
	fgH := len(fgLines)

	if bgH < fgH {
		bgW := 0
		for _, l := range bgLines {
			if w := ansiWidth(l); w > bgW {
				bgW = w
			}
		}
		return lipgloss.Place(bgW, bgH, lipgloss.Center, lipgloss.Center, fg)
	}

	fgW := 0
	for _, l := range fgLines {
		if w := ansiWidth(l); w > fgW {
			fgW = w
		}
	}

	bgW := 0
	for _, l := range bgLines {
		if w := ansiWidth(l); w > bgW {
			bgW = w
		}
	}

	startY := (bgH - fgH) / 2
	startX := (bgW - fgW) / 2
	if startX < 0 {
		startX = 0
	}

	result := make([]string, bgH)
	copy(result, bgLines)

	for i, fgLine := range fgLines {
		bgIdx := startY + i
		if bgIdx < 0 || bgIdx >= bgH {
			continue
		}
		result[bgIdx] = injectLine(result[bgIdx], fgLine, startX)
	}

	return strings.Join(result, "\n")
}

// injectLine places fgLine starting at visible column x within bgLine.
func injectLine(bgLine, fgLine string, x int) string {
	left := ansiPrefix(bgLine, x)
	leftVis := ansiWidth(left)
	if leftVis < x {
		left += strings.Repeat(" ", x-leftVis)
	}
	right := ansiSuffix(bgLine, x+ansiWidth(fgLine))
	return left + fgLine + "\033[0m" + right
}

// ansiWidth returns the visible (printable) width of s, ignoring ANSI escapes.
func ansiWidth(s string) int {
	w := 0
	i := 0
	runes := []rune(s)
	for i < len(runes) {
		r := runes[i]
		if r == '\033' {
			i = skipAnsiRune(runes, i)
			continue
		}
		w += charWidth(r)
		i++
	}
	return w
}

// ansiPrefix returns the prefix of s that covers exactly n visible columns,
// preserving all ANSI escape codes encountered along the way.
func ansiPrefix(s string, n int) string {
	if n <= 0 {
		return ""
	}
	col := 0
	var out strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r == '\033' {
			j := skipAnsiRune(runes, i)
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		cw := charWidth(r)
		if col+cw > n {
			break
		}
		col += cw
		out.WriteRune(r)
		i++
		if col >= n {
			break
		}
	}
	return out.String()
}

// ansiSuffix returns the portion of s starting at visible column n,
// prepending accumulated SGR state so colors are correct.
func ansiSuffix(s string, n int) string {
	col := 0
	var sgrState strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r == '\033' {
			j := skipAnsiRune(runes, i)
			seq := string(runes[i:j])
			if j > 0 && runes[j-1] == 'm' {
				if seq == "\033[0m" || seq == "\033[m" {
					sgrState.Reset()
				} else {
					sgrState.WriteString(seq)
				}
			}
			if col >= n {
				return sgrState.String() + string(runes[i:])
			}
			i = j
			continue
		}
		if col >= n {
			return sgrState.String() + string(runes[i:])
		}
		cw := charWidth(r)
		col += cw
		i++
	}
	return ""
}

// skipAnsiRune advances past an ANSI escape sequence starting at index i in runes.
func skipAnsiRune(runes []rune, i int) int {
	if i >= len(runes) || runes[i] != '\033' {
		return i + 1
	}
	i++ // skip ESC
	if i >= len(runes) {
		return i
	}
	if runes[i] == '[' {
		i++
		for i < len(runes) {
			c := runes[i]
			i++
			if c >= 0x40 && c <= 0x7E {
				break
			}
		}
	} else {
		i++
	}
	return i
}

// charWidth returns the display width of a single rune.
// 0 for control chars, 2 for wide CJK/emoji, 1 otherwise.
func charWidth(r rune) int {
	if r < 0x20 || (r >= 0x7F && r < 0xA0) {
		return 0
	}
	if (r >= 0x2E80 && r <= 0x9FFF) ||
		r >= 0x1F000 {
		return 2
	}
	return 1
}
