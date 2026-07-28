package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmMode selects how the user affirms the action.
type ConfirmMode int

const (
	// ConfirmYesNo: y/enter = yes, n/esc = no.
	ConfirmYesNo ConfirmMode = iota
	// ConfirmType: user must type Require exactly (case-sensitive) then enter.
	ConfirmType
)

// ConfirmResultMsg is emitted when a confirm modal closes.
type ConfirmResultMsg struct {
	// ID is the caller-supplied correlation token (e.g. "create", "destroy").
	ID string
	// Confirmed is true when the user accepted the action.
	Confirmed bool
	// Value is optional payload set by the opener (e.g. recursive flag encoded).
	Value string
}

// Confirm is a reusable yes/no or typed-confirm modal.
type Confirm struct {
	Active      bool
	Mode        ConfirmMode
	ID          string
	Title       string
	Body        string
	Require     string // for ConfirmType: exact string user must type
	Value       string // opaque payload returned in ConfirmResultMsg
	Input       string // typed buffer
	Danger      bool   // red emphasis for destructive actions
	Width       int
	DefaultYes  bool // for YesNo: default focus on Yes
	yesSelected bool
}

// OpenYesNo prepares a yes/no confirm (not yet Active until returned model is used).
func OpenYesNo(id, title, body, value string, danger bool) Confirm {
	return Confirm{
		Active:      true,
		Mode:        ConfirmYesNo,
		ID:          id,
		Title:       title,
		Body:        body,
		Value:       value,
		Danger:      danger,
		DefaultYes:  !danger,
		yesSelected: !danger,
	}
}

// OpenType prepares a typed confirm requiring the user to enter require exactly.
func OpenType(id, title, body, require, value string) Confirm {
	return Confirm{
		Active:  true,
		Mode:    ConfirmType,
		ID:      id,
		Title:   title,
		Body:    body,
		Require: require,
		Value:   value,
		Danger:  true,
	}
}

// Init implements tea.Model.
func (c Confirm) Init() tea.Cmd { return nil }

// Update handles confirm input. When inactive, returns unchanged.
func (c Confirm) Update(msg tea.Msg) (Confirm, tea.Cmd) {
	if !c.Active {
		return c, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch c.Mode {
		case ConfirmYesNo:
			return c.updateYesNo(msg)
		case ConfirmType:
			return c.updateType(msg)
		}
	}
	return c, nil
}

func (c Confirm) updateYesNo(msg tea.KeyMsg) (Confirm, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		c.Active = false
		return c, resultCmd(c.ID, false, c.Value)
	case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h", "right", "l", "tab"))):
		c.yesSelected = !c.yesSelected
	case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
		c.Active = false
		return c, resultCmd(c.ID, true, c.Value)
	case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N"))):
		c.Active = false
		return c, resultCmd(c.ID, false, c.Value)
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
		ok := c.yesSelected
		c.Active = false
		return c, resultCmd(c.ID, ok, c.Value)
	}
	return c, nil
}

func (c Confirm) updateType(msg tea.KeyMsg) (Confirm, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		c.Active = false
		return c, resultCmd(c.ID, false, c.Value)
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		ok := c.Input == c.Require
		c.Active = false
		return c, resultCmd(c.ID, ok, c.Value)
	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if len(c.Input) > 0 {
			c.Input = c.Input[:len(c.Input)-1]
		}
	case msg.Type == tea.KeyRunes:
		c.Input += string(msg.Runes)
	}
	return c, nil
}

func resultCmd(id string, confirmed bool, value string) tea.Cmd {
	return func() tea.Msg {
		return ConfirmResultMsg{ID: id, Confirmed: confirmed, Value: value}
	}
}

// View renders the modal panel.
func (c Confirm) View() string {
	if !c.Active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	if c.Danger {
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f87171"))
	}
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	borderColor := lipgloss.Color("#e879f9")
	if c.Danger {
		borderColor = lipgloss.Color("#f87171")
	}

	var lines []string
	lines = append(lines, titleStyle.Render(c.Title))
	lines = append(lines, "")
	for _, ln := range strings.Split(c.Body, "\n") {
		lines = append(lines, bodyStyle.Render(ln))
	}
	lines = append(lines, "")

	switch c.Mode {
	case ConfirmYesNo:
		yes := " Yes "
		no := " No "
		sel := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0d1117")).
			Background(lipgloss.Color("#22d3ee")).
			Bold(true)
		if c.Danger {
			sel = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0d1117")).
				Background(lipgloss.Color("#f87171")).
				Bold(true)
		}
		unsel := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e6edf3")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 0)
		// Simpler: highlight without nested border.
		unsel = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Padding(0, 1)
		if c.yesSelected {
			yes = sel.Render(yes)
			no = unsel.Render(no)
		} else {
			yes = unsel.Render(yes)
			no = sel.Render(no)
		}
		lines = append(lines, yes+"  "+no)
		lines = append(lines, "")
		lines = append(lines, muted.Render("y/n · enter confirm · esc cancel"))
	case ConfirmType:
		prompt := muted.Render("Type ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24")).Bold(true).Render(c.Require) +
			muted.Render(" to confirm:")
		lines = append(lines, prompt)
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e6edf3")).
			Background(lipgloss.Color("#161b22")).
			Padding(0, 1)
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Render("▌")
		lines = append(lines, inputStyle.Render(c.Input)+cursor)
		lines = append(lines, "")
		lines = append(lines, muted.Render("enter confirm · esc cancel"))
	}

	w := c.Width
	if w < 40 {
		w = 48
	}
	if w > 72 {
		w = 72
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(w).
		Render(strings.Join(lines, "\n"))
	return panel
}
