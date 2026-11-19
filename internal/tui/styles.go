package tui

import "github.com/charmbracelet/lipgloss"

// styles follow EasyLoom's palette, as Easyhog's do (copied from Easyhog's
// internal/tui/styles.go at commit 01336f8, cut down to what this shell draws):
// the terminal's own hues, one cyan brand colour, and two greys that stay readable
// on dark and light backgrounds. lipgloss drops colour for NO_COLOR and
// redirected output, and every cue colour carries has a plain-text twin, as in
// Easyhog and EasyLoom.
type styles struct {
	brand, faint, soft, warn, selected, unread, hint, hintKey, match lipgloss.Style
	bar, barDim, thumb                                               lipgloss.Style
	border                                                           lipgloss.Style

	// A post's heading: the sender's name, and a mention of someone in the text.
	sender, mention lipgloss.Style

	// A post's text. The markdown ones follow EasyLoom's.
	mdHeading, mdStrong, mdEmphasis, mdInlineCode, mdCode lipgloss.Style
	mdBullet, mdQuote, mdLink, mdRule                     lipgloss.Style
}

// colourOn says whether lipgloss will emit colour at all: it does not for
// NO_COLOR, a dumb terminal or redirected output. The renderer keeps a post's
// markers exactly as written when it does not, so it asks once, up front.
func colourOn() bool {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("x") != "x"
}

func newStyles() styles {
	faint := lipgloss.AdaptiveColor{Light: "243", Dark: "244"}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return styles{
		brand:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		faint:    lipgloss.NewStyle().Foreground(faint),
		soft:     lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "248"}),
		warn:     lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
		// A room with posts that arrived since it was last selected: bold in the brand
		// cyan, and a bullet in the marker column as its plain-text twin.
		unread:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		hint:    dim,
		hintKey: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
		match:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Underline(true),
		bar:     lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"}),
		barDim:  lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "255", Dark: "235"}),
		thumb:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		border:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")),

		sender: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
		// moot.sh read highlights a mention in bold cyan (moot.sh:643).
		mention: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),

		mdHeading:    lipgloss.NewStyle().Bold(true),
		mdStrong:     lipgloss.NewStyle().Bold(true),
		mdEmphasis:   lipgloss.NewStyle().Italic(true),
		mdInlineCode: lipgloss.NewStyle().Foreground(lipgloss.Color("151")),
		mdCode:       dim,
		mdBullet:     dim,
		mdQuote:      dim,
		mdLink:       lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
		mdRule:       dim,
	}
}
