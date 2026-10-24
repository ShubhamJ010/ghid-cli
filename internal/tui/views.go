package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/combos"
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
	case StateDeviceRegisterPicker:
		sb.WriteString(m.renderRegisterPicker())
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

	tagline := SubtitleStyle.Render("Android Bluetooth HID Automation Gateway")
	return fmt.Sprintf("%s %s\n%s\n", title, badge, tagline)
}

func (m Model) cardWidth() int {
	return calculateCardWidth(m.width)
}

func (m Model) renderInit() string {
	content := fmt.Sprintf(
		"%s Establishing link with Android Bluetooth Gateway...\n\n%s",
		m.spinner.View(),
		SubtitleStyle.Render("Checking USB/Wi-Fi connection & negotiating SDP profile..."),
	)
	return CardStyle.Width(m.cardWidth()).Render(content)
}

func (m Model) renderTelemetryCard() string {
	devText := "None (waiting for adb link)"
	if m.device != nil {
		devText = fmt.Sprintf("%s (%s)", m.phoneDisplayName(), m.device.Serial)
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

	cardWidth := m.cardWidth()
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

	cardWidth := m.cardWidth()

	// Dynamically calculate title column width so multi-word names never wrap
	maxTitleWidth := 14
	for _, item := range m.menuItems {
		w := lipgloss.Width(item.Title)
		if w > maxTitleWidth {
			maxTitleWidth = w
		}
	}
	if maxTitleWidth > 24 {
		maxTitleWidth = 24
	}

	for i, item := range m.menuItems {
		var line string
		keyStr := KeyBadgeStyle.Render(fmt.Sprintf("[%s]", item.Key))
		iconStr := padRight(item.Icon, 2)
		titlePadded := padRight(truncate(item.Title, maxTitleWidth), maxTitleWidth)
		titleStr := lipgloss.NewStyle().Bold(true).Render(titlePadded)

		// Space for description: cardWidth minus prefix, key, icon, title, separator, margins
		usedWidth := 2 + 1 + lipgloss.Width(keyStr) + 1 + lipgloss.Width(iconStr) + 1 + maxTitleWidth + 3 + 4
		availDesc := cardWidth - usedWidth
		if availDesc < 15 {
			availDesc = 15
		}
		descStr := SubtitleStyle.Render(truncate(item.Description, availDesc))

		content := fmt.Sprintf("%s %s %s - %s", keyStr, iconStr, titleStr, descStr)
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
		return fmt.Sprintf("\n%s\n\n%s", noticeBox, HelpStyle.Render("↑/↓: Navigate • [Enter]: Execute • [1-9/d/0]: Quick key • [q]: Exit"))
	}
	return HelpStyle.Render("↑/↓: Navigate • [Enter]: Execute • [1-9/d/0]: Quick key • [q]: Exit")
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
	sb.WriteString(HelpStyle.Render("[1] / [2]: Choose platform • [Esc]: Cancel"))

	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
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
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(m.phoneDisplayName()),
			lipgloss.NewStyle().Bold(true).Render("Connect"),
		)
		docWidth := m.cardWidth() - 6
		if docWidth < 60 {
			docWidth = 60
		}
		sb.WriteString(DocBoxStyle.Width(docWidth).Render(guideContent))
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
				"4. Pair Device: Click %s ➔ %s. If 2 entries show, pick the one\n"+
				"   with ⌨️ Keyboard icon (%s) [NOT audio/phone icon]. Confirm PIN.\n"+
				"5. Ready: Windows binds the device under 'Input (Mouse, keyboard & pen)'.",
			NoticeStyle.Render("✦ CRITICAL WINDOWS CO-EXISTENCE RULE:"),
			KeyBadgeStyle.Render("✦ STEP-BY-STEP PAIRING:"),
			lipgloss.NewStyle().Bold(true).Render("Settings"),
			lipgloss.NewStyle().Bold(true).Render("Bluetooth & devices"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("Advanced"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("Remove device"),
			lipgloss.NewStyle().Bold(true).Render("Add device"),
			lipgloss.NewStyle().Bold(true).Render("Bluetooth"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(m.phoneDisplayName()),
		)
		docWidth := m.cardWidth() - 6
		if docWidth < 60 {
			docWidth = 60
		}
		sb.WriteString(DocBoxStyle.Width(docWidth).Render(guideContent))
	}

	sb.WriteString("\n\n")
	sb.WriteString(HelpStyle.Render("[Enter / Space]: Next step • [Esc]: Cancel"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
}

func (m Model) renderRegisterPicker() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("📱 Select Target Device from Phone (Step 3 of 3)"))
	sb.WriteString("\n\n")

	if m.pickerLoading {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorYellow).Render("⟳ Querying phone's Bluetooth paired devices via ADB..."))
		sb.WriteString("\n\n")
		sb.WriteString(HelpStyle.Render("[Esc]: Back"))
		return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
	}

	sb.WriteString(SubtitleStyle.Render("Select a machine paired with your phone, or enter a new MAC manually:"))
	sb.WriteString("\n\n")

	cursorPrefix := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ ")

	if len(m.pickerDevices) == 0 {
		sb.WriteString(SubtitleStyle.Render("  (No paired Bluetooth devices detected on phone)"))
		sb.WriteString("\n\n")
	} else {
		for i, dev := range m.pickerDevices {
			cursor := "  "
			if i == m.pickerIndex {
				cursor = cursorPrefix
			}

			icon := "💻"
			nameLower := strings.ToLower(dev.Name)
			if strings.Contains(nameLower, "mac") || strings.Contains(nameLower, "apple") || strings.Contains(nameLower, "air") {
				icon = "🍏"
			} else if strings.Contains(nameLower, "windows") || strings.Contains(nameLower, "thinkpad") || strings.Contains(nameLower, "lp-") || strings.Contains(nameLower, "pc") {
				icon = "🪟"
			} else if strings.Contains(nameLower, "bud") || strings.Contains(nameLower, "headphone") || strings.Contains(nameLower, "flow") || strings.Contains(nameLower, "audio") {
				icon = "🎧"
			}

			isRegistered := false
			for _, rd := range m.cfg.Devices {
				if strings.EqualFold(rd.Address, dev.Address) {
					isRegistered = true
					break
				}
			}

			regTag := ""
			if isRegistered {
				regTag = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("[REGISTERED]")
			} else {
				regTag = lipgloss.NewStyle().Foreground(ColorCyan).Render("[PAIRED]")
			}

			typeTag := lipgloss.NewStyle().Foreground(ColorFgMuted).Render(fmt.Sprintf("[%s]", dev.Type))

			nameStr := padRight(truncate(dev.Name, 24), 24)
			addrStr := padRight(truncate(dev.Address, 18), 18)

			row := fmt.Sprintf("%s%s %s %s %s %s",
				cursor,
				icon,
				nameStr,
				addrStr,
				typeTag,
				regTag,
			)

			if i == m.pickerIndex {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorFgBright).Render(row))
			} else {
				sb.WriteString(row)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	manualIdx := len(m.pickerDevices)
	manCursor := "  "
	if m.pickerIndex == manualIdx {
		manCursor = cursorPrefix
	}
	manRow := fmt.Sprintf("%s✍️  Manual MAC address entry...", manCursor)
	if m.pickerIndex == manualIdx {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(manRow))
	} else {
		sb.WriteString(SubtitleStyle.Render(manRow))
	}

	sb.WriteString("\n\n")
	sb.WriteString(HelpStyle.Render("[↑/↓]: Navigate • [Enter]: Select & Register • [e]: Edit name • [m]: Manual • [r]: Refresh • [Esc]: Back"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
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

	sb.WriteString(HelpStyle.Render("[Tab / ↓]: Switch field • [Enter]: Save device • [Esc]: Cancel"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
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
			isTarget := strings.EqualFold(dev.Address, m.cfg.TargetAddress)
			isLiveConnected := m.status.Connected && (strings.EqualFold(dev.Address, m.status.Address) || isTarget)
			if isLiveConnected {
				activeBadge = BadgeConnected.Render()
			} else if isTarget {
				activeBadge = BadgeSelected.Render()
			}

			nameStr := lipgloss.NewStyle().Bold(true).Render(padRight(truncate(dev.Name, 20), 20))
			addrStr := lipgloss.NewStyle().Foreground(ColorFgMuted).Render(padRight(truncate(dev.Address, 20), 20))

			rowWidth := m.cardWidth() - 6
			if i == m.devicesIndex {
				prefix := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ ")
				line = MenuSelectedStyle.Width(rowWidth).Render(fmt.Sprintf("%s%s %s %s %s", prefix, icon, nameStr, addrStr, activeBadge))
			} else {
				prefix := "  "
				line = MenuItemStyle.Width(rowWidth).Render(fmt.Sprintf("%s%s %s %s %s", prefix, icon, nameStr, addrStr, activeBadge))
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(HelpStyle.Render("[Enter / s]: Connect • [a]: Add new • [d]: Delete • [u]: Unpair • [Esc]: Back"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
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

	sb.WriteString(HelpStyle.Render("[y]: Confirm unregister • [n / Esc]: Cancel"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
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
		HelpStyle.Render("[Tab / ↓]: Switch field • [Enter]: Save & run • [Esc]: Cancel"),
	)

	return FocusCardStyle.Width(m.cardWidth()).Render(content)
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

	osTag := "[" + combos.OSShortTag("") + "]"
	name := "Combo Sequence"
	desc := ""
	if m.execCombo != nil {
		osTag = "[" + combos.OSShortTag(m.execCombo.OS) + "]"
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
		osTag,
		TitleStyle.Render(name),
		SubtitleStyle.Render(desc),
		lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(stepInfo),
		m.progress.ViewAs(pct),
		remainingSec,
		HelpStyle.Render("[Esc] or [q]: Cancel Sequence"),
	)
	return FocusCardStyle.Width(m.cardWidth()).Render(content)
}

func (m Model) renderComboInput() string {
	content := fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		TitleStyle.Render("🎹 Custom Keyboard Chord & Gesture Sequence"),
		m.textInput.View(),
		SubtitleStyle.Render("Examples: super+l  •  ctrl+alt+del -> esc -> $USERNAME  •  click-left ->{0.5} win+d"),
		HelpStyle.Render("[Enter]: Transmit • Delays: -> (1.0s) or ->{1.5} • [Esc]: Cancel"),
	)
	return FocusCardStyle.Width(m.cardWidth()).Render(content)
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
		HelpStyle.Render("[Tab]: Toggle Type/Raw • [Enter]: Transmit • [Esc]: Cancel"),
	)
	return FocusCardStyle.Width(m.cardWidth()).Render(content)
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
	sb.WriteString(HelpStyle.Render("[Enter / Esc]: Return to menu"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
}

func (m Model) renderRecoveryView() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("🔧 Connection Recovery Options"))
	sb.WriteString("\n\n")
	sb.WriteString(HelpStyle.Render("[Enter / Esc]: Return to menu"))
	return FocusCardStyle.Width(m.cardWidth()).Render(sb.String())
}
