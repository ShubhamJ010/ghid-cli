package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ghost-hid/ghid/internal/combos"
)

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func newTestStudio(t *testing.T, sent *[]string) (ComboStudioModel, *combos.Store) {
	t.Helper()
	store := combos.NewStore(filepath.Join(t.TempDir(), "combos.json"))
	m := NewComboStudioModel(store, func(c combos.Combo) error {
		if sent != nil {
			*sent = append(*sent, c.Keys)
		}
		return nil
	})
	return m, store
}

func TestComboStudioEmptyState(t *testing.T) {
	m, _ := newTestStudio(t, nil)
	view := m.View()
	if !strings.Contains(view, "No combos yet") {
		t.Fatalf("empty studio should explain the empty state:\n%s", view)
	}
	if !strings.Contains(view, "SEND") {
		t.Fatalf("studio should always show the SEND button:\n%s", view)
	}
}

func TestComboStudioCreateAndSend(t *testing.T) {
	var sent []string
	m, store := newTestStudio(t, &sent)

	nm, _ := m.Update(keyRune('n'))
	m = nm.(ComboStudioModel)
	if m.mode != studioCreate {
		t.Fatal("n should open the create form")
	}

	m.nameInput.SetValue("Open Run")
	m.keysInput.SetValue("win+r")
	m.formFocus = 1

	nm, _ = m.saveCombo(false)
	m = nm.(ComboStudioModel)
	if len(m.list) != 1 {
		t.Fatalf("expected one saved combo, got %+v", m.list)
	}

	saved, ok := store.Find("open run")
	if !ok || saved.Keys != "win+r" {
		t.Fatalf("combo not persisted: %+v ok=%v", saved, ok)
	}

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(ComboStudioModel)
	if cmd == nil {
		t.Fatal("enter on the selected combo should produce a send command")
	}
	nm, _ = m.Update(cmd())
	m = nm.(ComboStudioModel)
	if len(sent) != 1 || sent[0] != "win+r" {
		t.Fatalf("expected one send of win+r, got %v", sent)
	}
	if !strings.Contains(m.View(), "Sent") {
		t.Fatalf("studio should report the send result:\n%s", m.View())
	}
}

func TestComboStudioRejectsMacKeys(t *testing.T) {
	m, store := newTestStudio(t, nil)
	nm, _ := m.Update(keyRune('n'))
	m = nm.(ComboStudioModel)
	m.nameInput.SetValue("Spotlight")
	m.keysInput.SetValue("cmd+space")
	m.formFocus = 1

	nm, _ = m.saveCombo(false)
	m = nm.(ComboStudioModel)

	if len(m.list) != 0 {
		t.Fatalf("macOS-only keys must not be saved: %+v", m.list)
	}
	if list, _ := store.Load(); len(list) != 0 {
		t.Fatalf("store should be empty, got %+v", list)
	}
	if !strings.Contains(m.View(), "macOS") {
		t.Fatalf("studio should explain that cmd is a macOS key:\n%s", m.View())
	}
}

func TestComboStudioDeleteConfirmation(t *testing.T) {
	m, store := newTestStudio(t, nil)
	if _, err := store.Add(combos.Combo{Name: "Lock", Keys: "win+l"}); err != nil {
		t.Fatalf("seed combo: %v", err)
	}
	m.list, _ = store.Load()

	// Delete asks first.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = nm.(ComboStudioModel)
	if m.mode != studioConfirmDelete {
		t.Fatal("delete key should open the confirmation prompt")
	}
	if !strings.Contains(m.View(), "Delete") {
		t.Fatalf("confirmation view missing:\n%s", m.View())
	}

	// n keeps it.
	nm, _ = m.Update(keyRune('n'))
	m = nm.(ComboStudioModel)
	if len(m.list) != 1 {
		t.Fatal("cancelling must keep the combo")
	}

	// y removes it.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = nm.(ComboStudioModel)
	nm, _ = m.Update(keyRune('y'))
	m = nm.(ComboStudioModel)
	if len(m.list) != 0 {
		t.Fatalf("confirmed delete should empty the list: %+v", m.list)
	}
	if list, _ := store.Load(); len(list) != 0 {
		t.Fatalf("confirmed delete should update disk: %+v", list)
	}
}

func TestComboStudioSendButton(t *testing.T) {
	var sent []string
	m, store := newTestStudio(t, &sent)
	if _, err := store.Add(combos.Combo{Name: "Lock", Keys: "win+l"}); err != nil {
		t.Fatalf("seed combo: %v", err)
	}
	m.list, _ = store.Load()

	// Tab focuses the SEND button, Enter presses it.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(ComboStudioModel)
	if m.focus != 1 {
		t.Fatalf("tab should focus SEND, focus = %d", m.focus)
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(ComboStudioModel)
	if cmd == nil {
		t.Fatal("pressing the SEND button should dispatch the combo")
	}
	nm, _ = m.Update(cmd())
	m = nm.(ComboStudioModel)
	if len(sent) != 1 || sent[0] != "win+l" {
		t.Fatalf("SEND button sent %v, want [win+l]", sent)
	}
}
