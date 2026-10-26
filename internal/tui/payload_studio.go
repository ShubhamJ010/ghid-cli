package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ShubhamJ010/ghid-cli/internal/payloads"
)

type payloadStudioMode int

const (
	payloadList payloadStudioMode = iota
	payloadCreate
	payloadConfirmDelete
)

// PayloadStudioModel allows viewing, creating, editing, and deleting payloads
// ({USERNAME}, {PASSWORD}, and any custom text or secrets).
type PayloadStudioModel struct {
	store       *payloads.Store
	list        []payloads.Payload
	index       int
	focus       int // 0 = list, 1 = NEW, 2 = EDIT, 3 = DELETE
	mode        payloadStudioMode
	formFocus   int // 0 = name, 1 = value, 2 = type toggle, 3 = secure toggle, 4 = save
	nameInput   textinput.Model
	valueInput  textinput.Model
	payloadType string // "type" or "raw"
	isSecure    bool
	editing     string // non-empty if editing existing payload
	notice      string
	isError     bool
	pendingDel  *payloads.Payload
	width       int
	height      int
	quitting    bool
}

// NewPayloadStudioModel initializes the Payload Studio.
func NewPayloadStudioModel(store *payloads.Store) PayloadStudioModel {
	name := textinput.New()
	name.Placeholder = "e.g. USERNAME, PIN, API_KEY"
	name.CharLimit = 32
	name.Width = 36

	val := textinput.New()
	val.Placeholder = "Payload text value or raw hex"
	val.CharLimit = 256
	val.Width = 36

	var list []payloads.Payload
	var loadErr error
	if store != nil {
		list, loadErr = store.List()
	}

	m := PayloadStudioModel{
		store:      store,
		list:       list,
		nameInput:  name,
		valueInput: val,
	}

	if loadErr != nil {
		m.notice = fmt.Sprintf("Could not load payloads: %v", loadErr)
		m.isError = true
	}
	return m
}

func (m PayloadStudioModel) Init() tea.Cmd {
	return nil
}

func (m PayloadStudioModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case payloadCreate:
			return m.updateCreate(msg)
		case payloadConfirmDelete:
			return m.updateConfirmDelete(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m PayloadStudioModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m.openCreate(payloads.Payload{})

	case "e", "enter":
		if m.focus == 1 {
			return m.openCreate(payloads.Payload{})
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
		// Toggle secure mode for selected payload
		if len(m.list) > 0 && m.index < len(m.list) && m.store != nil {
			target := m.list[m.index]
			newSecure := !target.Secure
			_, _ = m.store.Set(target.Name, target.Value, newSecure, target.Type)
			m.list, _ = m.store.List()
			status := "plain text"
			if newSecure {
				status = "secure (stored in Keychain)"
			}
			m.notice = fmt.Sprintf("✔ Payload {%s} marked as %s", target.Name, status)
			m.isError = false
		}
		return m, nil
	}

	return m, nil
}

func (m PayloadStudioModel) startDelete() (tea.Model, tea.Cmd) {
	if len(m.list) == 0 || m.index >= len(m.list) {
		return m, nil
	}
	p := m.list[m.index]
	if strings.EqualFold(p.Name, "username") || strings.EqualFold(p.Name, "password") || strings.EqualFold(p.Name, "email") || strings.EqualFold(p.Name, "WIN_SLEEP") || strings.EqualFold(p.Name, "PLAY_PAUSE") {
		m.notice = fmt.Sprintf("✖ Default payload {%s} cannot be deleted (clear value instead)", p.Name)
		m.isError = true
		return m, nil
	}
	m.pendingDel = &p
	m.mode = payloadConfirmDelete
	m.notice = ""
	return m, nil
}

func (m PayloadStudioModel) openCreate(existing payloads.Payload) (tea.Model, tea.Cmd) {
	m.mode = payloadCreate
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
	m.payloadType = existing.EffectiveType()
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

func (m *PayloadStudioModel) setEchoMode() {
	if m.isSecure {
		m.valueInput.EchoMode = textinput.EchoPassword
		m.valueInput.EchoCharacter = '•'
	} else {
		m.valueInput.EchoMode = textinput.EchoNormal
	}
}

func (m PayloadStudioModel) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = payloadList
		m.editing = ""
		m.nameInput.Blur()
		m.valueInput.Blur()
		return m, nil

	case "ctrl+s":
		return m.savePayload()

	case "tab":
		m.focusFormField(1)
		return m, textinput.Blink

	case "shift+tab":
		m.focusFormField(-1)
		return m, textinput.Blink

	case " ":
		if m.formFocus == 2 {
			if m.payloadType == "raw" {
				m.payloadType = "type"
			} else {
				m.payloadType = "raw"
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
			if m.payloadType == "raw" {
				m.payloadType = "type"
			} else {
				m.payloadType = "raw"
			}
			return m, nil
		case 3:
			m.isSecure = !m.isSecure
			m.setEchoMode()
			return m, nil
		case 4:
			return m.savePayload()
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

func (m *PayloadStudioModel) focusFormField(delta int) {
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

func (m PayloadStudioModel) savePayload() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if m.editing != "" {
		name = m.editing
	}
	cleanName, err := payloads.ValidateName(name)
	if err != nil {
		m.notice = "✖ " + err.Error()
		m.isError = true
		m.formFocus = 0
		m.nameInput.Focus()
		return m, textinput.Blink
	}

	val := m.valueInput.Value()
	if m.store != nil {
		_, err = m.store.Set(cleanName, val, m.isSecure, m.payloadType)
		if err != nil {
			m.notice = fmt.Sprintf("✖ Failed to save payload: %v", err)
			m.isError = true
			return m, nil
		}
		m.list, _ = m.store.List()
	}

	m.mode = payloadList
	m.editing = ""
	m.notice = fmt.Sprintf("✔ Payload {%s} saved", cleanName)
	m.isError = false

	for i, s := range m.list {
		if strings.EqualFold(s.Name, cleanName) {
			m.index = i
			break
		}
	}
	return m, nil
}

func (m PayloadStudioModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if m.pendingDel != nil {
			name := m.pendingDel.Name
			if m.store != nil {
				_, _, err := m.store.Delete(name)
				if err != nil {
					m.notice = fmt.Sprintf("✖ Failed to delete payload: %v", err)
					m.isError = true
				} else {
					m.list, _ = m.store.List()
					if m.index >= len(m.list) && len(m.list) > 0 {
						m.index = len(m.list) - 1
					}
					m.notice = fmt.Sprintf("✔ Deleted payload {%s}", name)
					m.isError = false
				}
			}
			m.pendingDel = nil
			m.mode = payloadList
		}
		return m, nil

	case "n", "N", "esc", "q":
		m.pendingDel = nil
		m.mode = payloadList
		m.notice = "Deletion cancelled."
		m.isError = false
		return m, nil
	}
	return m, nil
}

func (m PayloadStudioModel) cardWidth() int {
	return calculateCardWidth(m.width)
}

func (m PayloadStudioModel) View() string {
	var sb strings.Builder

	sb.WriteString(TitleStyle.Render("📋 PAYLOAD STUDIO · Named Payloads & Secrets"))
	sb.WriteString("\n")
	sb.WriteString(SubtitleStyle.Render("Define text and secure payloads for combos ({USERNAME}, {PASSWORD}, {WIN_SLEEP})"))
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
	case payloadCreate:
		sb.WriteString(m.renderCreateForm())
	case payloadConfirmDelete:
		sb.WriteString(m.renderDeleteConfirm())
	default:
		sb.WriteString(m.renderList())
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.renderStudioFooter())

	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
}

func (m PayloadStudioModel) renderList() string {
	var sb strings.Builder

	if len(m.list) == 0 {
		empty := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPurple).
			Padding(1, 3).
			Background(ColorBgCard).
			Render(
				TitleStyle.Render("No payloads found") + "\n\n" +
					lipgloss.NewStyle().Foreground(ColorFgLight).Bold(true).Render("Press [n] to create your first payload.") + "\n" +
					SubtitleStyle.Render("Payloads can be referenced in combo sequences as {NAME}."))
		sb.WriteString(empty)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(m.renderPayloadTable())
		sb.WriteString("\n")
	}

	sb.WriteString(m.renderButtons())
	return sb.String()
}

func (m PayloadStudioModel) renderPayloadTable() string {
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
	headerVar := padRight("PAYLOAD", colVar)
	headerType := padRight("TYPE", colType)
	headerVal := padRight("VALUE", valWidth)
	headerSec := padRight("SECURITY", colSec)
	headerLine := fmt.Sprintf("%s%s %s %s %s %s", headerPrefix, headerNum, headerVar, headerType, headerVal, headerSec)
	header := lipgloss.NewStyle().Foreground(ColorFgMuted).Render(headerLine)

	var rows []string
	rows = append(rows, header)
	for i, s := range m.list {
		tokenName := "{" + s.Name + "}"
		numCol := padRight(fmt.Sprintf("   %-2d", i+1), colNum)
		varCol := padRight(truncate(tokenName, colVar), colVar)
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

func (m PayloadStudioModel) renderButtons() string {
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

func (m PayloadStudioModel) renderCreateForm() string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(10)
	nameLabel := labelStyle.Render("Payload")
	valLabel := labelStyle.Render("Value")

	nameField := m.nameInput.View()
	if m.editing != "" {
		nameField = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("{" + m.editing + "} (Name locked)")
	} else if m.formFocus != 0 {
		nameField = SubtitleStyle.Render("{" + m.nameInput.Value() + "}")
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
	if m.payloadType == "raw" {
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
	saveBtn := saveBtnStyle.Render("SAVE PAYLOAD")

	title := "CREATE NEW PAYLOAD"
	if m.editing != "" {
		title = fmt.Sprintf("EDIT PAYLOAD {%s}", m.editing)
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

	help := SubtitleStyle.Render("Tab: switch field • Space: toggle option • Enter: save • Esc: cancel")
	return form + "\n\n" + help
}

func (m PayloadStudioModel) renderDeleteConfirm() string {
	name := "this payload"
	if m.pendingDel != nil {
		name = fmt.Sprintf("{%s}", m.pendingDel.Name)
	}

	body := fmt.Sprintf(
		"%s\n\n%s\n\n%s   %s",
		ErrorStyle.Render("DELETE PAYLOAD"),
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

func (m PayloadStudioModel) renderStudioFooter() string {
	if m.mode == payloadCreate {
		return HelpStyle.Render("[Tab]: Switch field • [Space]: Toggle option • [Enter / Ctrl+S]: Save • [Esc]: Cancel")
	}
	if m.mode == payloadConfirmDelete {
		return HelpStyle.Render("[y]: Confirm delete • [n / Esc]: Cancel")
	}
	return HelpStyle.Render("↑/↓: Navigate • [Enter/e]: Edit • [n]: New • [d]: Delete • [s]: Toggle Keychain • [Tab]: Focus buttons • [Esc/q]: Back")
}
