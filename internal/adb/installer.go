package adb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultGhostHidRepo is the official GitHub repository for GhostHID releases.
	DefaultGhostHidRepo = "ShubhamJ010/ghost-hid"
)

// SetupReport summarizes the outcome of the automated device setup flow.
type SetupReport struct {
	DeviceSerial       string `json:"device_serial"`
	DeviceModel        string `json:"device_model"`
	ApkInstalled       bool   `json:"apk_installed"`
	ApkDownloaded      bool   `json:"apk_downloaded"`
	BatteryWhitelisted bool   `json:"battery_whitelisted"`
	ServiceStarted     bool   `json:"service_started"`
	BluetoothOn        bool   `json:"bluetooth_on"`
	Registered         bool   `json:"registered"`
	Connected          bool   `json:"connected"`
	Target             string `json:"target,omitempty"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type githubReleaseResponse struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

// DownloadReleaseApk fetches the latest release APK from the specified GitHub
// repository (defaulting to ShubhamJ010/ghost-hid). It writes the file to a
// temporary directory and returns the absolute path to the downloaded APK.
// Callers should remove the file once installation completes.
func DownloadReleaseApk(repo string, progress func(downloaded, total int64)) (string, error) {
	if strings.TrimSpace(repo) == "" {
		repo = DefaultGhostHidRepo
	}

	downloadURL, _, err := resolveReleaseApkURL(repo)
	if err != nil {
		// Fallback: try gh CLI if present on host
		if ghPath, lookErr := exec.LookPath("gh"); lookErr == nil {
			tmpDir, mkErr := os.MkdirTemp("", "ghost-hid-gh-*")
			if mkErr == nil {
				cmd := exec.Command(ghPath, "release", "download", "--repo", repo, "--pattern", "*.apk", "--dir", tmpDir)
				if cmdErr := cmd.Run(); cmdErr == nil {
					matches, _ := filepath.Glob(filepath.Join(tmpDir, "*.apk"))
					if len(matches) > 0 {
						return matches[0], nil
					}
				}
				_ = os.RemoveAll(tmpDir)
			}
		}
		return "", fmt.Errorf("could not locate release APK for %s: %w", repo, err)
	}

	tmpFile, err := os.CreateTemp("", "ghost-hid-*.apk")
	if err != nil {
		return "", fmt.Errorf("create temporary APK file: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	req.Header.Set("User-Agent", "ghid-cli")

	resp, err := client.Do(req)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download APK failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download APK HTTP status: %s", resp.Status)
	}

	var reader io.Reader = resp.Body
	if progress != nil && resp.ContentLength > 0 {
		reader = &progressReader{
			reader:     resp.Body,
			total:      resp.ContentLength,
			onProgress: progress,
		}
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temporary APK: %w", err)
	}

	return tmpFile.Name(), nil
}

type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.onProgress != nil {
		pr.onProgress(pr.downloaded, pr.total)
	}
	return n, err
}

func resolveReleaseApkURL(repo string) (string, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "ghid-cli")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var rel githubReleaseResponse
		if decErr := json.NewDecoder(resp.Body).Decode(&rel); decErr == nil {
			for _, asset := range rel.Assets {
				if strings.HasSuffix(strings.ToLower(asset.Name), ".apk") {
					return asset.BrowserDownloadURL, rel.TagName, nil
				}
			}
		}
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Fallback: check redirect on releases/latest HTML
	webClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	webResp, err := webClient.Get(fmt.Sprintf("https://github.com/%s/releases/latest", repo))
	if err == nil && (webResp.StatusCode == http.StatusFound || webResp.StatusCode == http.StatusMovedPermanently) {
		loc := webResp.Header.Get("Location")
		_ = webResp.Body.Close()
		re := regexp.MustCompile(`/releases/tag/([^/]+)$`)
		if m := re.FindStringSubmatch(loc); len(m) > 1 {
			tag := m[1]
			expandedURL := fmt.Sprintf("https://github.com/%s/releases/expanded_assets/%s", repo, tag)
			expResp, expErr := client.Get(expandedURL)
			if expErr == nil {
				defer expResp.Body.Close()
				bodyBytes, _ := io.ReadAll(expResp.Body)
				reAsset := regexp.MustCompile(fmt.Sprintf(`href="(/%s/releases/download/%s/[^"]+\.apk)"`, regexp.QuoteMeta(repo), regexp.QuoteMeta(tag)))
				if sub := reAsset.FindStringSubmatch(string(bodyBytes)); len(sub) > 1 {
					return "https://github.com" + sub[1], tag, nil
				}
			}
		}
	}

	return "", "", errors.New("no APK asset found in latest release")
}

// GrantAllPermissions ensures battery optimization whitelist and runtime permissions
// are granted to the GhostHID package.
func (m *Manager) GrantAllPermissions() {
	_, _ = m.Run("shell", "dumpsys", "deviceidle", "whitelist", "+"+PackageName)
	perms := []string{
		"android.permission.POST_NOTIFICATIONS",
		"android.permission.BLUETOOTH_CONNECT",
		"android.permission.BLUETOOTH_SCAN",
		"android.permission.BLUETOOTH_ADVERTISE",
	}
	for _, p := range perms {
		_, _ = m.Run("shell", "pm", "grant", PackageName, p)
	}
}

// InstallReleaseApk downloads the latest APK from the specified GitHub repo,
// installs it onto the device with -g (all permissions) and -r, and immediately
// deletes the temporary downloaded file.
func (m *Manager) InstallReleaseApk(repo string) error {
	tmpApk, err := DownloadReleaseApk(repo, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpApk)
	}()

	// -g grants all runtime permissions on Android 6+
	_, err = m.Run("install", "-g", "-r", tmpApk)
	if err != nil {
		if strings.Contains(err.Error(), "unknown option -g") || strings.Contains(err.Error(), "invalid option") {
			_, err = m.Run("install", "-r", tmpApk)
		}
		if err != nil {
			return fmt.Errorf("adb install: %w", err)
		}
	}

	m.GrantAllPermissions()
	_, _ = m.Run("shell", "am", "force-stop", PackageName)
	return nil
}

// SetupDevice executes a minimal, clean setup flow:
// 1. Checks if GhostHID APK is installed; if not, downloads & installs from GitHub release.
// 2. Ensures battery optimization whitelist and runtime permissions.
// 3. Starts the foreground gateway service.
// 4. Verifies Bluetooth state.
// 5. Connects to target laptop if address is supplied.
func (m *Manager) SetupDevice(targetAddr string) (*SetupReport, error) {
	if _, err := m.SelectDevice(); err != nil {
		return nil, fmt.Errorf("no connected device: %w", err)
	}

	rep := &SetupReport{
		DeviceSerial: m.Serial,
		DeviceModel:  m.GetDeviceModel(),
	}

	if !m.IsPackageInstalled() {
		if err := m.InstallReleaseApk(DefaultGhostHidRepo); err != nil {
			return rep, fmt.Errorf("install APK from release: %w", err)
		}
		rep.ApkDownloaded = true
		rep.ApkInstalled = true
	} else {
		rep.ApkInstalled = true
		m.GrantAllPermissions()
	}

	rep.BatteryWhitelisted = true

	if err := m.StartGateway(); err != nil {
		return rep, fmt.Errorf("start gateway service: %w", err)
	}
	rep.ServiceStarted = true

	rep.BluetoothOn = m.GetBluetoothState()

	time.Sleep(800 * time.Millisecond)
	status := m.GetStatus()
	rep.Registered = status.Registered
	rep.Connected = status.Connected

	if targetAddr != "" {
		rep.Target = targetAddr
		_ = m.ConnectTarget(targetAddr)
		time.Sleep(500 * time.Millisecond)
		status = m.GetStatus()
		rep.Connected = status.Connected
	}

	return rep, nil
}
