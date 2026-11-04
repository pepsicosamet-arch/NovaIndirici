//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Test amaçlı (Linux) taklit sürümler.
func existingDirRaw() string {
	d := filepath.Join(os.TempDir(), "nova-setup-test", "Programs", FolderName)
	if _, err := os.Stat(filepath.Join(d, ExeName)); err == nil {
		return d
	}
	return ""
}
func stopRunning() {}
func registerApp(exe, dir string, desktop bool, sizeKB int64) error {
	return os.WriteFile(filepath.Join(dir, "shortcuts.txt"), []byte(fmt.Sprintf("desktop=%v size=%d", desktop, sizeKB)), 0o644)
}
func pickFolder(initial string) (string, error) { return "", fmt.Errorf("desteklenmiyor") }
func openWindow(url string) bool                { return false }
func launch(exe string)                         {}
func fallbackInstall(dir string)                { fmt.Println("fallback", dir) }
func closeWindow()                              {}
