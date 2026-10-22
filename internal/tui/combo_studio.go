package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/combos"
)

// ComboSendFunc transmits one saved combo through the HID gateway.
type ComboSendFunc func(combo combos.Combo) error

type studioMode int

const (
	studioList studioMode = iota
	studioCreate
	studioConfirmDelete
)

type comboResultMsg struct {
	name string
	keys string
	err  error
}

// ComboStudioModel is the interactive Windows keyboard/mouse combo manager
// behind `ghid combo`.
type ComboStudioModel struct {
	store *combos.Store
	send  ComboSendFunc

	list       []combos.Combo
	index      int
	focus      int // 0 = list, 1 = SEND, 2 = NEW, 3 = DELETE
	mode       studioMode
	formFocus  int // 0 = name, 1 = keys, 2 = save, 3 = save & send
	nameInput  textinput.Model
	keysInput  textinput.Model
	editing    string
	notice     string
	isError    bool
	busy       bool
	pendingDel *combos.Combo
	width      int
	height     int
	quitting   bool
}

// NewComboStudioModel loads the saved library and builds the studio model.
func NewComboStudioModel(store *combos.Store, send ComboSendFunc) ComboStudioModel {
	name := textinput.New()
	name.Placeholder = "e.g. Open Run dialog"
	name.CharLimit = 40
	name.Width = 42

	keys := textinput.New()
	keys.Placeholder = "win+s ->{0.6} type:rustdesk ->{0.3} enter"
	keys.CharLimit = 256
	keys.Width = 42

	list, err := store.Load()

	m := ComboStudioModel{
		store:     store,
		send:      send,
		list:      list,
		nameInput: name,
		keysInput: keys,
	}
	if err != nil {
		m.notice = fmt.Sprintf("Could not read combos: %v", err)
		m.isError = true
	}
	if len(list) == 0 {
		m.notice = "No combos saved yet. Press n to create your first Windows keyboard or mouse combo."
		m.isError = false
	}
	return m
}

// RunComboStudio opens the full-screen combo studio.
func RunComboStudio(store *combos.Store, send ComboSendFunc) error {
	model := NewComboStudioModel(store, send)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m ComboStudioModel) Init() tea.Cmd {
	return nil
}

func (m ComboStudioModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case comboResultMsg:
		m.busy = false
		if msg.err != nil {
			m.notice = fmt.Sprintf("✖ Send failed: %v", msg.err)
			m.isError = true
		} else {
			m.notice = fmt.Sprintf("✔ Sent %q  →  %s", msg.name, msg.keys)
			m.isError = false
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		switch m.mode {
		case studioCreate:
			return m.updateCreate(msg)
		case studioConfirmDelete:
			return m.updateConfirmDelete(msg)
		default:
			return m.updateList(msg)
		}
	}

	return m, nil
}

func (m ComboStudioModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "q", "esc":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.focus == 0 {
			if m.index > 0 {
				m.index--
			} else if len(m.list) > 0 {
				m.index = len(m.list) - 1
			}
		} else {
			m.focus--
			if m.focus < 1 {
				m.focus = 3
			}
		}
		return m, nil

	case "down", "j":
		if m.focus == 0 {
			if m.index < len(m.list)-1 {
				m.index++
			} else {
				m.index = 0
			}
		} else {
			m.focus++
			if m.focus > 3 {
				m.focus = 1
			}
		}
		return m, nil

	case "tab":
		m.focus = (m.focus + 1) % 4
		return m, nil

	case "shift+tab":
		m.focus = (m.focus + 3) % 4
		return m, nil

	case "n", "a":
		return m.openCreate(combos.Combo{})

	case "e":
		if len(m.list) > 0 && m.index < len(m.list) {
			return m.openCreate(m.list[m.index])
		}
		return m, nil

	case "s":
		return m.sendSelected()

	case "d", "x", "delete":
		if len(m.list) > 0 && m.index < len(m.list) {
			combo := m.list[m.index]
			m.pendingDel = &combo
			m.mode = studioConfirmDelete
			m.notice = ""
		}
		return m, nil

	case "enter":
		switch m.focus {
		case 0:
			return m.sendSelected()
		case 1:
			return m.sendSelected()
		case 2:
			return m.openCreate(combos.Combo{})
		case 3:
			if len(m.list) > 0 && m.index < len(m.list) {
				combo := m.list[m.index]
				m.pendingDel = &combo
				m.mode = studioConfirmDelete
				m.notice = ""
			}
			return m, nil
		}
	}

	return m, nil
}

func (m ComboStudioModel) sendSelected() (tea.Model, tea.Cmd) {
	if len(m.list) == 0 {
		m.notice = "Nothing to send yet. Press n to create a combo."
		m.isError = false
		return m, nil
	}
	if m.index >= len(m.list) {
		m.index = len(m.list) - 1
	}
	combo := m.list[m.index]
	if m.busy {
		return m, nil
	}
	m.busy = true
	m.notice = fmt.Sprintf("Sending %q…", combo.Name)
	m.isError = false
	return m, m.sendCmd(combo)
}

func (m ComboStudioModel) sendCmd(combo combos.Combo) tea.Cmd {
	return func() tea.Msg {
		if m.send == nil {
			return comboResultMsg{name: combo.Name, keys: combo.Keys, err: fmt.Errorf("gateway unavailable")}
		}
		err := m.send(combo)
		return comboResultMsg{name: combo.Name, keys: combo.Keys, err: err}
	}
}

func (m ComboStudioModel) openCreate(existing combos.Combo) (tea.Model, tea.Cmd) {
	m.mode = studioCreate
	m.editing = existing.Name
	m.formFocus = 0
	m.nameInput.SetValue(existing.Name)
	m.keysInput.SetValue(existing.Keys)
	m.nameInput.CursorEnd()
	m.keysInput.CursorEnd()
	m.nameInput.Focus()
	m.keysInput.Blur()
	m.notice = ""
	m.isError = false
	return m, textinput.Blink
}

func (m ComboStudioModel) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = studioList
		m.editing = ""
		m.nameInput.Blur()
		m.keysInput.Blur()
		return m, nil

	case "ctrl+s":
		return m.saveCombo(false)

	case "tab":
		m.focusFormField(1)
		return m, textinput.Blink

	case "shift+tab":
		m.focusFormField(-1)
		return m, textinput.Blink

	case "enter":
		switch m.formFocus {
		case 0:
			m.focusFormField(1)
			return m, textinput.Blink
		case 1:
			return m.saveCombo(false)
		case 2:
			return m.saveCombo(false)
		case 3:
			return m.saveCombo(true)
		}
	}

	var cmd tea.Cmd
	switch m.formFocus {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.keysInput, cmd = m.keysInput.Update(msg)
	}
	return m, cmd
}

func (m *ComboStudioModel) focusFormField(delta int) {
	order := []int{0, 1, 2, 3}
	current := 0
	for i, v := range order {
		if v == m.formFocus {
			current = i
			break
		}
	}
	next := (current + delta + len(order)) % len(order)
	m.formFocus = order[next]

	m.nameInput.Blur()
	m.keysInput.Blur()
	if m.formFocus == 0 {
		m.nameInput.Focus()
	} else if m.formFocus == 1 {
		m.keysInput.Focus()
	}
}

func (m ComboStudioModel) saveCombo(andSend bool) (tea.Model, tea.Cmd) {
	name, err := combos.ValidateName(m.nameInput.Value())
	if err != nil {
		m.notice = "✖ " + err.Error()
		m.isError = true
		m.formFocus = 0
		m.nameInput.Focus()
		m.keysInput.Blur()
		return m, textinput.Blink
	}
	keys := strings.TrimSpace(m.keysInput.Value())
	if err := combos.Validate(keys); err != nil {
		m.notice = fmt.Sprintf("✖ %v", err)
		m.isError = true
		m.formFocus = 1
		m.keysInput.Focus()
		m.nameInput.Blur()
		return m, textinput.Blink
	}

	saved := combos.Combo{Name: name, Keys: keys}
	list, err := m.store.Add(saved)
	if err != nil {
		m.notice = fmt.Sprintf("✖ Could not save combo: %v", err)
		m.isError = true
		return m, nil
	}

	// Renaming an edited combo should move it, not leave the old name behind.
	if m.editing != "" && !strings.EqualFold(m.editing, name) {
		if remaining, deleted, derr := m.store.Delete(m.editing); derr == nil && deleted {
			list = remaining
		}
	}
	m.editing = ""
	m.list = list
	m.mode = studioList
	m.focus = 0
	m.nameInput.Blur()
	m.keysInput.Blur()

	for i := range m.list {
		if strings.EqualFold(m.list[i].Name, name) {
			m.index = i
			break
		}
	}
	m.notice = fmt.Sprintf("✔ Saved %q  ·  %s", name, keys)
	m.isError = false

	if andSend {
		return m.sendSelected()
	}
	return m, nil
}

func (m ComboStudioModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.pendingDel == nil {
			m.mode = studioList
			return m, nil
		}
		name := m.pendingDel.Name
		list, deleted, err := m.store.Delete(name)
		if err != nil {
			m.notice = fmt.Sprintf("✖ Delete failed: %v", err)
			m.isError = true
		} else if deleted {
			m.list = list
			if m.index >= len(m.list) {
				m.index = max(0, len(m.list)-1)
			}
			m.notice = fmt.Sprintf("✔ Deleted %q", name)
			m.isError = false
		}
		m.pendingDel = nil
		m.mode = studioList
		return m, nil

	case "n", "N", "esc", "q":
		m.pendingDel = nil
		m.mode = studioList
		return m, nil
	}
	return m, nil
}

func (m ComboStudioModel) View() string {
	if m.quitting {
		return SuccessStyle.Render("\n👋 Combo studio closed. Your combos are saved.\n")
	}

	var sb strings.Builder
	sb.WriteString(m.renderStudioHeader())
	sb.WriteString("\n")

	switch m.mode {
	case studioCreate:
		sb.WriteString(m.renderCreateForm())
	case studioConfirmDelete:
		sb.WriteString(m.renderDeleteConfirm())
	default:
		sb.WriteString(m.renderList())
	}

	if m.notice != "" {
		sb.WriteString("\n")
		if m.isError {
			sb.WriteString(ErrorStyle.Render(m.notice))
		} else {
			sb.WriteString(NoticeStyle.Render(m.notice))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(m.renderStudioFooter())
	return sb.String()
}

func (m ComboStudioModel) renderStudioHeader() string {
	title := TitleStyle.Render("🎹 COMBO STUDIO")
	windows := BadgeWindows.Render()
	count := SubtitleStyle.Render(fmt.Sprintf("%d saved", len(m.list)))
	return fmt.Sprintf("%s  %s  %s\n%s\n", title, windows, count,
		SubtitleStyle.Render("Named Windows keyboard + mouse shortcuts, replayed over Bluetooth HID"))
}

func (m ComboStudioModel) renderList() string {
	var sb strings.Builder

	if len(m.list) == 0 {
		empty := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPurple).
			Padding(1, 3).
			Background(ColorBgCard).
			Render(
				TitleStyle.Render("No combos yet") + "\n\n" +
					comboTextStyle().Render("Press [n] to create your first combo.") + "\n" +
					SubtitleStyle.Render("Create one, name it, then hit the SEND button."))
		sb.WriteString(empty)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(m.renderComboTable())
		sb.WriteString("\n")
	}

	sb.WriteString(m.renderButtons())
	return sb.String()
}

func (m ComboStudioModel) renderComboTable() string {
	header := lipgloss.NewStyle().Foreground(ColorFgMuted).Render(
		fmt.Sprintf("   %-3s %-28s %s", "#", "NAME", "WINDOWS KEYS"))

	var rows []string
	rows = append(rows, header)
	for i, c := range m.list {
		name := c.Name
		if runes := []rune(name); len(runes) > 28 {
			name = string(runes[:27]) + "…"
		}
		keys := c.Keys
		if runes := []rune(keys); len(runes) > 38 {
			keys = string(runes[:37]) + "…"
		}
		line := fmt.Sprintf(" %2d  %-28s %s", i+1, name, keys)
		if i == m.index {
			prefix := "▸"
			if m.focus == 0 {
				rows = append(rows, MenuSelectedStyle.Render(prefix+line))
			} else {
				rows = append(rows, lipgloss.NewStyle().
					Foreground(ColorCyan).PaddingLeft(1).Render(prefix+line))
			}
		} else {
			rows = append(rows, MenuItemStyle.Render(" "+line))
		}
	}
	return strings.Join(rows, "\n")
}

func (m ComboStudioModel) renderButtons() string {
	labels := []string{"✈ SEND", "+ NEW", "🗑 DELETE"}
	buttons := make([]string, 0, len(labels))
	for i, label := range labels {
		style := lipgloss.NewStyle().
			Foreground(ColorFgLight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorFgMuted).
			Padding(0, 2)
		if m.focus == i+1 {
			style = style.
				Bold(true).
				Foreground(ColorBgDark).
				Background(ColorCyan).
				BorderForeground(ColorCyan)
		}
		buttons = append(buttons, style.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, buttons[0], "  ", buttons[1], "  ", buttons[2])
}

func (m ComboStudioModel) renderCreateForm() string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(9)
	nameLabel := labelStyle.Render("Name")
	keysLabel := labelStyle.Render("Keys")

	nameField := m.nameInput.View()
	if m.formFocus != 0 {
		nameField = SubtitleStyle.Render(m.nameInput.Value())
	}
	keysField := m.keysInput.View()
	if m.formFocus != 1 {
		keysField = SubtitleStyle.Render(m.keysInput.Value())
	}

	save := m.formButton("SAVE", 2)
	sendSave := m.formButton("SAVE & SEND", 3)

	form := fmt.Sprintf(
		"%s\n%s  %s\n%s  %s\n\n%s",
		TitleStyle.Render("CREATE / EDIT COMBO"),
		nameLabel, nameField,
		keysLabel, keysField,
		lipgloss.JoinHorizontal(lipgloss.Center, save, "  ", sendSave),
	)

	reference := SubtitleStyle.Render(
		"Windows keyboard: win ctrl alt shift · a–z 0–9 · f1–f12 · enter esc tab space\n" +
			"backspace del insert home end pgup pgdn arrows · printscreen capslock numlock menu\n" +
			"Mouse: click-left click-right click-middle double-click scroll-up scroll-down\n" +
			"Chain steps with ->{seconds}:  win+r ->{0.6} click-left")

	return CardStyle.Width(78).Render(form) + "\n" + reference
}

func (m ComboStudioModel) formButton(label string, focus int) string {
	style := lipgloss.NewStyle().
		Foreground(ColorFgLight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorFgMuted).
		Padding(0, 2)
	if m.formFocus == focus {
		style = style.Bold(true).
			Foreground(ColorBgDark).
			Background(ColorCyan).
			BorderForeground(ColorCyan)
	}
	return style.Render(label)
}

func (m ComboStudioModel) renderDeleteConfirm() string {
	name := "this combo"
	keys := ""
	if m.pendingDel != nil {
		name = fmt.Sprintf("%q", m.pendingDel.Name)
		keys = m.pendingDel.Keys
	}

	body := fmt.Sprintf(
		"%s\n\n%s\n%s\n\n%s   %s",
		ErrorStyle.Render("DELETE COMBO"),
		fmt.Sprintf("Delete %s?", name),
		SubtitleStyle.Render(keys),
		lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("[ Y ] Yes, delete"),
		lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("[ N ] Keep it"),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorRed).
		Padding(1, 3).
		Background(ColorBgCard).
		Width(64).
		Render(body)
}

func (m ComboStudioModel) renderStudioFooter() string {
	if m.mode == studioCreate {
		return HelpStyle.Render("tab next field · enter save (last button sends) · ctrl+s save · esc back")
	}
	if m.mode == studioConfirmDelete {
		return HelpStyle.Render("y confirm · n or esc cancel")
	}
	return HelpStyle.Render("↑↓ select · enter send · tab SEND / NEW / DELETE · s send · n new · e edit · delete remove · esc back")
}

func comboTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ColorFgLight).Bold(true)
}
