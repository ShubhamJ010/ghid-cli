package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/mattn/go-runewidth"
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
	formFocus  int // 0 = name, 1 = emoji, 2 = desc, 3 = keys, 4 = save, 5 = save & send
	nameInput  textinput.Model
	emojiInput textinput.Model
	descInput  textinput.Model
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
	name.Width = 46

	emoji := textinput.New()
	emoji.Placeholder = "e.g. 🔑, ⚡, 🚀, 🔒, 🌙 (optional)"
	emoji.CharLimit = 16
	emoji.Width = 46

	desc := textinput.New()
	desc.Placeholder = "e.g. Windows unlock & credential injection (optional)"
	desc.CharLimit = 80
	desc.Width = 46

	keys := textinput.New()
	keys.Placeholder = "win+s ->{0.6} type:rustdesk ->{0.3} enter"
	keys.CharLimit = 256
	keys.Width = 46

	var list []combos.Combo
	var err error
	if store != nil {
		list, err = store.Load()
	}

	m := ComboStudioModel{
		store:      store,
		send:       send,
		list:       list,
		nameInput:  name,
		emojiInput: emoji,
		descInput:  desc,
		keysInput:  keys,
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

	case "f":
		if len(m.list) > 0 && m.index < len(m.list) && m.store != nil {
			combo := m.list[m.index]
			list, ok, err := m.store.ToggleFavorite(combo.Name)
			if err != nil {
				m.notice = fmt.Sprintf("✖ Could not toggle favorite: %v", err)
				m.isError = true
			} else if ok {
				m.list = list
				status := "pinned to favorites"
				if !m.list[m.index].Favorite {
					status = "removed from favorites"
				}
				m.notice = fmt.Sprintf("✔ Combo %q %s", combo.Name, status)
				m.isError = false
			}
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
	m.emojiInput.SetValue(existing.Emoji)
	m.descInput.SetValue(existing.Description)
	m.keysInput.SetValue(existing.Keys)
	m.nameInput.CursorEnd()
	m.emojiInput.CursorEnd()
	m.descInput.CursorEnd()
	m.keysInput.CursorEnd()
	m.nameInput.Focus()
	m.emojiInput.Blur()
	m.descInput.Blur()
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
		m.emojiInput.Blur()
		m.descInput.Blur()
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
		case 0, 1, 2:
			m.focusFormField(1)
			return m, textinput.Blink
		case 3:
			return m.saveCombo(false)
		case 4:
			return m.saveCombo(false)
		case 5:
			return m.saveCombo(true)
		}
	}

	var cmd tea.Cmd
	switch m.formFocus {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.emojiInput, cmd = m.emojiInput.Update(msg)
	case 2:
		m.descInput, cmd = m.descInput.Update(msg)
	case 3:
		m.keysInput, cmd = m.keysInput.Update(msg)
	}
	return m, cmd
}

func (m *ComboStudioModel) focusFormField(delta int) {
	order := []int{0, 1, 2, 3, 4, 5}
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
	m.emojiInput.Blur()
	m.descInput.Blur()
	m.keysInput.Blur()
	switch m.formFocus {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.emojiInput.Focus()
	case 2:
		m.descInput.Focus()
	case 3:
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
		m.emojiInput.Blur()
		m.descInput.Blur()
		m.keysInput.Blur()
		return m, textinput.Blink
	}
	keys := strings.TrimSpace(m.keysInput.Value())
	if err := combos.Validate(keys); err != nil {
		m.notice = fmt.Sprintf("✖ %v", err)
		m.isError = true
		m.formFocus = 3
		m.keysInput.Focus()
		m.nameInput.Blur()
		m.emojiInput.Blur()
		m.descInput.Blur()
		return m, textinput.Blink
	}

	emoji := strings.TrimSpace(m.emojiInput.Value())
	desc := strings.TrimSpace(m.descInput.Value())

	isFav := false
	if m.editing != "" && m.store != nil {
		if old, ok := m.store.Find(m.editing); ok {
			isFav = old.Favorite
		}
	}

	saved := combos.Combo{
		Name:        name,
		Keys:        keys,
		Emoji:       emoji,
		Description: desc,
		Favorite:    isFav,
	}
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

	m.list = list
	m.mode = studioList
	m.editing = ""
	m.nameInput.Blur()
	m.emojiInput.Blur()
	m.descInput.Blur()
	m.keysInput.Blur()
	m.notice = fmt.Sprintf("✔ Saved combo %q", name)
	m.isError = false

	for i, c := range m.list {
		if strings.EqualFold(c.Name, name) {
			m.index = i
			break
		}
	}

	if andSend {
		return m.sendSelected()
	}
	return m, nil
}

func (m ComboStudioModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if m.pendingDel != nil {
			name := m.pendingDel.Name
			list, deleted, err := m.store.Delete(name)
			m.pendingDel = nil
			m.mode = studioList
			if err != nil {
				m.notice = fmt.Sprintf("✖ Could not delete %q: %v", name, err)
				m.isError = true
			} else if deleted {
				m.list = list
				if m.index >= len(m.list) && len(m.list) > 0 {
					m.index = len(m.list) - 1
				}
				m.notice = fmt.Sprintf("✔ Deleted combo %q", name)
				m.isError = false
			}
		}
		return m, nil

	case "n", "N", "esc", "q":
		m.pendingDel = nil
		m.mode = studioList
		m.notice = "Deletion cancelled."
		m.isError = false
		return m, nil
	}
	return m, nil
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncate(s string, maxVisualWidth int) string {
	if lipgloss.Width(s) <= maxVisualWidth {
		return s
	}
	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)
	target := maxVisualWidth - ellipsisWidth
	if target <= 0 {
		return ellipsis
	}
	var res strings.Builder
	curWidth := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if curWidth+rw > target {
			break
		}
		res.WriteRune(r)
		curWidth += rw
	}
	res.WriteString(ellipsis)
	return res.String()
}

func (m ComboStudioModel) View() string {
	cardWidth := 96
	if m.width > 0 {
		cardWidth = m.width - 4
		if cardWidth < 78 {
			cardWidth = 78
		} else if cardWidth > 110 {
			cardWidth = 110
		}
	}
	innerWidth := cardWidth - 6

	var sb strings.Builder

	sb.WriteString(TitleStyle.Render("🎹 COMBO STUDIO · Windows keyboard & mouse shortcuts"))
	sb.WriteString("\n")
	sb.WriteString(SubtitleStyle.Render("Create sequences using Windows keys (win+r, esc), mouse clicks, text typing and $VARIABLES"))
	sb.WriteString("\n\n")

	if m.notice != "" {
		style := lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
		if m.isError {
			style = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
		}
		sb.WriteString(style.Render(m.notice))
		sb.WriteString("\n\n")
	}

	switch m.mode {
	case studioCreate:
		sb.WriteString(m.renderCreateForm())
	case studioConfirmDelete:
		sb.WriteString(m.renderDeleteConfirm())
	default:
		sb.WriteString(m.renderList(innerWidth))
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.renderStudioFooter())

	return FocusCardStyle.Width(cardWidth).Render(sb.String())
}

func (m ComboStudioModel) renderList(innerWidth int) string {
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
		sb.WriteString(m.renderComboTable(innerWidth))
		sb.WriteString("\n")
	}

	sb.WriteString(m.renderButtons())
	return sb.String()
}

func (m ComboStudioModel) renderComboTable(innerWidth int) string {
	colFavNum := 6
	prefixWidth := 2
	spaces := 3 // spaces between prefix+colFavNum, name, keys, desc
	avail := innerWidth - prefixWidth - colFavNum - spaces
	if avail < 40 {
		avail = 40
	}

	nameWidth := 22
	keysWidth := 28
	if avail < 70 {
		nameWidth = 18
		keysWidth = 22
	}
	descWidth := avail - nameWidth - keysWidth
	if descWidth < 12 {
		nameWidth = 16
		keysWidth = 18
		descWidth = avail - nameWidth - keysWidth
		if descWidth < 10 {
			descWidth = 10
		}
	}

	headerPrefix := "  "
	headerFavNum := padRight("  #", colFavNum)
	headerName := padRight("NAME", nameWidth)
	headerKeys := padRight("KEYS", keysWidth)
	headerDesc := padRight("DESCRIPTION", descWidth)
	headerLine := fmt.Sprintf("%s%s %s %s %s", headerPrefix, headerFavNum, headerName, headerKeys, headerDesc)
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

		emojiStr := ""
		if c.Emoji != "" {
			emojiStr = c.Emoji + " "
		}
		fullName := emojiStr + c.Name
		nameCol := padRight(truncate(fullName, nameWidth), nameWidth)
		keysCol := padRight(truncate(c.Keys, keysWidth), keysWidth)
		descCol := padRight(truncate(c.Description, descWidth), descWidth)

		if i == m.index {
			prefix := "▸ "
			line := fmt.Sprintf("%s%s %s %s %s", prefix, favNumCol, nameCol, keysCol, descCol)
			if m.focus == 0 {
				rows = append(rows, lipgloss.NewStyle().
					Bold(true).
					Foreground(ColorCyan).
					Background(lipgloss.Color("#313244")).
					Render(line))
			} else {
				rows = append(rows, lipgloss.NewStyle().
					Foreground(ColorCyan).
					Render(line))
			}
		} else {
			prefix := "  "
			line := fmt.Sprintf("%s%s %s %s %s", prefix, favNumCol, nameCol, keysCol, descCol)
			rows = append(rows, lipgloss.NewStyle().
				Foreground(ColorFgLight).
				Render(line))
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
			Padding(0, 2).
			Background(ColorBgCard)
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
	labelStyle := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(13)
	nameLabel := labelStyle.Render("Name")
	emojiLabel := labelStyle.Render("Emoji")
	descLabel := labelStyle.Render("Description")
	keysLabel := labelStyle.Render("Keys")

	nameField := m.nameInput.View()
	if m.formFocus != 0 {
		nameVal := m.nameInput.Value()
		if nameVal == "" {
			nameVal = "(empty)"
		}
		nameField = SubtitleStyle.Render(nameVal)
	}
	emojiField := m.emojiInput.View()
	if m.formFocus != 1 {
		emojiVal := m.emojiInput.Value()
		if emojiVal == "" {
			emojiVal = "(none)"
		}
		emojiField = SubtitleStyle.Render(emojiVal)
	}
	descField := m.descInput.View()
	if m.formFocus != 2 {
		descVal := m.descInput.Value()
		if descVal == "" {
			descVal = "(none)"
		}
		descField = SubtitleStyle.Render(descVal)
	}
	keysField := m.keysInput.View()
	if m.formFocus != 3 {
		keysVal := m.keysInput.Value()
		if keysVal == "" {
			keysVal = "(empty)"
		}
		keysField = SubtitleStyle.Render(keysVal)
	}

	save := m.formButton("SAVE", 4)
	sendSave := m.formButton("SAVE & SEND", 5)

	form := fmt.Sprintf(
		"%s\n\n%s  %s\n%s  %s\n%s  %s\n%s  %s\n\n%s",
		TitleStyle.Render("CREATE / EDIT COMBO"),
		nameLabel, nameField,
		emojiLabel, emojiField,
		descLabel, descField,
		keysLabel, keysField,
		lipgloss.JoinHorizontal(lipgloss.Center, save, "  ", sendSave),
	)

	help := SubtitleStyle.Render(
		"Supported: win, ctrl, alt, shift, letters/numbers, esc, tab, enter, space, arrows.\n" +
			"Delays: ->{0.8} or ->. Mouse: click-left, scroll-up. Text: type:rustdesk. Variables: $USERNAME, $PASSWORD")
	return form + "\n\n" + help
}

func (m ComboStudioModel) formButton(label string, focusIdx int) string {
	style := lipgloss.NewStyle().
		Foreground(ColorFgLight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorFgMuted).
		Padding(0, 2).
		Background(ColorBgCard)
	if m.formFocus == focusIdx {
		style = style.
			Bold(true).
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
		return HelpStyle.Render("tab next field · shift+tab prev · enter next/save · ctrl+s save · esc cancel")
	}
	if m.mode == studioConfirmDelete {
		return HelpStyle.Render("y confirm · n or esc cancel")
	}
	return HelpStyle.Render("↑↓ select · enter send · f favorite (❤️ pin) · tab SEND/NEW/DEL · s send · n new · e edit · del remove · esc back")
}

func comboTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ColorFgLight).Bold(true)
}
