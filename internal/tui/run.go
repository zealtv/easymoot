package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run shows the model. For redirected output it reads the moot once, prints one
// frame and returns (the read is Init's command, run here, outside Update and View);
// interactive sessions restore the terminal through Bubble Tea and share the
// caller's cancellation context.
func Run(ctx context.Context, model Model, input io.Reader, output io.Writer, interactive bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !interactive {
		if cmd := model.Init(); cmd != nil {
			next, _ := model.Update(cmd())
			model = next.(Model)
		}
		_, err := io.WriteString(output, model.View())
		return err
	}
	p := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output),
		tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
