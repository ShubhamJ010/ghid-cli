package combos

import (
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := []string{
		"win+r",
		"ctrl+shift+esc",
		"alt+tab",
		"printscreen",
		"win+shift+s",
		"click-left",
		"win+r ->{0.9} tab ->{0.2} click-left",
		"scroll-down",
		"win+s ->{0.7} type:rustdesk ->{0.3} enter",
		`type:"rust desk"`,
		"ctrl+alt+del ->{1.0} esc ->{0.5} $USERNAME ->{0.2} tab ->{0.2} $PASSWORD ->{0.2} enter",
		"type:$USERNAME -> type:${PASSWORD}",
	}
	for _, keys := range valid {
		if err := Validate(keys); err != nil {
			t.Errorf("Validate(%q) unexpected error: %v", keys, err)
		}
	}

	invalid := []string{
		"",
		"cmd+space",
		"super+l",
		"option+tab",
		"hyper+q",
		"click:left",
		"type:",
		"type:\"snowman ☃\"",
	}
	for _, keys := range invalid {
		if err := Validate(keys); err == nil {
			t.Errorf("Validate(%q) expected an error", keys)
		}
	}
}

func TestStoreAddReplaceDelete(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "combos.json"))

	list, err := store.Add(Combo{Name: "Run", Keys: "win+r"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Run" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Same name (different case) replaces instead of duplicating.
	list, err = store.Add(Combo{Name: "run", Keys: "win+r ->{0.5} click-left"})
	if err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 combo after replace, got %d", len(list))
	}
	if list[0].Keys != "win+r ->{0.5} click-left" {
		t.Fatalf("replacement keys not saved: %q", list[0].Keys)
	}

	if _, err := store.Add(Combo{Name: "Bad", Keys: "hyper+space"}); err == nil {
		t.Fatal("expected invalid keys to be rejected")
	}
	if _, err := store.Add(Combo{Name: "  ", Keys: "win+r"}); err == nil {
		t.Fatal("expected empty name to be rejected")
	}

	if _, ok := store.Find("RUN"); !ok {
		t.Fatal("Find should be case-insensitive")
	}

	list, deleted, err := store.Delete("Run")
	if err != nil || !deleted {
		t.Fatalf("Delete failed: deleted=%v err=%v", deleted, err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty library, got %+v", list)
	}

	if _, deleted, _ := store.Delete("Run"); deleted {
		t.Fatal("deleting a missing combo should report deleted=false")
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "nested", "combos.json"))
	list, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file returned error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func TestStoreEnsureDefaultsAndFavorites(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "combos.json"))

	list, err := store.EnsureDefaults()
	if err != nil {
		t.Fatalf("EnsureDefaults failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 default combo, got %d", len(list))
	}
	if list[0].Name != "Login" || !list[0].Favorite {
		t.Errorf("expected Login favorite combo, got %+v", list[0])
	}
	if list[0].Keys != DefaultLoginKeys {
		t.Errorf("expected DefaultLoginKeys, got %q", list[0].Keys)
	}

	favs, err := store.Favorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "Login" {
		t.Errorf("Favorites failed: %+v", favs)
	}

	// Toggle favorite
	_, ok, err := store.ToggleFavorite("Login")
	if err != nil || !ok {
		t.Fatalf("ToggleFavorite failed: ok=%v err=%v", ok, err)
	}
	updated, _ := store.Find("Login")
	if updated.Favorite {
		t.Errorf("expected Login to no longer be favorite")
	}

	// Toggle back
	_, ok, err = store.ToggleFavorite("Login")
	if err != nil || !ok {
		t.Fatalf("ToggleFavorite failed: ok=%v err=%v", ok, err)
	}
	updated, _ = store.Find("Login")
	if !updated.Favorite {
		t.Errorf("expected Login to be favorite again")
	}
}
