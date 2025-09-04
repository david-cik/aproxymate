package lib

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfigLocation represents a config file location option
type ConfigLocation struct {
	Path        string
	DisplayName string
	Description string
}

// Display implements the Displayable interface
func (c ConfigLocation) Display() string {
	return fmt.Sprintf("%s (%s)", c.DisplayName, c.Description)
}

// SelectFromSlice is a generic convenience function for selecting from any slice of strings
func SelectFromSlice[T ~string](title string, items []T, emptyMessage string) (T, error) {
	var zero T
	if len(items) == 0 {
		return zero, fmt.Errorf("no items available")
	}

	config := SelectorConfig[T]{
		Title:         title,
		Items:         items,
		EmptyMessage:  emptyMessage,
		CancelMessage: "Selection cancelled",
		AllowEmpty:    true,
	}

	selected, cancelled, err := RunSelector(config)
	if err != nil {
		return zero, fmt.Errorf("failed to run selection: %w", err)
	}

	if cancelled {
		return zero, fmt.Errorf("selection cancelled")
	}

	return selected, nil
}

// SelectKubernetesClusterTUI uses the generic selector for cluster selection
func SelectKubernetesClusterTUI(invalidCluster string) (string, error) {
	clusters, err := GetKubernetesContexts("")
	if err != nil {
		return "", fmt.Errorf("failed to get available Kubernetes contexts: %w", err)
	}

	if len(clusters) == 0 {
		return "", fmt.Errorf("no Kubernetes contexts found in kubeconfig. Please ensure kubectl is configured with at least one cluster")
	}

	config := SelectorConfig[string]{
		Title:         "Select Kubernetes Cluster:",
		Items:         clusters,
		InvalidInput:  invalidCluster,
		EmptyMessage:  "No Kubernetes contexts found in kubeconfig",
		CancelMessage: "Cluster selection cancelled",
		AllowEmpty:    true,
	}

	selected, cancelled, err := RunSelector(config)
	if err != nil {
		return "", fmt.Errorf("failed to run cluster selection TUI: %w", err)
	}

	if cancelled {
		return "", fmt.Errorf("no cluster selected")
	}

	return selected, nil
}

// SelectAWSProfileTUI uses the generic selector for AWS profile selection
func SelectAWSProfileTUI() (string, error) {
	profiles, err := ParseAWSProfiles()
	if err != nil {
		return "", fmt.Errorf("failed to parse AWS profiles: %w", err)
	}

	return SelectFromSlice("Select AWS Profile:", profiles, "No AWS profiles found. Please configure AWS CLI with 'aws configure'")
}

// SelectAWSRegionTUI uses the generic selector for AWS region selection
func SelectAWSRegionTUI() (string, error) {
	return SelectFromSlice("Select AWS Region:", standardUSRegions, "No AWS regions available")
}

// SelectConfigLocationTUI uses the generic selector for config location selection
func SelectConfigLocationTUI(locations []ConfigLocation) (string, error) {
	if len(locations) == 0 {
		return "", fmt.Errorf("no config locations available")
	}

	config := SelectorConfig[ConfigLocation]{
		Title:         "📍 Select Configuration File Location",
		Items:         locations,
		EmptyMessage:  "No locations available",
		CancelMessage: "Location selection cancelled",
		AllowEmpty:    true,
	}

	selected, cancelled, err := RunSelector(config)
	if err != nil {
		return "", fmt.Errorf("failed to run location selection TUI: %w", err)
	}

	if cancelled {
		return "", fmt.Errorf("location selection cancelled")
	}

	return selected.Path, nil
}

// PromptConfigLocationTUI prompts the user to select a configuration file location
func PromptConfigLocationTUI() (location string, cancelled bool, err error) {
	locations := GetConfigLocations()
	selectedLocation, err := SelectConfigLocationTUI(locations)
	if err != nil {
		// Check if it was cancelled
		if err.Error() == "location selection cancelled" {
			return "", true, nil
		}
		return "", false, err
	}

	return selectedLocation, false, nil
}

// PromptConfigCreationTUI handles the complete config creation workflow (deprecated - use PromptConfigLocationTUI for location selection only)
func PromptConfigCreationTUI() (shouldCreate bool, location string, cancelled bool, err error) {
	// First ask if user wants to create a config file
	shouldCreate, cancelled, err = ConfirmConfigCreationTUI()
	if err != nil {
		return false, "", false, err
	}

	if !shouldCreate || cancelled {
		return shouldCreate, "", cancelled, nil
	}

	// If user wants to create config, select location
	locations := GetConfigLocations()
	selectedLocation, err := SelectConfigLocationTUI(locations)
	if err != nil {
		// Check if it was cancelled
		if err.Error() == "location selection cancelled" {
			return false, "", true, nil
		}
		return false, "", false, err
	}

	return true, selectedLocation, false, nil
}

// ConfirmConfigCreationTUI asks the user if they want to create a config file
func ConfirmConfigCreationTUI() (shouldCreate bool, cancelled bool, err error) {
	// Create items for yes/no selection
	items := []string{"Yes, create a sample configuration file", "No, continue without a config file"}

	title := "📝 Configuration File Not Found\n\n" +
		"⚠️  No configuration file was found in the standard locations.\n\n" +
		"Would you like to create a sample configuration file?\n" +
		"The sample will include example proxy configurations and detailed comments."

	selected, err := SelectFromSlice(title, items, "No options available")
	if err != nil {
		if err.Error() == "selection cancelled" {
			return false, true, nil
		}
		return false, false, fmt.Errorf("failed to run config creation confirmation: %w", err)
	}

	// Check which option was selected
	shouldCreate = (selected == items[0]) // "Yes, create..."

	return shouldCreate, false, nil
}

// TextInputModel represents a simple text input TUI
type TextInputModel struct {
	textInput   textinput.Model
	title       string
	placeholder string
	quitting    bool
	cancelled   bool
	forceQuit   bool
}

// NewTextInput creates a new text input model
func NewTextInput(title, placeholder string) TextInputModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 500 // Reasonable limit for names
	ti.Width = 50

	return TextInputModel{
		textInput:   ti,
		title:       title,
		placeholder: placeholder,
	}
}

// Init implements tea.Model
func (m TextInputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m TextInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.forceQuit = true
			return m, tea.Quit

		case "esc":
			m.cancelled = true
			m.quitting = true
			return m, tea.Quit

		case "enter":
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View implements tea.Model
func (m TextInputModel) View() string {
	if m.quitting && m.cancelled {
		return "Input cancelled.\n"
	}

	var b strings.Builder

	// Header style
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Margin(1, 0)

	// Instruction style
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Margin(1, 0)

	// Title
	b.WriteString(headerStyle.Render(m.title))
	b.WriteString("\n")

	// Text input
	b.WriteString(m.textInput.View())
	b.WriteString("\n")

	// Instructions
	b.WriteString(instructionStyle.Render("enter: submit • esc: cancel • ctrl+c: quit"))
	b.WriteString("\n")

	return b.String()
}

// GetInput returns the input value
func (m TextInputModel) GetInput() string {
	return strings.TrimSpace(m.textInput.Value())
}

// WasCancelled returns true if the user cancelled the input
func (m TextInputModel) WasCancelled() bool {
	return m.cancelled
}

// WasForceQuit returns true if the user pressed ctrl+c
func (m TextInputModel) WasForceQuit() bool {
	return m.forceQuit
}

// PromptTextInput runs the text input TUI and returns the input
func PromptTextInput(title, placeholder string) (string, bool, error) {
	model := NewTextInput(title, placeholder)
	program := tea.NewProgram(model)

	finalModel, err := program.Run()
	if err != nil {
		return "", false, err
	}

	if textInputModel, ok := finalModel.(TextInputModel); ok {
		// If ctrl+c was pressed, exit the entire program
		if textInputModel.WasForceQuit() {
			return "", false, fmt.Errorf("operation cancelled by user")
		}
		return textInputModel.GetInput(), textInputModel.WasCancelled(), nil
	}

	return "", false, fmt.Errorf("unexpected model type")
}

// PromptForNamesFilter prompts user to decide if they want to filter by names and get the names
func PromptForNamesFilter() (wantsFilter bool, names string, cancelled bool, err error) {
	// First ask if they want to filter by names
	items := []string{"Yes, filter by specific RDS instance/cluster names", "No, import all available RDS endpoints"}

	title := "🔍 Filter RDS Endpoints by Names\n\n" +
		"Would you like to filter RDS endpoints by specific instance or cluster names?\n" +
		"This allows you to import only the databases you need."

	selected, err := SelectFromSlice(title, items, "No options available")
	if err != nil {
		if err.Error() == "selection cancelled" {
			return false, "", true, nil
		}
		return false, "", false, fmt.Errorf("failed to run name filter selection: %w", err)
	}

	// Check if user wants to filter
	wantsFilter = (selected == items[0])

	if !wantsFilter {
		return false, "", false, nil
	}

	// If they want to filter, prompt for names
	inputTitle := "🏷️  Enter RDS Instance/Cluster Names\n\n" +
		"Enter a comma-separated list of RDS instance or cluster names to filter by.\n" +
		"Partial matching is supported (e.g., 'prod' will match 'prod-db-cluster').\n\n" +
		"Examples:\n" +
		"• prod-db,staging-cluster\n" +
		"• user-service\n" +
		"• mysql-prod,postgres-dev"

	placeholder := "prod-db,staging-cluster,user-service"

	namesInput, inputCancelled, err := PromptTextInput(inputTitle, placeholder)
	if err != nil {
		return false, "", false, fmt.Errorf("failed to get names input: %w", err)
	}

	if inputCancelled {
		return false, "", true, nil
	}

	return true, namesInput, false, nil
}

// PromptRDSImportConfirmation prompts user to confirm the RDS import with a detailed summary
func PromptRDSImportConfirmation(newConfigs []ProxyConfig, existingCount int) (confirmed bool, cancelled bool, err error) {
	if len(newConfigs) == 0 {
		return false, false, fmt.Errorf("no configurations to import")
	}

	// Build a detailed summary of what will be imported
	var summaryBuilder strings.Builder
	summaryBuilder.WriteString("📋 RDS Import Summary\n\n")
	summaryBuilder.WriteString(fmt.Sprintf("The following %d RDS instance(s) will be imported:\n\n", len(newConfigs)))

	summaryBuilder.WriteString("\n📊 Configuration Summary:\n")
	summaryBuilder.WriteString(fmt.Sprintf("  • Existing configurations: %d\n", existingCount))
	summaryBuilder.WriteString(fmt.Sprintf("  • New configurations: %d\n", len(newConfigs)))
	summaryBuilder.WriteString(fmt.Sprintf("  • Total after import: %d\n", existingCount+len(newConfigs)))

	summaryBuilder.WriteString("\n🤔 Do you want to proceed with importing these RDS instances?")

	// Create confirmation options
	items := []string{
		"✅ Yes, import all RDS instances",
		"❌ No, cancel the import",
	}

	selected, err := SelectFromSlice(summaryBuilder.String(), items, "No options available")
	if err != nil {
		if err.Error() == "selection cancelled" {
			return false, true, nil
		}
		return false, false, fmt.Errorf("failed to run RDS import confirmation: %w", err)
	}

	// Check if user confirmed the import
	confirmed = (selected == items[0])

	return confirmed, false, nil
}
