package tui

import (
	"fmt"
	"strings"

	"github.com/ShubhamJ010/ghid-cli/internal/payloads"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type payloadStudioMode int

const (
	payloadList payloadStudioMode = iota
	payloadCreate
	payloadConfirmDelete
	payloadPromptSecure
)

// PayloadSendFunc dispatches a payload to the device.
type PayloadSendFunc func(payload payloads.Payload) error

// PayloadStudioModel allows viewing, creating, editing, and deleting payloads
// ({USERNAME}, {PASSWORD}, and any custom text or secrets).
type PayloadStudioModel struct {
	store             *payloads.Store
	sendFn            PayloadSendFunc
	list              []payloads.Payload
	index             int
	mode              payloadStudioMode
	formFocus         int // 0 = name, 1 = value, 2 = type toggle, 3 = secure toggle, 4 = save
	nameInput         textinput.Model
	valueInput        textinput.Model
	payloadType       string // "type" or "raw"
	isSecure          bool
	editing           string // non-empty if editing existing payload
	notice            string
	isError           bool
	pendingDel        *payloads.Payload
	promptPayload     *payloads.Payload
	promptSecureInput textinput.Model
	width             int
	height            int
	quitting          bool
}

// NewPayloadStudioModel initializes the Payload Studio.
func NewPayloadStudioModel(store *payloads.Store, sendFn PayloadSendFunc) PayloadStudioModel {
	name := textinput.New()
	name.Placeholder = "e.g. USERNAME, PIN, API_KEY"
	name.CharLimit = 32
	name.Width = 36

	val := textinput.New()
	val.Placeholder = "Payload text value or raw hex"
	val.CharLimit = 256
	val.Width = 36

	pInput := textinput.New()
	pInput.EchoMode = textinput.EchoPassword
	pInput.EchoCharacter = '•'
	pInput.Width = 36

	var list []payloads.Payload
	var loadErr error
	if store != nil {
		list, loadErr = store.List()
	}

	m := PayloadStudioModel{
		store:             store,
		sendFn:            sendFn,
		list:              list,
		nameInput:         name,
		valueInput:        val,
		promptSecureInput: pInput,
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
		case payloadPromptSecure:
			return m.updatePromptSecure(msg)
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
		if m.index > 0 {
			m.index--
		} else if len(m.list) > 0 {
			m.index = len(m.list) - 1
		}
		m.notice = ""
		return m, nil

	case "down", "j":
		if m.index < len(m.list)-1 {
			m.index++
		} else {
			m.index = 0
		}
		m.notice = ""
		return m, nil

	case "n", "a":
		return m.openCreate(payloads.Payload{})

	case "e":
		if len(m.list) > 0 && m.index < len(m.list) {
			return m.openCreate(m.list[m.index])
		}
		return m, nil

	case "enter", "s":
		// Send payload! Matches Combo Studio behavior
		if len(m.list) > 0 && m.index < len(m.list) {
			p := m.list[m.index]
			if p.Secure || strings.EqualFold(p.Name, "password") {
				return m.openPromptSecure(p)
			}
			return m.dispatchSend(p)
		}
		return m, nil

	case "d", "x", "delete":
		return m.startDelete()
	}

	return m, nil
}

func (m PayloadStudioModel) openPromptSecure(p payloads.Payload) (tea.Model, tea.Cmd) {
	m.mode = payloadPromptSecure
	m.promptPayload = &p
	m.promptSecureInput.SetValue("")
	m.promptSecureInput.Placeholder = fmt.Sprintf("Enter value for {%s}", strings.ToUpper(p.Name))
	m.promptSecureInput.Focus()
	m.notice = ""
	m.isError = false
	return m, textinput.Blink
}

func (m PayloadStudioModel) updatePromptSecure(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = payloadList
		m.promptPayload = nil
		m.notice = "Send cancelled."
		m.isError = false
		return m, nil

	case "enter":
		if m.promptPayload == nil {
			m.mode = payloadList
			return m, nil
		}
		entered := m.promptSecureInput.Value()
		p := *m.promptPayload

		// Save entered value to payload store / keychain on-the-go
		if m.store != nil {
			saved, err := m.store.Set(p.Name, entered, true, p.EffectiveType())
			if err != nil {
				m.notice = fmt.Sprintf("Failed to save secret: %v", err)
				m.isError = true
				m.mode = payloadList
				return m, nil
			}
			p = saved
			m.list, _ = m.store.List()
		}

		m.mode = payloadList
		m.promptPayload = nil
		return m.dispatchSend(p)

	default:
		var cmd tea.Cmd
		m.promptSecureInput, cmd = m.promptSecureInput.Update(msg)
		return m, cmd
	}
}

func (m PayloadStudioModel) dispatchSend(p payloads.Payload) (tea.Model, tea.Cmd) {
	if m.sendFn == nil {
		m.notice = fmt.Sprintf("✔ Prepared {%s} (send function not configured)", strings.ToUpper(p.Name))
		m.isError = false
		return m, nil
	}
	err := m.sendFn(p)
	if err != nil {
		m.notice = fmt.Sprintf("✖ Send {%s} failed: %v", strings.ToUpper(p.Name), err)
		m.isError = true
	} else {
		m.notice = fmt.Sprintf("✔ Sent payload {%s}", strings.ToUpper(p.Name))
		m.isError = false
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
		m.notice = ""
		return m, nil

	case "tab", "down":
		m.formFocus = (m.formFocus + 1) % 5
		m.syncFieldFocus()
		return m, textinput.Blink

	case "shift+tab", "up":
		m.formFocus = (m.formFocus + 4) % 5
		m.syncFieldFocus()
		return m, textinput.Blink

	case "enter":
		if m.formFocus == 4 || (m.formFocus == 1 && m.valueInput.Value() != "") {
			return m.savePayload()
		}
		if m.formFocus == 2 {
			if m.payloadType == "type" {
				m.payloadType = "raw"
			} else {
				m.payloadType = "type"
			}
			return m, nil
		}
		if m.formFocus == 3 {
			m.isSecure = !m.isSecure
			m.setEchoMode()
			return m, nil
		}
		m.formFocus = (m.formFocus + 1) % 5
		m.syncFieldFocus()
		return m, textinput.Blink

	case " ":
		if m.formFocus == 2 {
			if m.payloadType == "type" {
				m.payloadType = "raw"
			} else {
				m.payloadType = "type"
			}
			return m, nil
		}
		if m.formFocus == 3 {
			m.isSecure = !m.isSecure
			m.setEchoMode()
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.formFocus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else if m.formFocus == 1 {
		m.valueInput, cmd = m.valueInput.Update(msg)
	}
	return m, cmd
}

func (m *PayloadStudioModel) syncFieldFocus() {
	if m.formFocus == 0 {
		m.nameInput.Focus()
		m.valueInput.Blur()
	} else if m.formFocus == 1 {
		m.nameInput.Blur()
		m.valueInput.Focus()
	} else {
		m.nameInput.Blur()
		m.valueInput.Blur()
	}
}

func (m PayloadStudioModel) savePayload() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.notice = "Payload name cannot be empty"
		m.isError = true
		return m, nil
	}

	if _, err := payloads.ValidateName(name); err != nil {
		m.notice = fmt.Sprintf("Invalid name: %v", err)
		m.isError = true
		return m, nil
	}

	val := m.valueInput.Value()
	pType := m.payloadType
	if pType == "" {
		pType = "type"
	}

	_, err := m.store.Set(name, val, m.isSecure, pType)
	if err != nil {
		m.notice = fmt.Sprintf("Failed to save payload: %v", err)
		m.isError = true
		return m, nil
	}

	m.list, _ = m.store.List()
	for i, p := range m.list {
		if strings.EqualFold(p.Name, name) {
			m.index = i
			break
		}
	}

	m.mode = payloadList
	m.notice = fmt.Sprintf("✔ Saved payload {%s}", strings.ToUpper(name))
	m.isError = false
	return m, nil
}

func (m PayloadStudioModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y", "enter":
		if m.pendingDel != nil {
			name := m.pendingDel.Name
			_, _, err := m.store.Delete(name)
			if err != nil {
				m.notice = fmt.Sprintf("Delete error: %v", err)
				m.isError = true
			} else {
				m.notice = fmt.Sprintf("✔ Deleted payload {%s}", strings.ToUpper(name))
				m.isError = false
				m.list, _ = m.store.List()
				if m.index >= len(m.list) && m.index > 0 {
					m.index = len(m.list) - 1
				}
			}
		}
		m.mode = payloadList
		m.pendingDel = nil
		return m, nil

	case "n", "esc", "q":
		m.mode = payloadList
		m.pendingDel = nil
		m.notice = ""
		return m, nil
	}

	return m, nil
}

func (m PayloadStudioModel) View() string {
	if m.quitting {
		return ""
	}

	var content string
	switch m.mode {
	case payloadCreate:
		content = m.renderCreateForm()
	case payloadConfirmDelete:
		content = m.renderDeleteConfirm()
	case payloadPromptSecure:
		content = m.renderPromptSecure()
	default:
		content = m.renderList()
	}

	cardWidth := calculateCardWidth(m.width)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPurple).
		Padding(1, 2).
		Width(cardWidth).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m PayloadStudioModel) renderList() string {
	header := m.renderHeader()
	table := m.renderTable()
	footer := m.renderFooter()

	var noticeBlock string
	if m.notice != "" {
		st := SuccessStyle
		if m.isError {
			st = ErrorStyle
		}
		noticeBlock = "\n\n" + st.Render(m.notice)
	}

	return fmt.Sprintf("%s\n\n%s%s\n\n%s", header, table, noticeBlock, footer)
}

func (m PayloadStudioModel) renderHeader() string {
	title := TitleStyle.Render("📋 PAYLOAD STUDIO")
	count := SubtitleStyle.Render(fmt.Sprintf("%d universal payloads configured", len(m.list)))
	return fmt.Sprintf("%s  %s\n%s", title, count, SubtitleStyle.Render(strings.Repeat("─", 68)))
}

func (m PayloadStudioModel) renderTable() string {
	if len(m.list) == 0 {
		return SubtitleStyle.Render("  No payloads found. Press [n] to create your first payload.")
	}

	colNum := 6
	colVar := 22
	colType := 10
	colSec := 18
	valWidth := 20

	headerNum := padRight("  #", colNum)
	headerVar := padRight("TOKEN", colVar)
	headerType := padRight("TYPE", colType)
	headerVal := padRight("VALUE", valWidth)
	headerSec := padRight("SECURITY", colSec)

	headerLine := SubtitleStyle.Render(fmt.Sprintf("  %s %s %s %s %s", headerNum, headerVar, headerType, headerVal, headerSec))
	divLine := SubtitleStyle.Render(fmt.Sprintf("  %s", strings.Repeat("─", 74)))

	rows := []string{headerLine, divLine}

	for i, s := range m.list {
		tokenName := fmt.Sprintf("{%s}", strings.ToUpper(s.Name))
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
			rows = append(rows, lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan).
				Background(lipgloss.Color("#313244")).
				Render(line))
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

func (m PayloadStudioModel) renderFooter() string {
	switch m.mode {
	case payloadCreate:
		return HelpStyle.Render("[Tab / ↓]: Next field • [Enter]: Save payload • [Esc]: Cancel")
	case payloadConfirmDelete:
		return HelpStyle.Render("[Y / Enter]: Delete • [N / Esc]: Cancel")
	case payloadPromptSecure:
		return HelpStyle.Render("[Enter]: Save & Send • [Esc]: Cancel")
	default:
		return HelpStyle.Render("↑/↓: Navigate • [Enter / s]: Send • [n]: New • [e]: Edit • [d]: Delete • [Esc/q]: Back")
	}
}

func (m PayloadStudioModel) renderPromptSecure() string {
	tokenName := "PAYLOAD"
	if m.promptPayload != nil {
		tokenName = "{" + strings.ToUpper(m.promptPayload.Name) + "}"
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("🔐 SECURE PAYLOAD INPUT")
	sub := SubtitleStyle.Render(fmt.Sprintf("Enter on-the-go value for %s:", tokenName))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(0, 1).
		Width(40).
		Render(m.promptSecureInput.View())

	help := HelpStyle.Render("[Enter]: Save & Send • [Esc]: Cancel")

	card := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		sub,
		box,
		"",
		help,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(1, 3).
		Background(ColorBgCard).
		Render(card)
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
		Render(body)
}

// RunPayloadStudio runs the Payload Studio standalone in a Bubble Tea program.
func RunPayloadStudio(store *payloads.Store, sendFn PayloadSendFunc) error {
	m := NewPayloadStudioModel(store, sendFn)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
