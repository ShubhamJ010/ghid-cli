package adb

import (
	"strings"
	"testing"
)

func TestResolveReleaseApkURL(t *testing.T) {
	url, tag, err := resolveReleaseApkURL(DefaultGhostHidRepo)
	if err != nil {
		t.Fatalf("resolveReleaseApkURL failed: %v", err)
	}
	if !strings.HasPrefix(url, "https://") || !strings.HasSuffix(url, ".apk") {
		t.Errorf("unexpected download URL: %s", url)
	}
	if tag == "" {
		t.Errorf("empty tag returned")
	}
	t.Logf("Resolved latest release tag %s with APK at: %s", tag, url)
}
