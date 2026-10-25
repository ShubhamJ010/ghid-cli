package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/snippets"
)

type snipStudioMode int

const (
	snipList snipStudioMode = iota
	snipCreate
	snipConfirmDelete
)

// SnippetStudioModel allows viewing, creating, editing, and deleting snippets
// ($USERNAME, $PASSWORD, and any custom text or secrets).
type SnippetStudioModel struct {
	store      *snippets.Store
	list       []snippets.Snippet
	index      int
	focus      int // 0 = list, 1 = NEW, 2 = EDIT, 3 = DELETE
	mode       snipStudioMode
	formFocus  int // 0 = name, 1 = value, 2 = secure toggle, 3 = save
	nameInput  textinput.Model
	valueInput textinput.Model
	snipType   string // "type" or "raw"
	isSecure   bool
	editing    string // non-empty if editing existing snippet
	notice     string
	isError    bool
	pendingDel *snippets.Snippet
	width      int
	height     int
	quitting   bool
}

// NewSnippetStudioModel initializes the Snippet Studio.
func NewSnippetStudioModel(store *snippets.Store) SnippetStudioModel {
	name := textinput.New()
	name.Placeholder = "e.g. USERNAME, PIN, API_KEY"
	name.CharLimit = 32
	name.Width = 36

	val := textinput.New()
	val.Placeholder = "Snippet text value"
	val.CharLimit = 256
	val.Width = 36

	var list []snippets.Snippet
	var loadErr error
	if store != nil {
		list, loadErr = store.List()
	}

	m := SnippetStudioModel{
		store:      store,
		list:       list,
		nameInput:  name,
		valueInput: val,
	}

	if loadErr != nil {
		m.notice = fmt.Sprintf("Could not load snippets: %v", loadErr)
		m.isError = true
	}
	return m
}

func (m SnippetStudioModel) Init() tea.Cmd {
	return nil
}

func (m SnippetStudioModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		switch m.mode {
		case snipCreate:
			return m.updateCreate(msg)
		case snipConfirmDelete:
			return m.updateConfirmDelete(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m SnippetStudioModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m.openCreate(snippets.Snippet{})

	case "e", "enter":
		if m.focus == 1 {
			return m.openCreate(snippets.Snippet{})
		}
		if m.focus == 2 || m.focus == 0 {
			if len(m.list) > 0 && m.index < len(m.list) {
				return m.openCreate(m.list[m.index])
			}
		}
		if m.focus == 3 {
			return m.startDelete()
		}
		if len(m.list) > 0 && m.index < len(m.list) {
			return m.openCreate(m.list[m.index])
		}
		return m, nil

	case "d", "x", "delete":
		return m.startDelete()

	case "s":
		// Toggle secure mode for selected snippet
		if len(m.list) > 0 && m.index < len(m.list) && m.store != nil {
			target := m.list[m.index]
			newSecure := !target.Secure
			_, _ = m.store.Set(target.Name, target.Value, newSecure)
			m.list, _ = m.store.List()
			status := "plain text"
			if newSecure {
				status = "secure (stored in Keychain)"
			}
			m.notice = fmt.Sprintf("✔ Snippet %q marked as %s", target.Name, status)
			m.isError = false
		}
		return m, nil
	}

	return m, nil
}

func (m SnippetStudioModel) startDelete() (tea.Model, tea.Cmd) {
	if len(m.list) == 0 || m.index >= len(m.list) {
		return m, nil
	}
	snip := m.list[m.index]
	if strings.EqualFold(snip.Name, "username") || strings.EqualFold(snip.Name, "password") || strings.EqualFold(snip.Name, "email") || strings.EqualFold(snip.Name, "sleep_windows") || strings.EqualFold(snip.Name, "play_pause") {
		m.notice = fmt.Sprintf("✖ Default snippet %q cannot be deleted (clear value instead)", snip.Name)
		m.isError = true
		return m, nil
	}
	m.pendingDel = &snip
	m.mode = snipConfirmDelete
	m.notice = ""
	return m, nil
}

func (m SnippetStudioModel) openCreate(existing snippets.Snippet) (tea.Model, tea.Cmd) {
	m.mode = snipCreate
	m.editing = existing.Name
	m.formFocus = 0

	m.nameInput.SetValue(existing.Name)
	if existing.Name != "" {
		m.nameInput.Blur()
		m.formFocus = 1
	} else {
		m.nameInput.Focus()
	}

	m.valueInput.SetValue(existing.Value)
	m.snipType = existing.EffectiveType()
	m.isSecure = existing.Secure
	m.setEchoMode()

	if m.formFocus == 1 {
		m.valueInput.Focus()
	} else {
		m.valueInput.Blur()
	}

	m.notice = ""
	m.isError = false
	return m, textinput.Blink
}

func (m *SnippetStudioModel) setEchoMode() {
	if m.isSecure {
		m.valueInput.EchoMode = textinput.EchoPassword
		m.valueInput.EchoCharacter = '•'
	} else {
		m.valueInput.EchoMode = textinput.EchoNormal
	}
}

func (m SnippetStudioModel) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = snipList
		m.editing = ""
		m.nameInput.Blur()
		m.valueInput.Blur()
		return m, nil

	case "ctrl+s":
		return m.saveSnippet()

	case "tab":
		m.focusFormField(1)
		return m, textinput.Blink

	case "shift+tab":
		m.focusFormField(-1)
		return m, textinput.Blink

	case " ":
		if m.formFocus == 2 {
			if m.snipType == "raw" {
				m.snipType = "type"
			} else {
				m.snipType = "raw"
			}
			return m, nil
		}
		if m.formFocus == 3 {
			m.isSecure = !m.isSecure
			m.setEchoMode()
			return m, nil
		}

	case "enter":
		switch m.formFocus {
		case 0:
			m.focusFormField(1)
			return m, textinput.Blink
		case 1:
			m.focusFormField(1)
			return m, textinput.Blink
		case 2:
			if m.snipType == "raw" {
				m.snipType = "type"
			} else {
				m.snipType = "raw"
			}
			return m, nil
		case 3:
			m.isSecure = !m.isSecure
			m.setEchoMode()
			return m, nil
		case 4:
			return m.saveSnippet()
		}
	}

	var cmd tea.Cmd
	switch m.formFocus {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.valueInput, cmd = m.valueInput.Update(msg)
	}
	return m, cmd
}

func (m *SnippetStudioModel) focusFormField(delta int) {
	// If editing default snippet, name is locked
	order := []int{0, 1, 2, 3, 4}
	if m.editing != "" {
		order = []int{1, 2, 3, 4}
	}

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
	m.valueInput.Blur()
	if m.formFocus == 0 {
		m.nameInput.Focus()
	} else if m.formFocus == 1 {
		m.valueInput.Focus()
	}
}

func (m SnippetStudioModel) saveSnippet() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if m.editing != "" {
		name = m.editing
	}
	cleanName, err := snippets.ValidateName(name)
	if err != nil {
		m.notice = "✖ " + err.Error()
		m.isError = true
		m.formFocus = 0
		m.nameInput.Focus()
		return m, textinput.Blink
	}

	val := m.valueInput.Value()
	if m.store != nil {
		_, err = m.store.Set(cleanName, val, m.isSecure, m.snipType)
		if err != nil {
			m.notice = fmt.Sprintf("✖ Failed to save snippet: %v", err)
			m.isError = true
			return m, nil
		}
		m.list, _ = m.store.List()
	}

	m.mode = snipList
	m.editing = ""
	m.notice = fmt.Sprintf("✔ Snippet %q saved", cleanName)
	m.isError = false
	for i, s := range m.list {
		if strings.EqualFold(s.Name, cleanName) {
			m.index = i
			break
		}
	}
	return m, nil
}

func (m SnippetStudioModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if m.pendingDel != nil {
			name := m.pendingDel.Name
			if m.store != nil {
				_, _, err := m.store.Delete(name)
				if err != nil {
					m.notice = fmt.Sprintf("✖ Failed to delete snippet: %v", err)
					m.isError = true
				} else {
					m.list, _ = m.store.List()
					if m.index >= len(m.list) && len(m.list) > 0 {
						m.index = len(m.list) - 1
					}
					m.notice = fmt.Sprintf("✔ Deleted snippet %q", name)
					m.isError = false
				}
			}
			m.pendingDel = nil
			m.mode = snipList
		}
		return m, nil

	case "n", "N", "esc", "q":
		m.pendingDel = nil
		m.mode = snipList
		m.notice = "Deletion cancelled."
		m.isError = false
		return m, nil
	}
	return m, nil
}

func (m SnippetStudioModel) cardWidth() int {
	return calculateCardWidth(m.width)
}

func (m SnippetStudioModel) View() string {
	var sb strings.Builder

	sb.WriteString(TitleStyle.Render("📋 SNIPPET STUDIO · Variables & Text Snippets"))
	sb.WriteString("\n")
	sb.WriteString(SubtitleStyle.Render("Define text and secure variables for combos ($USERNAME, $PASSWORD, $PIN)"))
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
	case snipCreate:
		sb.WriteString(m.renderCreateForm())
	case snipConfirmDelete:
		sb.WriteString(m.renderDeleteConfirm())
	default:
		sb.WriteString(m.renderList())
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.renderStudioFooter())

	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
}

func (m SnippetStudioModel) renderList() string {
	var sb strings.Builder

	if len(m.list) == 0 {
		empty := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPurple).
			Padding(1, 3).
			Background(ColorBgCard).
			Render(
				TitleStyle.Render("No snippets found") + "\n\n" +
					lipgloss.NewStyle().Foreground(ColorFgLight).Bold(true).Render("Press [n] to create your first snippet.") + "\n" +
					SubtitleStyle.Render("Snippets can be referenced in combos as $NAME or typed directly."))
		sb.WriteString(empty)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(m.renderSnippetTable())
		sb.WriteString("\n")
	}

	sb.WriteString(m.renderButtons())
	return sb.String()
}

func (m SnippetStudioModel) renderSnippetTable() string {
	cardWidth := m.cardWidth()
	innerWidth := cardWidth - 6

	colNum := 5
	colVar := 18
	colType := 8
	colSec := 15
	valWidth := innerWidth - colNum - colVar - colType - colSec - 8
	if valWidth < 14 {
		valWidth = 14
	}

	headerPrefix := "  "
	headerNum := padRight("  #", colNum)
	headerVar := padRight("VARIABLE", colVar)
	headerType := padRight("TYPE", colType)
	headerVal := padRight("VALUE", valWidth)
	headerSec := padRight("SECURITY", colSec)
	headerLine := fmt.Sprintf("%s%s %s %s %s %s", headerPrefix, headerNum, headerVar, headerType, headerVal, headerSec)
	header := lipgloss.NewStyle().Foreground(ColorFgMuted).Render(headerLine)

	var rows []string
	rows = append(rows, header)
	for i, s := range m.list {
		varName := "$" + strings.ToUpper(s.Name)
		numCol := padRight(fmt.Sprintf("   %-2d", i+1), colNum)
		varCol := padRight(truncate(varName, colVar), colVar)
		typeCol := padRight(strings.ToUpper(s.EffectiveType()), colType)
		valCol := padRight(truncate(s.MaskedValue(), valWidth), valWidth)

		var secCol string
		if s.Secure {
			secCol = padRight("[KEYCHAIN 🔒]", colSec)
		} else {
			secCol = padRight("[PLAIN]", colSec)
		}

		if i == m.index {
			prefix := "▸ "
			line := fmt.Sprintf("%s%s %s %s %s %s", prefix, numCol, varCol, typeCol, valCol, secCol)
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
			line := fmt.Sprintf("%s%s %s %s %s %s", prefix, numCol, varCol, typeCol, valCol, secCol)
			rows = append(rows, lipgloss.NewStyle().
				Foreground(ColorFgLight).
				Render(line))
		}
	}
	return strings.Join(rows, "\n")
}

func (m SnippetStudioModel) renderButtons() string {
	labels := []string{"+ NEW", "✎ EDIT", "🗑 DELETE"}
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

func (m SnippetStudioModel) renderCreateForm() string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(10)
	nameLabel := labelStyle.Render("Variable")
	valLabel := labelStyle.Render("Value")

	nameField := m.nameInput.View()
	if m.editing != "" {
		nameField = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("$" + strings.ToUpper(m.editing) + " (Name locked)")
	} else if m.formFocus != 0 {
		nameField = SubtitleStyle.Render("$" + strings.ToUpper(m.nameInput.Value()))
	}

	valField := m.valueInput.View()
	if m.formFocus != 1 {
		val := m.valueInput.Value()
		if m.isSecure && val != "" {
			val = "••••••••"
		}
		valField = SubtitleStyle.Render(val)
	}

	typeLabel := "[Type: TEXT]"
	if m.snipType == "raw" {
		typeLabel = "[Type: RAW REPORT]"
	}
	typeStyle := lipgloss.NewStyle().Foreground(ColorCyan)
	if m.formFocus == 2 {
		typeStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorBgDark).Background(ColorCyan).Padding(0, 1)
	}
	typeBtn := typeStyle.Render(typeLabel)

	toggleLabel := "[ ] Plaintext storage"
	if m.isSecure {
		toggleLabel = "[✔] Secure in macOS Keychain"
	}
	toggleStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	if m.formFocus == 3 {
		toggleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorBgDark).Background(ColorCyan).Padding(0, 1)
	}
	toggleBtn := toggleStyle.Render(toggleLabel)

	saveBtnStyle := lipgloss.NewStyle().
		Foreground(ColorFgLight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorFgMuted).
		Padding(0, 2)
	if m.formFocus == 4 {
		saveBtnStyle = saveBtnStyle.
			Bold(true).
			Foreground(ColorBgDark).
			Background(ColorCyan).
			BorderForeground(ColorCyan)
	}
	saveBtn := saveBtnStyle.Render("SAVE SNIPPET")

	title := "CREATE NEW SNIPPET"
	if m.editing != "" {
		title = fmt.Sprintf("EDIT SNIPPET $%s", strings.ToUpper(m.editing))
	}

	form := fmt.Sprintf(
		"%s\n\n%s  %s\n%s  %s\n\nMode:       %s\n\nSecurity:   %s\n\n%s",
		TitleStyle.Render(title),
		nameLabel, nameField,
		valLabel, valField,
		typeBtn,
		toggleBtn,
		saveBtn,
	)

	help := SubtitleStyle.Render("Tab: switch field • Space: toggle Keychain encryption • Enter: save • Esc: cancel")
	return form + "\n\n" + help
}

func (m SnippetStudioModel) renderDeleteConfirm() string {
	name := "this snippet"
	if m.pendingDel != nil {
		name = fmt.Sprintf("$%s", strings.ToUpper(m.pendingDel.Name))
	}

	body := fmt.Sprintf(
		"%s\n\n%s\n\n%s   %s",
		ErrorStyle.Render("DELETE SNIPPET"),
		fmt.Sprintf("Are you sure you want to delete %s?", name),
		lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("[ Y ] Yes, delete"),
		lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("[ N ] Cancel"),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorRed).
		Padding(1, 3).
		Background(ColorBgCard).
		Width(64).
		Render(body)
}

func (m SnippetStudioModel) renderStudioFooter() string {
	if m.mode == snipCreate {
		return HelpStyle.Render("[Tab]: Switch field • [Space]: Toggle Keychain • [Enter / Ctrl+S]: Save • [Esc]: Cancel")
	}
	if m.mode == snipConfirmDelete {
		return HelpStyle.Render("[y]: Confirm delete • [n / Esc]: Cancel")
	}
	return HelpStyle.Render("↑/↓: Navigate • [Enter/e]: Edit • [n]: New • [d]: Delete • [s]: Toggle Keychain • [Tab]: Focus buttons • [Esc/q]: Back")
}
