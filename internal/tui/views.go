package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/keychain"
)

func (m Model) View() string {
	if m.quitting {
		return lipgloss.NewStyle().Foreground(ColorCyan).Render("\n👋 GhostHID gateway released. Goodbye!\n")
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
	case StateLoginExecuting:
		sb.WriteString(m.renderLoginExecuting())
	case StateSleepCountdown:
		sb.WriteString(m.renderSleepCountdown())
	case StateTextInput:
		sb.WriteString(m.renderTextInput("⌨️ Virtual Keyboard: Type Text"))
	case StateComboInput:
		sb.WriteString(m.renderComboInput())
	case StateRawInput:
		sb.WriteString(m.renderTextInput("⚙️ Raw 8-Byte HID Report (16 Hex Characters)"))
	case StateCredsMenu:
		sb.WriteString(m.renderCredsMenu())
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
	devText := "None"
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

	targetText := "None"
	selDev := m.cfg.GetSelectedDevice()
	if selDev != nil {
		targetText = fmt.Sprintf("%s [%s]", selDev.Name, selDev.Address)
	} else if m.status.Target != "" && m.status.Target != "none" {
		targetText = fmt.Sprintf("%s [%s]", m.status.Target, m.status.Address)
	}

	credText := "None (Enter credentials at login)"
	if m.cfg.DefaultUser != "" {
		if keychain.IsAvailable() {
			if _, err := keychain.GetPassword(m.cfg.DefaultUser); err == nil {
				credText = fmt.Sprintf("%s (Keychain 🔒)", m.cfg.DefaultUser)
			} else {
				credText = m.cfg.DefaultUser
			}
		} else {
			credText = m.cfg.DefaultUser
		}
	}

	labelStyle := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(16)
	valStyle := lipgloss.NewStyle().Foreground(ColorFgLight).Bold(true)

	cardContent := fmt.Sprintf(
		"%s %s\n%s %s\n%s %s  %s\n%s %s",
		labelStyle.Render("Phone Device:"), valStyle.Render(devText),
		labelStyle.Render("Gateway SDP:"), sdpBadge,
		labelStyle.Render("Target Laptop:"), valStyle.Render(targetText), connBadge,
		labelStyle.Render("Credentials:"), valStyle.Render(credText),
	)

	return CardStyle.Width(64).Render(cardContent)
}

func (m Model) renderMenu() string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("ACTIONS"))
	sb.WriteString("\n")

	for i, item := range m.menuItems {
		var line string
		keyStr := KeyBadgeStyle.Render(fmt.Sprintf("[%s]", item.Key))
		titleStr := lipgloss.NewStyle().Bold(true).Width(12).Render(item.Title)
		descStr := SubtitleStyle.Render(item.Description)

		if i == m.menuIndex {
			prefix := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ ")
			line = MenuSelectedStyle.Width(62).Render(fmt.Sprintf("%s%s %s %s - %s", prefix, keyStr, item.Icon, titleStr, descStr))
		} else {
			prefix := "  "
			line = MenuItemStyle.Render(fmt.Sprintf("%s%s %s %s - %s", prefix, keyStr, item.Icon, titleStr, descStr))
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
		return fmt.Sprintf("\n%s\n\n%s", noticeBox, HelpStyle.Render("↑/↓ or [1-0/d]: Select action • [Enter]: Execute • [q]: Exit"))
	}
	return HelpStyle.Render("↑/↓ or [1-0/d]: Select action • [Enter]: Execute • [q]: Exit")
}

func (m Model) renderDevicesList() string {
	var sb strings.Builder
	title := TitleStyle.Render("💻 Registered Bluetooth Target Devices")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if len(m.cfg.Devices) == 0 {
		sb.WriteString(SubtitleStyle.Render("  No devices registered yet.\n\n"))
	} else {
		for i, dev := range m.cfg.Devices {
			var osBadge string
			if dev.Platform == "macos" {
				osBadge = BadgeMacOS.Render()
			} else {
				osBadge = BadgeWindows.Render()
			}

			nameStr := lipgloss.NewStyle().Bold(true).Width(20).Render(dev.Name)
			addrStr := lipgloss.NewStyle().Foreground(ColorFgMuted).Width(20).Render(dev.Address)

			var activeBadge string
			if dev.Selected {
				activeBadge = " " + BadgeActiveTarget.Render()
			}

			rowContent := fmt.Sprintf("%s  %s %s%s", osBadge, nameStr, addrStr, activeBadge)

			if i == m.devicesIndex {
				prefix := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ ")
				sb.WriteString(MenuSelectedStyle.Width(64).Render(prefix + rowContent))
			} else {
				sb.WriteString("  " + rowContent)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	if m.notice != "" {
		if m.isErrorNotice {
			sb.WriteString(ErrorStyle.Render(m.notice) + "\n\n")
		} else {
			sb.WriteString(SuccessStyle.Render(m.notice) + "\n\n")
		}
	}

	helper := HelpStyle.Render("[↑/↓]: Navigate • [s/Enter]: Select & Connect • [a]: Add New • [d]: Unregister • [Esc]: Back")
	sb.WriteString(helper)

	return CardStyle.Width(68).Render(sb.String())
}

func (m Model) renderRegisterPlatform() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("✨ Register New Device - Step 1 of 3: Choose Operating System"))
	sb.WriteString("\n\n")

	infoText := lipgloss.NewStyle().Foreground(ColorFgLight).Render(
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
			lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("Advanced"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("Remove device"),
			lipgloss.NewStyle().Bold(true).Render("Add device"),
			lipgloss.NewStyle().Bold(true).Render("Bluetooth"),
			lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render(m.deviceModel),
		)
		sb.WriteString(DocBoxStyle.Width(64).Render(guideContent))
	}

	sb.WriteString("\n\n")
	sb.WriteString(HelpStyle.Render("[Enter / c]: Continue to Device Details • [Esc / b]: Back"))

	return FocusCardStyle.Width(68).Render(sb.String())
}

func (m Model) renderRegisterInput() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("📝 Register Device Details (Step 3 of 3)"))
	sb.WriteString("\n\n")

	var osBadge string
	if m.regPlatform == "macos" {
		osBadge = BadgeMacOS.Render()
	} else {
		osBadge = BadgeWindows.Render()
	}
	sb.WriteString(fmt.Sprintf("Target OS: %s\n\n", osBadge))

	var nameLabel, addrLabel string
	if m.regActiveIn == 0 {
		nameLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Device Friendly Name:")
		addrLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Bluetooth MAC Address:")
	} else {
		nameLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Device Friendly Name:")
		addrLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Bluetooth MAC Address:")
	}

	sb.WriteString(fmt.Sprintf("%s\n%s\n\n%s\n%s\n\n", nameLabel, m.devNameInput.View(), addrLabel, m.devAddrInput.View()))

	if m.notice != "" && m.isErrorNotice {
		sb.WriteString(ErrorStyle.Render(m.notice) + "\n\n")
	}

	sb.WriteString(HelpStyle.Render("[Tab/↓]: Switch Field • [Enter]: Save & Auto-Connect • [Esc]: Back"))
	return FocusCardStyle.Width(60).Render(sb.String())
}

func (m Model) renderDeleteConfirm() string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("⚠️ UNREGISTER DEVICE CONFIRMATION")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	devName := "Unknown"
	devAddr := "Unknown"
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
	userLabel := "Username:"
	passLabel := "Password:"
	if m.loginActiveIn == 0 {
		userLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Username:")
		passLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Password:")
	} else {
		userLabel = lipgloss.NewStyle().Foreground(ColorFgMuted).Render("  Username:")
		passLabel = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▶ Password:")
	}

	content := fmt.Sprintf(
		"🔐 %s\n\n%s\n%s\n\n%s\n%s\n\n%s",
		TitleStyle.Render("Windows Automated Login"),
		userLabel,
		m.userInput.View(),
		passLabel,
		m.passInput.View(),
		SubtitleStyle.Render("[Tab/↓]: Switch Field • [Enter]: Save & Unlock • [Esc]: Cancel"),
	)

	return FocusCardStyle.Width(50).Render(content)
}

func (m Model) renderLoginExecuting() string {
	content := fmt.Sprintf(
		"⚡ %s\n\n%s Transmitting automated authentication sequence...\n\n%s",
		TitleStyle.Render("Unlocking Windows"),
		m.spinner.View(),
		SubtitleStyle.Render("Ctrl+Alt+Del ➔ Esc ➔ Username ➔ Tab ➔ Password ➔ Enter"),
	)
	return CardStyle.Width(50).Render(content)
}

func (m Model) renderSleepCountdown() string {
	percent := float64(m.sleepTotal-m.sleepSeconds) / float64(m.sleepTotal)
	bar := m.progress.ViewAs(percent)

	content := fmt.Sprintf(
		"🌙 %s\n\nWindows sleep sequence (Esc ➔ Esc ➔ Win+X ➔ u ➔ s) dispatched!\nTerminating gateway to prevent phone battery drain in:\n\n  %s %s\n\n%s",
		TitleStyle.Render("Windows Host Sleep"),
		lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(fmt.Sprintf("%d SECONDS", m.sleepSeconds)),
		m.spinner.View(),
		bar,
	)
	return CardStyle.Width(54).Render(content)
}

func (m Model) renderComboInput() string {
	content := fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		TitleStyle.Render("🎹 Key Combination & Sequence Recorder"),
		m.textInput.View(),
		SubtitleStyle.Render("[Enter]: Transmit • Shortcuts (Ctrl+L, Alt+P, Esc) auto-record • Delays: -> (1.0s) or ->{1.5} • [Ctrl+D / Esc on empty]: Cancel"),
	)
	return FocusCardStyle.Width(72).Render(content)
}

func (m Model) renderTextInput(title string) string {
	content := fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		TitleStyle.Render(title),
		m.textInput.View(),
		SubtitleStyle.Render("[Enter]: Transmit to Host • [Esc]: Cancel"),
	)
	return FocusCardStyle.Width(52).Render(content)
}

func (m Model) renderCredsMenu() string {
	user := m.cfg.DefaultUser
	if user == "" {
		user = "None configured"
	}
	hasPass := "No"
	if keychain.IsAvailable() && m.cfg.DefaultUser != "" {
		if _, err := keychain.GetPassword(m.cfg.DefaultUser); err == nil {
			hasPass = "Yes (Encrypted in Keychain)"
		}
	}

	content := fmt.Sprintf(
		"🔑 %s\n\nDefault User: %s\nKeychain Pass: %s\n\n%s\n%s\n%s",
		TitleStyle.Render("Credential Vault"),
		lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(user),
		lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render(hasPass),
		lipgloss.NewStyle().Foreground(ColorYellow).Render("[u] Update credentials"),
		lipgloss.NewStyle().Foreground(ColorRed).Render("[c] Clear stored credentials"),
		SubtitleStyle.Render("[Esc]: Return to menu"),
	)
	return CardStyle.Width(50).Render(content)
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
		title := lipgloss.NewStyle().Bold(true).Width(24).Render(check.Title)
		info := SubtitleStyle.Render(check.Info)
		sb.WriteString(fmt.Sprintf("  %s %s %s\n", icon, title, info))
	}

	sb.WriteString("\n")
	sb.WriteString(SubtitleStyle.Render("[Enter/Esc]: Return to main menu"))
	return CardStyle.Width(62).Render(sb.String())
}

func (m Model) renderRecoveryView() string {
	content := fmt.Sprintf(
		"⚠️ %s\n\n%s\n\n%s\n%s\n%s\n\n%s",
		lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Render("Device Recovery Mode"),
		lipgloss.NewStyle().Foreground(ColorRed).Render(m.notice),
		"1. Connect phone via USB with USB Debugging enabled, OR",
		fmt.Sprintf("2. Connect Wireless ADB: 'adb connect %s'", m.cfg.WirelessADBAddress),
		"3. Check phone screen for 'Allow USB Debugging' authorization prompt.",
		SubtitleStyle.Render("[Enter/Esc]: Retry discovery"),
	)
	return CardStyle.Width(60).Render(content)
}
