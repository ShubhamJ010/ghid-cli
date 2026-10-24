package combos

import (
	"encoding/json"
	"os"
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
	if len(list) != 6 {
		t.Fatalf("expected 6 default combos (3 Windows + 3 macOS), got %d", len(list))
	}
	names := map[string]Combo{}
	for _, c := range list {
		names[c.Name] = c
	}
	if c, ok := names["Login"]; !ok || !c.Favorite || c.OS != OSWindows || c.Description == "" {
		t.Errorf("expected Login built-in with OS and description, got %+v", c)
	}
	if c, ok := names["Lock"]; !ok || !c.Favorite || c.OS != OSWindows || c.Description == "" {
		t.Errorf("expected Lock built-in with OS and description, got %+v", c)
	}
	if c, ok := names["Sleep"]; !ok || !c.Favorite || c.OS != OSWindows || c.Description == "" {
		t.Errorf("expected Sleep built-in with OS and description, got %+v", c)
	}
	if c, ok := names["MacSleep"]; !ok || !c.Favorite || c.OS != OSMacOS || c.Description == "" {
		t.Errorf("expected MacSleep built-in with OS and description, got %+v", c)
	}
	if c, ok := names["ForceQuit"]; !ok || !c.Favorite || c.OS != OSMacOS || c.Description == "" {
		t.Errorf("expected ForceQuit built-in with OS and description, got %+v", c)
	}
	if c, ok := names["PlayPause"]; !ok || !c.Favorite || c.OS != OSMacOS || c.Description == "" {
		t.Errorf("expected PlayPause built-in with OS and description, got %+v", c)
	}

	favs, err := store.Favorites()
	if err != nil || len(favs) != 6 {
		t.Errorf("expected 6 favorites, got %d: %+v", len(favs), favs)
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

func TestNormalizeOS(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", OSWindows, false},
		{"windows", OSWindows, false},
		{"WIN", OSWindows, false},
		{"macos", OSMacOS, false},
		{"Mac", OSMacOS, false},
		{"darwin", OSMacOS, false},
		{"ubuntu", OSLinux, false},
		{"Linux", OSLinux, false},
		{"beos", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeOS(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizeOS(%q) expected an error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("NormalizeOS(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestStoreAddKeepsOSAndRejectsUnknown(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "combos.json"))

	if _, err := store.Add(Combo{Name: "Terminal", Keys: "ctrl+alt+t", OS: OSMacOS}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// An update without an OS keeps the stored tag instead of resetting it.
	list, err := store.Add(Combo{Name: "Terminal", Keys: "ctrl+alt+t ->{0.5} type:htop"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if list[0].OS != OSMacOS {
		t.Fatalf("expected macOS to be preserved, got %q", list[0].OS)
	}

	if _, err := store.Add(Combo{Name: "Bogus", Keys: "win+r", OS: "beos"}); err == nil {
		t.Fatal("expected unknown OS to be rejected")
	}
}

func TestStoreMigratesLegacyEmojiCombos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "combos.json")
	legacy := `[{"name":"Old","keys":"win+r","emoji":"🚀","favorite":true,"created_at":"2026-10-20T10:00:00Z"}]`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	store := NewStore(path)
	list, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(list) != 1 || list[0].OS != OSWindows {
		t.Fatalf("legacy combo should migrate to windows, got %+v", list)
	}

	// Saving after migration writes the os field and drops emoji.
	if _, err := store.Add(list[0]); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, hasEmoji := raw[0]["emoji"]; hasEmoji {
		t.Error("emoji should no longer be persisted")
	}
	if raw[0]["os"] != OSWindows {
		t.Errorf("expected os=windows, got %v", raw[0]["os"])
	}
}

func TestValidateForOS(t *testing.T) {
	macKeys := []string{
		"cmd+space",
		"super+l",
		"option+tab",
		"raw:cd00 ->{0.06} raw:0000",
		"ctrl+cmd+q",
	}
	for _, keys := range macKeys {
		if err := ValidateForOS(keys, OSMacOS); err != nil {
			t.Errorf("ValidateForOS(%q, macos) unexpected error: %v", keys, err)
		}
	}
}
