package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Brand colors
	Primary   = lipgloss.Color("#7C3AED") // violet
	Secondary = lipgloss.Color("#06B6D4") // cyan
	Success   = lipgloss.Color("#10B981") // emerald
	Warning   = lipgloss.Color("#F59E0B") // amber
	Error     = lipgloss.Color("#EF4444") // red
	Muted     = lipgloss.Color("#6B7280") // gray

	// Text styles
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary)

	Subtitle = lipgloss.NewStyle().
			Foreground(Muted)

	Bold = lipgloss.NewStyle().
		Bold(true)

	Highlight = lipgloss.NewStyle().
			Foreground(Secondary)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success)

	WarningStyle = lipgloss.NewStyle().
			Foreground(Warning)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error)

	MutedStyle = lipgloss.NewStyle().
			Foreground(Muted)

	// Status indicators
	StatusRunning = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	StatusStopped = lipgloss.NewStyle().
			Foreground(Muted)

	// Symbols
	SymbolSuccess = SuccessStyle.Render("✓")
	SymbolError   = ErrorStyle.Render("✗")
	SymbolWarning = WarningStyle.Render("!")
	SymbolInfo    = Highlight.Render("→")
	SymbolDot     = MutedStyle.Render("•")
)

// FormatStatus returns a styled status indicator
func FormatStatus(running bool) string {
	if running {
		return StatusRunning.Render("● running")
	}
	return StatusStopped.Render("○ stopped")
}

// FormatHash returns a styled short hash
func FormatHash(hash string) string {
	if len(hash) > 8 {
		hash = hash[:8]
	}
	return MutedStyle.Render(hash)
}
