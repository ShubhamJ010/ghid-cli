package tui

import (
	"fmt"
	"strings"

	"github.com/ShubhamJ010/ghid-cli/internal/combos"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ComboSendFunc transmits one saved combo through the HID gateway.
type ComboSendFunc func(combo combos.Combo) error

// ComboStudioModel manages the interactive TUI for viewing, editing, and creating
// cross-platform combos with the unified bracket sequence DSL.
type ComboStudioModel struct {
	store    *combos.Store
	sendFn   ComboSendFunc
	list     []combos.Combo
	cursor   int
	quitting bool
	busy     bool
	notice   string
	isError  bool

	// Sub-states: 0 = list view, 1 = create/edit form, 2 = delete confirmation
	mode int

	// Form inputs: 0: name, 1: sequence, 2: os, 3: emoji, 4: desc
	formActiveIndex int
	inputName       textinput.Model
	inputSeq        textinput.Model
	inputOS         textinput.Model
	inputEmoji      textinput.Model
	inputDesc       textinput.Model
	editOrigName    string

	width  int
	height int
}

// NewComboStudioModel creates a new Combo Studio model.
func NewComboStudioModel(store *combos.Store, sendFn ComboSendFunc) ComboStudioModel {
	inName := textinput.New()
	inName.Placeholder = "Name (e.g. VS Code Format)"
	inName.CharLimit = 40
	inName.Width = 36

	inSeq := textinput.New()
	inSeq.Placeholder = "Sequence (e.g. {mod+shift+p}Format{Enter}{500ms}{mod+s})"
	inSeq.CharLimit = 256
	inSeq.Width = 48

	inOS := textinput.New()
	inOS.Placeholder = "OS: windows, macos, linux, or all"
	inOS.CharLimit = 16
	inOS.Width = 24

	inEmoji := textinput.New()
	inEmoji.Placeholder = "Icon: optional emoji (e.g. ⚡)"
	inEmoji.CharLimit = 4
	inEmoji.Width = 10

	inDesc := textinput.New()
	inDesc.Placeholder = "Description (optional)"
	inDesc.CharLimit = 80
	inDesc.Width = 48

	m := ComboStudioModel{
		store:           store,
		sendFn:          sendFn,
		mode:            0,
		formActiveIndex: 0,
		inputName:       inName,
		inputSeq:        inSeq,
		inputOS:         inOS,
		inputEmoji:      inEmoji,
		inputDesc:       inDesc,
	}

	if store != nil {
		m.list, _ = store.Load()
	}

	return m
}

func (m ComboStudioModel) Init() tea.Cmd {
	return textinput.Blink
}

type comboResultMsg struct {
	name     string
	sequence string
	err      error
}

func (m ComboStudioModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case 0: // List mode
			switch msg.String() {
			case "q", "esc":
				m.quitting = true
				return m, nil

			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				} else if len(m.list) > 0 {
					m.cursor = len(m.list) - 1
				}
				m.notice = ""

			case "down", "j":
				if m.cursor < len(m.list)-1 {
					m.cursor++
				} else {
					m.cursor = 0
				}
				m.notice = ""

			case "f", "fav":
				if len(m.list) > 0 && m.cursor < len(m.list) {
					c := m.list[m.cursor]
					newList, _, err := m.store.ToggleFavorite(c.Name)
					if err == nil {
						m.list = newList
						m.notice = fmt.Sprintf("Toggled favorite for %q", c.Name)
						m.isError = false
					}
				}

			case "n", "a": // New / Add combo
				m.mode = 1
				m.editOrigName = ""
				m.inputName.SetValue("")
				m.inputSeq.SetValue("")
				m.inputOS.SetValue("windows")
				m.inputEmoji.SetValue("")
				m.inputDesc.SetValue("")
				m.formActiveIndex = 0
				m.inputName.Focus()
				m.inputSeq.Blur()
				m.inputOS.Blur()
				m.inputEmoji.Blur()
				m.inputDesc.Blur()
				m.notice = ""
				return m, textinput.Blink

			case "e": // Edit combo
				if len(m.list) > 0 && m.cursor < len(m.list) {
					c := m.list[m.cursor]
					m.mode = 1
					m.editOrigName = c.Name
					m.inputName.SetValue(c.Name)
					m.inputSeq.SetValue(c.Sequence)
					m.inputOS.SetValue(c.OS)
					m.inputEmoji.SetValue(c.Emoji)
					m.inputDesc.SetValue(c.Description)
					m.formActiveIndex = 0
					m.inputName.Focus()
					m.inputSeq.Blur()
					m.inputOS.Blur()
					m.inputEmoji.Blur()
					m.inputDesc.Blur()
					m.notice = ""
					return m, textinput.Blink
				}

			case "d", "x": // Delete confirmation
				if len(m.list) > 0 && m.cursor < len(m.list) {
					m.mode = 2
					m.notice = ""
					return m, nil
				}

			case "enter", "s": // Send combo
				if len(m.list) > 0 && m.cursor < len(m.list) && !m.busy {
					c := m.list[m.cursor]
					m.busy = true
					m.notice = fmt.Sprintf("Sending %q...", c.Name)
					m.isError = false
					return m, func() tea.Msg {
						var err error
						if m.sendFn != nil {
							err = m.sendFn(c)
						}
						return comboResultMsg{name: c.Name, sequence: c.Sequence, err: err}
					}
				}
			}

		case 1: // Form edit/create mode
			switch msg.String() {
			case "esc":
				m.mode = 0
				m.notice = ""
				return m, nil

			case "tab", "down":
				m.formActiveIndex = (m.formActiveIndex + 1) % 5
				m.updateFormFocus()
				return m, textinput.Blink

			case "shift+tab", "up":
				m.formActiveIndex = (m.formActiveIndex - 1 + 5) % 5
				m.updateFormFocus()
				return m, textinput.Blink

			case "enter":
				name := strings.TrimSpace(m.inputName.Value())
				seq := strings.TrimSpace(m.inputSeq.Value())
				osVal := strings.TrimSpace(m.inputOS.Value())
				emojiVal := strings.TrimSpace(m.inputEmoji.Value())
				descVal := strings.TrimSpace(m.inputDesc.Value())

				if name == "" {
					m.notice = "Name cannot be empty"
					m.isError = true
					return m, nil
				}
				if seq == "" {
					m.notice = "Sequence cannot be empty"
					m.isError = true
					return m, nil
				}

				if err := combos.Validate(seq); err != nil {
					m.notice = fmt.Sprintf("Invalid sequence: %v", err)
					m.isError = true
					return m, nil
				}

				normOS, err := combos.NormalizeOS(osVal)
				if err != nil {
					m.notice = fmt.Sprintf("Invalid OS: %v", err)
					m.isError = true
					return m, nil
				}

				cleanEmoji, err := combos.ValidateEmoji(emojiVal)
				if err != nil {
					m.notice = fmt.Sprintf("Invalid emoji: %v", err)
					m.isError = true
					return m, nil
				}

				// If renaming existing combo, delete old one first
				if m.editOrigName != "" && !strings.EqualFold(m.editOrigName, name) {
					_, _, _ = m.store.Delete(m.editOrigName)
				}

				// Preserve favorite flag if editing
				fav := false
				if m.editOrigName != "" {
					if old, ok := m.store.Find(m.editOrigName); ok {
						fav = old.Favorite
					}
				}

				combo := combos.Combo{
					Name:        name,
					Sequence:    seq,
					OS:          normOS,
					Emoji:       cleanEmoji,
					Description: descVal,
					Favorite:    fav,
				}

				newList, err := m.store.Add(combo)
				if err != nil {
					m.notice = fmt.Sprintf("Save failed: %v", err)
					m.isError = true
					return m, nil
				}

				m.list = newList
				m.mode = 0
				m.notice = fmt.Sprintf("✔ Saved combo %q", name)
				m.isError = false
				return m, nil
			}

			// Forward key to currently focused input
			var cmd tea.Cmd
			switch m.formActiveIndex {
			case 0:
				m.inputName, cmd = m.inputName.Update(msg)
			case 1:
				m.inputSeq, cmd = m.inputSeq.Update(msg)
			case 2:
				m.inputOS, cmd = m.inputOS.Update(msg)
			case 3:
				m.inputEmoji, cmd = m.inputEmoji.Update(msg)
			case 4:
				m.inputDesc, cmd = m.inputDesc.Update(msg)
			}
			return m, cmd

		case 2: // Delete confirmation
			switch msg.String() {
			case "y", "enter":
				if len(m.list) > 0 && m.cursor < len(m.list) {
					c := m.list[m.cursor]
					newList, _, err := m.store.Delete(c.Name)
					if err != nil {
						m.notice = fmt.Sprintf("Delete failed: %v", err)
						m.isError = true
					} else {
						m.list = newList
						if m.cursor >= len(m.list) && m.cursor > 0 {
							m.cursor = len(m.list) - 1
						}
						m.notice = fmt.Sprintf("✔ Deleted combo %q", c.Name)
						m.isError = false
					}
				}
				m.mode = 0
				return m, nil

			case "n", "esc":
				m.mode = 0
				m.notice = ""
				return m, nil
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *ComboStudioModel) updateFormFocus() {
	m.inputName.Blur()
	m.inputSeq.Blur()
	m.inputOS.Blur()
	m.inputEmoji.Blur()
	m.inputDesc.Blur()

	switch m.formActiveIndex {
	case 0:
		m.inputName.Focus()
	case 1:
		m.inputSeq.Focus()
	case 2:
		m.inputOS.Focus()
	case 3:
		m.inputEmoji.Focus()
	case 4:
		m.inputDesc.Focus()
	}
}

func (m ComboStudioModel) View() string {
	cardWidth := calculateCardWidth(m.width)

	var sb strings.Builder
	title := "🎹 COMBO STUDIO · Universal Cross-Platform Sequences"
	sb.WriteString(TitleStyle.Render(title))
	sb.WriteString("\n\n")

	switch m.mode {
	case 1:
		sb.WriteString(m.renderForm())
	case 2:
		sb.WriteString(m.renderDeleteConfirm())
	default:
		sb.WriteString(m.renderList())
	}

	sb.WriteString("\n")
	if m.notice != "" {
		style := SuccessStyle
		if m.isError {
			style = ErrorStyle
		}
		sb.WriteString(style.Render(m.notice))
		sb.WriteString("\n\n")
	}

	sb.WriteString(m.renderFooter())

	return FocusCardStyle.Width(cardWidth).Render(sb.String())
}

func (m ComboStudioModel) renderForm() string {
	title := "Add New Combo"
	if m.editOrigName != "" {
		title = fmt.Sprintf("Edit Combo: %s", m.editOrigName)
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(title))
	sb.WriteString("\n\n")

	labels := []string{
		"Combo Name:",
		"Sequence ({delay}, {chord}, {key}, {PAYLOAD}):",
		"Target OS (windows, macos, linux, all):",
		"Badge Emoji (e.g. ⚡, 🍎, 🪟, 🔒):",
		"Description (optional):",
	}

	inputs := []*textinput.Model{
		&m.inputName,
		&m.inputSeq,
		&m.inputOS,
		&m.inputEmoji,
		&m.inputDesc,
	}

	for i := 0; i < 5; i++ {
		prefix := "  "
		lblStyle := lipgloss.NewStyle().Foreground(ColorFgMuted)
		if i == m.formActiveIndex {
			prefix = "▶ "
			lblStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorCyan)
		}
		sb.WriteString(lblStyle.Render(prefix + labels[i]))
		sb.WriteString("\n")
		sb.WriteString("  " + inputs[i].View())
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func (m ComboStudioModel) renderDeleteConfirm() string {
	cName := ""
	if len(m.list) > 0 && m.cursor < len(m.list) {
		cName = m.list[m.cursor].Name
	}

	content := fmt.Sprintf(
		"Are you sure you want to delete combo %q?\n\n[ Y ] Yes, delete   [ N / Esc ] Cancel",
		cName,
	)
	return ErrorStyle.Render(content) + "\n"
}

func (m ComboStudioModel) renderList() string {
	if len(m.list) == 0 {
		return lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  No combos saved yet. Press [n] to create one.\n")
	}

	colFavNum := 6
	nameWidth := 20
	seqWidth := 34
	descWidth := 16

	for _, c := range m.list {
		nw := len(c.Name) + 5
		if nw > nameWidth {
			nameWidth = nw
		}
		sw := len(c.Sequence)
		if sw > seqWidth {
			seqWidth = sw
		}
		dw := len(c.Description)
		if dw > descWidth {
			descWidth = dw
		}
	}

	maxTableWidth := m.width - 12
	if maxTableWidth < 60 {
		maxTableWidth = 60
	}
	totalNeeded := colFavNum + nameWidth + seqWidth + descWidth + 8
	if totalNeeded > maxTableWidth {
		excess := totalNeeded - maxTableWidth
		seqWidth -= excess / 2
		if seqWidth < 18 {
			seqWidth = 18
		}
		descWidth -= excess / 2
		if descWidth < 10 {
			descWidth = 10
		}
	}

	headerPrefix := "  "
	headerFavNum := padRight("  #", colFavNum)
	headerName := padRight("NAME", nameWidth)
	headerSeq := padRight("SEQUENCE", seqWidth)
	headerDesc := padRight("DESCRIPTION", descWidth)
	headerLine := fmt.Sprintf("%s%s %s %s %s", headerPrefix, headerFavNum, headerName, headerSeq, headerDesc)
	header := lipgloss.NewStyle().Foreground(ColorFgMuted).Render(headerLine)

	var rows []string
	rows = append(rows, header)
	for i, c := range m.list {
		var favNumCol string
		if c.Favorite {
			numStr := fmt.Sprintf("%-2d", i+1)
			favNumCol = padRight("❤️ "+numStr, colFavNum)
		} else {
			favNumCol = padRight(fmt.Sprintf("   %-2d", i+1), colFavNum)
		}

		osBadge := c.Badge()
		displayName := osBadge + " " + c.Name
		nameCol := padRight(truncate(displayName, nameWidth), nameWidth)
		seqCol := padRight(truncate(c.Sequence, seqWidth), seqWidth)
		descCol := padRight(truncate(c.Description, descWidth), descWidth)

		prefix := "  "
		line := fmt.Sprintf("%s%s %s %s %s", prefix, favNumCol, nameCol, seqCol, descCol)

		if i == m.cursor {
			selectedStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan).
				Background(lipgloss.Color("#26233a"))
			line = selectedStyle.Render("▸ " + line[2:])
		}
		rows = append(rows, line)
	}

	return strings.Join(rows, "\n") + "\n"
}

func (m ComboStudioModel) renderFooter() string {
	switch m.mode {
	case 1:
		return HelpStyle.Render("[Tab / ↓]: Next field • [Enter]: Save combo • [Esc]: Cancel")
	case 2:
		return HelpStyle.Render("[Y / Enter]: Delete • [N / Esc]: Cancel")
	default:
		return HelpStyle.Render("↑/↓: Navigate • [Enter / s]: Send • [n]: New • [e]: Edit • [f]: Fav • [d]: Delete • [Esc/q]: Back")
	}
}

// RunComboStudio runs the Combo Studio standalone in a Bubble Tea program.
func RunComboStudio(store *combos.Store, sendFn ComboSendFunc) error {
	m := NewComboStudioModel(store, sendFn)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
