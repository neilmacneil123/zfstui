package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds default application key bindings.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
	DryRun   key.Binding
	Filter   key.Binding
	Create   key.Binding
	Destroy  key.Binding
	Restore  key.Binding
	Schedule key.Binding
}

// Keys is the default key map for the TUI.
var Keys = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "backspace"),
		key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	DryRun: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "toggle dry-run"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Create: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "create"),
	),
	Destroy: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "destroy"),
	),
	Restore: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "restore"),
	),
	Schedule: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "schedules"),
	),
}

// ShortHelp returns compact key hints for the home help bar.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.DryRun, k.Help, k.Quit}
}

// FullHelp returns all bindings for the help overlay.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.DryRun, k.Filter, k.Help, k.Quit},
		{k.Create, k.Destroy, k.Restore, k.Schedule},
	}
}
