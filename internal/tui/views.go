package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.quitting {
		return lipgloss.NewStyle().Foreground(ColorCyan).Render("\n👋 GhostHID gateway released. Goodbye!\n")
	}

	if m.state == StateComboStudio && m.comboStudio != nil {
		return m.comboStudio.View()
	}

	if m.state == StateSnippetStudio && m.snippetStudio != nil {
		return m.snippetStudio.View()
	}

	var sb strings.Builder

	// Top Banner
	sb.WriteString(m.renderBanner())
	sb.WriteString("\n")

	switch m.state {
	case StateInit:
		sb.WriteString(m.renderInit())
	case StateMenu:
		sb.WriteString(m.renderTelemetryCard())
		sb.WriteString("\n")
		sb.WriteString(m.renderMenu())
		sb.WriteString("\n")
		sb.WriteString(m.renderFooter())
	case StateLoginPrompt:
		sb.WriteString(m.renderLoginPrompt())
	case StateComboExecuting, StateLoginExecuting, StateSleepCountdown:
		sb.WriteString(m.renderComboCountdown())
	case StateQuickSend:
		sb.WriteString(m.renderQuickSend())
	case StateComboInput:
		sb.WriteString(m.renderComboInput())
	case StateDoctorView:
		sb.WriteString(m.renderDoctorView())
	case StateDeviceRecovery:
		sb.WriteString(m.renderRecoveryView())
	case StateDevicesList:
		sb.WriteString(m.renderDevicesList())
	case StateDeviceRegisterPlatform:
		sb.WriteString(m.renderRegisterPlatform())
	case StateDeviceRegisterGuide:
		sb.WriteString(m.renderRegisterGuide())
	case StateDeviceRegisterInput:
		sb.WriteString(m.renderRegisterInput())
	case StateDeviceDeleteConfirm:
		sb.WriteString(m.renderDeleteConfirm())
	}

	return sb.String()
}

func (m Model) renderBanner() string {
	title := TitleStyle.Render("👻 GHOST·HID")
	badge := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBgDark).
		Background(ColorMagenta).
		Padding(0, 1).
		Render("v1.0.0")

	tagline := SubtitleStyle.Render("Headless Android Bluetooth HID Automation Gateway")
	return fmt.Sprintf("%s %s\n%s\n", title, badge, tagline)
}

func (m Model) renderInit() string {
	return CardStyle.Render(fmt.Sprintf(
		"%s Establishing link with Android Bluetooth Gateway...\n\n%s",
		m.spinner.View(),
		SubtitleStyle.Render("Checking USB/Wi-Fi connection & negotiating SDP profile..."),
	))
}

func (m Model) renderTelemetryCard() string {
	devText := "None (waiting for adb link)"
	if m.device != nil {
		devText = fmt.Sprintf("%s (%s)", m.deviceModel, m.device.Serial)
	}

	var sdpBadge string
	if m.status.Registered {
		sdpBadge = BadgeReady.Render()
	} else {
		sdpBadge = BadgeWait.Render()
	}

	var connBadge string
	if m.status.Connected {
		connBadge = BadgeConnected.Render()
	} else {
		connBadge = BadgeDisconnected.Render()
	}

	targetText := "None (press [d] to pair or select)"
	selDev := m.cfg.GetSelectedDevice()
	if selDev != nil {
		targetText = fmt.Sprintf("%s [%s]", selDev.Name, selDev.Address)
	} else if m.status.Target != "" && m.status.Target != "none" {
		targetText = fmt.Sprintf("%s [%s]", m.status.Target, m.status.Address)
	}

	cardWidth := 78
	if m.width > 0 {
		cardWidth = m.width - 4
		if cardWidth < 70 {
			cardWidth = 70
		} else if cardWidth > 96 {
			cardWidth = 96
		}
	}
	innerWidth := cardWidth - 6

	rightColWidth := 24
	leftColWidth := innerWidth - rightColWidth - 2
	if leftColWidth < 28 {
		leftColWidth = 28
	}

	labelStyle := lipgloss.NewStyle().Foreground(ColorFgMuted)
	valStyle := lipgloss.NewStyle().Foreground(ColorFgLight).Bold(true)

	labelPhone := labelStyle.Render("📱 Phone:  ")
	labelTarget := labelStyle.Render("💻 Target: ")
	labelSDP := labelStyle.Render("SDP:  ")
	labelLink := labelStyle.Render("Link: ")

	valPhone := valStyle.Render(truncate(devText, leftColWidth-11))
	valTarget := valStyle.Render(truncate(targetText, leftColWidth-11))

	left1 := padRight(labelPhone+valPhone, leftColWidth)
	left2 := padRight(labelTarget+valTarget, leftColWidth)

	right1 := labelSDP + sdpBadge
	right2 := labelLink + connBadge

	cardContent := fmt.Sprintf("%s  %s\n%s  %s", left1, right1, left2, right2)

	return CardStyle.Width(cardWidth).Render(cardContent)
}

func (m Model) renderMenu() string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("ACTIONS"))
	sb.WriteString("\n")

	cardWidth := 78
	if m.width > 0 {
		cardWidth = m.width - 4
		if cardWidth < 70 {
			cardWidth = 70
		} else if cardWidth > 96 {
			cardWidth = 96
		}
	}

	for i, item := range m.menuItems {
		var line string
		keyStr := KeyBadgeStyle.Render(fmt.Sprintf("[%s]", item.Key))
		titleStr := lipgloss.NewStyle().Bold(true).Width(12).Render(item.Title)
		descStr := SubtitleStyle.Render(item.Description)

		content := fmt.Sprintf("%s %s %s - %s", keyStr, item.Icon, titleStr, descStr)
		if i == m.menuIndex {
			prefix := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ ")
			line = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan).
				Background(lipgloss.Color("#313244")).
				PaddingLeft(1).
				Width(cardWidth).
				Render(prefix + content)
		} else {
			prefix := "  "
			line = lipgloss.NewStyle().
				Foreground(ColorFgLight).
				PaddingLeft(1).
				Width(cardWidth).
				Render(prefix + content)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderFooter() string {
	if m.notice != "" {
		var noticeBox string
		if m.isErrorNotice {
			noticeBox = ErrorStyle.Render(m.notice)
		} else {
			noticeBox = SuccessStyle.Render(m.notice)
		}
		return fmt.Sprintf("\n%s\n\n%s", noticeBox, HelpStyle.Render("↑/↓ or key: Select action • [Enter]: Execute • [q]: Exit"))
	}
	return HelpStyle.Render("↑/↓ or key: Select action • [Enter]: Execute • [q]: Exit")
}

func (m Model) renderRegisterPlatform() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("💻 Target Host Operating System (Step 1 of 3)"))
	sb.WriteString("\n\n")

	infoText := SubtitleStyle.Render(
		"GhostHID advertises your smartphone as a hardware-level Bluetooth HID Peripheral keyboard\n" +
			"(using standard L2CAP PSM channels). Windows and macOS require slightly different pairing flows.",
	)
	sb.WriteString(infoText)
	sb.WriteString("\n\n")

	opt1 := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("[1] 🪟 Windows PC / Laptop")
	opt1Desc := SubtitleStyle.Render("    Standard corporate / personal Windows 10/11 machines.")

	opt2 := lipgloss.NewStyle().Bold(true).Foreground(ColorMagenta).Render("[2] 🍏 Apple Mac (macOS)")
	opt2Desc := SubtitleStyle.Render("    MacBook, Mac Studio, Mac Mini running macOS Sonoma / Sequoia.")

	sb.WriteString(fmt.Sprintf("%s\n%s\n\n%s\n%s\n\n", opt1, opt1Desc, opt2, opt2Desc))
	sb.WriteString(HelpStyle.Render("Press [1] or [2] to choose platform • [Esc]: Cancel"))

	return FocusCardStyle.Width(66).Render(sb.String())
}

func (m Model) renderRegisterGuide() string {
	var sb strings.Builder

	if m.regPlatform == "macos" {
		sb.WriteString(TitleStyle.Render("🍏 macOS HID Peripheral Pairing Instructions (Step 2 of 3)"))
		sb.WriteString("\n\n")

		guideContent := fmt.Sprintf(
			"%s\n"+
				"GhostHID operates as a driverless Bluetooth HID keyboard peripheral directly to Apple Silicon / Intel Macs.\n\n"+
				"%s\n"+
				"1. Phone Status: Ensure GhostHID service is active (SDP profile: READY).\n"+
				"2. Mac Settings: Open %s ➔ %s.\n"+
				"3. Nearby Devices: Look for your phone (%s) under 'Nearby Devices'.\n"+
				"4. Pair & Confirm: Click %s and verify the 6-digit numeric passkey on both screens.\n"+
				"5. Keyboard Setup: macOS immediately maps standard ANSI 101/104-key HID keyboard events.",
			NoticeStyle.Render("✦ HOW IT WORKS:"),
			KeyBadgeStyle.Render("✦ STEP-BY-STEP PAIRING:"),
			lipgloss.NewStyle().Bold(true).Render("System Settings"),
			lipgloss.NewStyle().Bold(true).Render("Bluetooth"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(m.deviceModel),
			lipgloss.NewStyle().Bold(true).Render("Connect"),
		)
		sb.WriteString(DocBoxStyle.Width(64).Render(guideContent))
	} else {
		sb.WriteString(TitleStyle.Render("🪟 Windows HID Peripheral Pairing Instructions (Step 2 of 3)"))
		sb.WriteString("\n\n")

		guideContent := fmt.Sprintf(
			"%s\n"+
				"Windows caches Bluetooth devices aggressively. To ensure Windows installs the\n"+
				"hardware Human Interface Device driver (mshidkmdf.sys) rather than treating your phone as an audio device:\n\n"+
				"%s\n"+
				"1. Advanced Discovery: In Windows %s ➔ %s, scroll down to\n"+
				"   'Bluetooth devices discovery' and change dropdown from Default to %s.\n"+
				"2. Clear Stale Pairing: If your phone is listed under 'Other devices', click %s.\n"+
				"3. Ensure SDP Ready: GhostHID foreground service must be running.\n"+
				"4. Pair Device: Click %s ➔ %s, select your phone (%s),\n"+
				"   and confirm the 6-digit PIN on both your phone and laptop.\n"+
				"5. Ready: Windows binds the device under 'Input (Mouse, keyboard & pen)'.",
			NoticeStyle.Render("✦ CRITICAL WINDOWS CO-EXISTENCE RULE:"),
			KeyBadgeStyle.Render("✦ STEP-BY-STEP PAIRING:"),
			lipgloss.NewStyle().Bold(true).Render("Settings"),
			lipgloss.NewStyle().Bold(true).Render("Bluetooth & devices"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("Advanced"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("Remove device"),
			lipgloss.NewStyle().Bold(true).Render("Add device"),
			lipgloss.NewStyle().Bold(true).Render("Bluetooth"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(m.deviceModel),
		)
		sb.WriteString(DocBoxStyle.Width(64).Render(guideContent))
	}

	sb.WriteString("\n\n")
	sb.WriteString(HelpStyle.Render("Press [Enter] or [Space] when paired ➔ Next step • [Esc]: Cancel"))
	return FocusCardStyle.Width(68).Render(sb.String())
}

func (m Model) renderRegisterInput() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("📝 Register Device Details (Step 3 of 3)"))
	sb.WriteString("\n\n")

	nameLabel := "Target Name:"
	addrLabel := "Bluetooth MAC Address:"

	if m.regActiveIn == 0 {
		nameLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Target Name:")
		addrLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Bluetooth MAC Address:")
	} else {
		nameLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Target Name:")
		addrLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Bluetooth MAC Address:")
	}

	sb.WriteString(nameLabel + "\n")
	sb.WriteString(m.devNameInput.View() + "\n\n")
	sb.WriteString(addrLabel + "\n")
	sb.WriteString(m.devAddrInput.View() + "\n\n")

	sb.WriteString(HelpStyle.Render("[Tab/↓]: Switch Field • [Enter]: Save Device • [Esc]: Cancel"))
	return FocusCardStyle.Width(64).Render(sb.String())
}

func (m Model) renderDevicesList() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("💻 Registered Target Devices"))
	sb.WriteString("\n")
	sb.WriteString(SubtitleStyle.Render("Select a machine to immediately connect GhostHID via Bluetooth"))
	sb.WriteString("\n\n")

	if len(m.cfg.Devices) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(ColorYellow).Render("No devices registered yet.\nPress [a] to add your Windows PC or Mac.")
		sb.WriteString(emptyMsg)
		sb.WriteString("\n\n")
	} else {
		for i, dev := range m.cfg.Devices {
			var line string
			var icon string
			if dev.Platform == "macos" {
				icon = "🍏"
			} else {
				icon = "🪟"
			}

			activeBadge := ""
			if dev.Address == m.cfg.TargetAddress {
				activeBadge = BadgeConnected.Render()
			}

			nameStr := lipgloss.NewStyle().Bold(true).Width(20).Render(dev.Name)
			addrStr := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(20).Render(dev.Address)

			if i == m.devicesIndex {
				prefix := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ ")
				line = MenuSelectedStyle.Width(62).Render(fmt.Sprintf("%s%s %s %s %s", prefix, icon, nameStr, addrStr, activeBadge))
			} else {
				prefix := "  "
				line = MenuItemStyle.Render(fmt.Sprintf("%s%s %s %s %s", prefix, icon, nameStr, addrStr, activeBadge))
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(HelpStyle.Render("[Enter/s]: Connect • [a]: Add New • [d]: Delete • [u]: Unpair • [Esc]: Back"))
	return CardStyle.Width(68).Render(sb.String())
}

func (m Model) renderDeleteConfirm() string {
	var sb strings.Builder
	sb.WriteString(ErrorStyle.Render("⚠ Confirm Target Removal"))
	sb.WriteString("\n\n")

	devName := "Unknown"
	devAddr := ""
	if m.deviceToDelete != nil {
		devName = m.deviceToDelete.Name
		devAddr = m.deviceToDelete.Address
	}

	warning := fmt.Sprintf(
		"Are you sure you want to unregister the following device?\n\n"+
			"  Device Name: %s\n"+
			"  MAC Address: %s\n\n"+
			"This will delete it from your registered devices and unpair the Bluetooth bonding on your phone.",
		lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(devName),
		lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(devAddr),
	)
	sb.WriteString(warning)
	sb.WriteString("\n\n")

	sb.WriteString(HelpStyle.Render("Press [y] to Confirm Unregister • Press [n / Esc] to Cancel"))
	return FocusCardStyle.Width(64).Render(sb.String())
}

func (m Model) renderLoginPrompt() string {
	userLabel := "Username ($USERNAME):"
	passLabel := "Password ($PASSWORD):"
	if m.loginActiveIn == 0 {
		userLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Username ($USERNAME):")
		passLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Password ($PASSWORD):")
	} else {
		userLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Username ($USERNAME):")
		passLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Password ($PASSWORD):")
	}

	title := "Set Snippet Values"
	if m.pendingCombo != nil {
		title = fmt.Sprintf("Run %q · Missing Snippets", m.pendingCombo.Name)
	}

	content := fmt.Sprintf(
		"🔑 %s\n\n%s\n%s\n\n%s\n%s\n\n%s",
		TitleStyle.Render(title),
		userLabel,
		m.userInput.View(),
		passLabel,
		m.passInput.View(),
		SubtitleStyle.Render("[Tab/↓]: Switch Field • [Enter]: Save & Run • [Esc]: Cancel"),
	)

	return FocusCardStyle.Width(56).Render(content)
}

func (m Model) renderComboCountdown() string {
	pct := 0.0
	if m.execTotalDuration > 0 {
		elapsed := m.execTotalDuration - m.execRemaining
		pct = float64(elapsed) / float64(m.execTotalDuration)
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
	}

	emoji := "⚡"
	name := "Combo Sequence"
	desc := ""
	if m.execCombo != nil {
		if m.execCombo.Emoji != "" {
			emoji = m.execCombo.Emoji
		}
		if m.execCombo.Name != "" {
			name = m.execCombo.Name
		}
		if m.execCombo.Description != "" {
			desc = m.execCombo.Description
		} else {
			desc = m.execCombo.Keys
		}
	}

	remainingSec := m.execRemaining.Seconds()
	if remainingSec < 0 {
		remainingSec = 0
	}

	stepNum := m.execStepIdx + 1
	totalSteps := len(m.execSteps)
	if totalSteps == 0 {
		totalSteps = 1
	}
	if stepNum > totalSteps {
		stepNum = totalSteps
	}

	stepInfo := fmt.Sprintf("Step %d of %d: %s", stepNum, totalSteps, m.execStepDesc)

	content := fmt.Sprintf(
		"%s %s\n\n%s\n\n%s\n\n%s\nRemaining: %.1fs\n\n%s",
		emoji,
		TitleStyle.Render(name),
		SubtitleStyle.Render(desc),
		lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(stepInfo),
		m.progress.ViewAs(pct),
		remainingSec,
		HelpStyle.Render("[Esc] or [q]: Cancel Sequence"),
	)
	return CardStyle.Width(52).Render(content)
}

func (m Model) renderComboInput() string {
	content := fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		TitleStyle.Render("🎹 Custom Keyboard Chord & Gesture Sequence"),
		m.textInput.View(),
		SubtitleStyle.Render("Examples: super+l  •  ctrl+alt+del -> esc -> $USERNAME  •  click-left ->{0.5} win+d"),
		SubtitleStyle.Render("[Enter]: Transmit • Shortcuts auto-record • Delays: -> (1.0s) or ->{1.5} • [Esc]: Cancel"),
	)
	return FocusCardStyle.Width(72).Render(content)
}

func (m Model) renderQuickSend() string {
	modeType := lipgloss.NewStyle().Foreground(ColorFgMuted).Render("[ ] Type Text")
	modeRaw := lipgloss.NewStyle().Foreground(ColorFgMuted).Render("[ ] Raw 16-Hex HID")

	var desc string
	if m.quickSendRaw {
		modeRaw = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("[✔] Raw 16-Hex HID")
		desc = "Transmit raw 8-byte HID keyboard report (16 hex chars e.g. 0000040000000000)"
	} else {
		modeType = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("[✔] Type Text")
		desc = "Type text directly into active target window via virtual HID keyboard"
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Left, modeType, "    ", modeRaw)

	content := fmt.Sprintf(
		"%s\n\nMode: %s\n\n%s\n\n%s\n\n%s",
		TitleStyle.Render("⚡ QUICK SEND · Instant Keyboard & Raw HID"),
		tabs,
		SubtitleStyle.Render(desc),
		m.textInput.View(),
		SubtitleStyle.Render("[Tab]: Toggle Type/Raw • [Enter]: Transmit • [Esc]: Cancel"),
	)
	return FocusCardStyle.Width(68).Render(content)
}

func (m Model) renderDoctorView() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("🩺 GhostHID Health Diagnostics"))
	sb.WriteString("\n\n")

	for _, check := range m.doctorChecks {
		var icon string
		if check.Pass {
			icon = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("✔")
		} else {
			icon = lipgloss.NewStyle().Foreground(ColorRed).Bold(true).Render("✖")
		}
		sb.WriteString(fmt.Sprintf("%s %-30s %s\n", icon, check.Title, lipgloss.NewStyle().Foreground(ColorFgMuted).Render(check.Info)))
	}

	sb.WriteString("\n")
	sb.WriteString(HelpStyle.Render("Press [Enter] or [Esc] to return to menu"))
	return CardStyle.Width(64).Render(sb.String())
}

func (m Model) renderRecoveryView() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("🔧 Connection Recovery Options"))
	sb.WriteString("\n\n")
	sb.WriteString(HelpStyle.Render("Press [Enter] or [Esc] to return to menu"))
	return CardStyle.Width(64).Render(sb.String())
}
