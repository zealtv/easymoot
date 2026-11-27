package tui

import (
	"context"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zealtv/easymoot/internal/twglance"
	"github.com/zealtv/easymoot/internal/twpane"
)

// Run shows the model. For redirected output it reads the moot once, prints one
// frame and returns (the read is Init's command, run here, outside Update and View);
// interactive sessions restore the terminal through Bubble Tea and share the
// caller's cancellation context. The string is the line to print after the screen is
// gone ("" unless ctrl+c stopped a post; see Model.ExitLine).
func Run(ctx context.Context, model Model, input io.Reader, output io.Writer, interactive bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !interactive {
		if cmd := model.Init(); cmd != nil {
			next, _ := model.Update(cmd())
			model = next.(Model)
		}
		_, err := io.WriteString(output, model.View())
		return "", err
	}
	if id := paneID(os.Getenv); id != "" {
		model.twFocus.ID = id
		model.tw = twpane.New(id)
		model.twg = twglance.New(id)
	}
	p := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output),
		tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	_ = model.tw.Clear()  // @tw-next and the wait go with the screen, and nothing writes after
	_ = model.twg.Clear() // and so does the glance
	line := ""
	if fm, ok := final.(Model); ok {
		line = fm.ExitLine()
	}
	if ctx.Err() != nil {
		return line, ctx.Err()
	}
	return line, err
}

// paneID is the tmux pane Easymoot sets its options on: its own, and only inside tmux
// ($TMUX and $TMUX_PANE both set), else "".
func paneID(getenv func(string) string) string {
	if getenv("TMUX") == "" {
		return ""
	}
	return getenv("TMUX_PANE")
}
