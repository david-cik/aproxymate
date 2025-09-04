package lib

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Displayable is an interface for items that can be displayed in the selector
type Displayable interface {
	Display() string
}

// SelectorConfig holds configuration for the generic selector
type SelectorConfig[T any] struct {
	Title         string         // Main title to display
	Items         []T            // Items to select from
	DisplayFunc   func(T) string // Function to get display text (optional if T implements Displayable)
	InvalidInput  string         // Optional invalid input to highlight
	EmptyMessage  string         // Message when no items available
	CancelMessage string         // Message when user cancels
	AllowEmpty    bool           // Whether selection can be empty/cancelled
}

// SelectorModel represents a generic TUI selector
type SelectorModel[T any] struct {
	config    SelectorConfig[T]
	cursor    int
	selected  T
	quitting  bool
	forceQuit bool // true when ctrl+c was pressed
}

// NewSelector creates a new generic selector model
func NewSelector[T any](config SelectorConfig[T]) SelectorModel[T] {
	var zero T
	return SelectorModel[T]{
		config:   config,
		cursor:   0,
		selected: zero,
	}
}

// Init implements tea.Model
func (m SelectorModel[T]) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m SelectorModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// ctrl+c always quits the entire program
			m.forceQuit = true
			return m, tea.Quit

		case "q", "esc":
			if m.config.AllowEmpty {
				m.quitting = true
				return m, tea.Quit
			}

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.config.Items)-1 {
				m.cursor++
			}

		case "enter", " ":
			if m.cursor < len(m.config.Items) {
				m.selected = m.config.Items[m.cursor]
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

// View implements tea.Model
func (m SelectorModel[T]) View() string {
	if m.quitting && isZero(m.selected) && m.config.CancelMessage != "" {
		return m.config.CancelMessage + "\n"
	}

	var b strings.Builder

	// Header style
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Margin(1, 0)

	// Error style (for invalid input)
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Margin(0, 0, 1, 0)

	// Item styles
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("170")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	// Title
	b.WriteString(headerStyle.Render(m.config.Title))
	b.WriteString("\n")

	// Show invalid input error if provided
	if m.config.InvalidInput != "" {
		b.WriteString(errorStyle.Render("❌ Invalid input: '" + m.config.InvalidInput + "'"))
		b.WriteString("\n")
	}

	// Handle empty items
	if len(m.config.Items) == 0 {
		if m.config.EmptyMessage != "" {
			b.WriteString(m.config.EmptyMessage)
		} else {
			b.WriteString("No items available")
		}
		b.WriteString("\n")
		return b.String()
	}

	// Render items
	for i, item := range m.config.Items {
		cursor := " "
		displayText := m.getDisplayText(item)

		if m.cursor == i {
			cursor = ">"
			displayText = selectedStyle.Render(displayText)
		} else {
			displayText = normalStyle.Render(displayText)
		}

		b.WriteString(cursor + " " + displayText + "\n")
	}

	// Instructions
	b.WriteString("\n")
	instructionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	b.WriteString(instructionStyle.Render("↑/↓: navigate • enter: select • ctrl+c: quit"))

	if m.config.AllowEmpty {
		b.WriteString(instructionStyle.Render(" • q/esc: cancel"))
	}
	b.WriteString("\n")

	return b.String()
}

// getDisplayText returns the display text for an item
func (m SelectorModel[T]) getDisplayText(item T) string {
	// If a custom display function is provided, use it
	if m.config.DisplayFunc != nil {
		return m.config.DisplayFunc(item)
	}

	// If the item implements Displayable, use it
	if displayable, ok := any(item).(Displayable); ok {
		return displayable.Display()
	}

	// Fall back to string representation
	return fmt.Sprintf("%v", item)
}

// isZero checks if a value is the zero value of its type
func isZero[T any](v T) bool {
	var zero T
	return any(v) == any(zero)
}

// GetSelected returns the selected item
func (m SelectorModel[T]) GetSelected() T {
	return m.selected
}

// WasCancelled returns true if the user cancelled the selection
func (m SelectorModel[T]) WasCancelled() bool {
	return m.quitting && isZero(m.selected) && !m.forceQuit
}

// WasForceQuit returns true if the user pressed ctrl+c
func (m SelectorModel[T]) WasForceQuit() bool {
	return m.forceQuit
}

// RunSelector runs the selector TUI and returns the selected item
func RunSelector[T any](config SelectorConfig[T]) (T, bool, error) {
	var zero T
	model := NewSelector(config)

	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return zero, false, err
	}

	if selector, ok := finalModel.(SelectorModel[T]); ok {
		// If ctrl+c was pressed, exit the entire program
		if selector.WasForceQuit() {
			os.Exit(0)
		}
		return selector.GetSelected(), selector.WasCancelled(), nil
	}

	return zero, false, nil
}
