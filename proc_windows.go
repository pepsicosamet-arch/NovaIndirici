//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	k := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(cmd.Process.Pid))
	hideWindow(k)
	if err := k.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func findBrowser() (string, bool) {
	cands := []string{}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		if b := os.Getenv(env); b != "" {
			cands = append(cands,
				filepath.Join(b, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(b, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(b, `BraveSoftware\Brave-Browser\Application\brave.exe`))
		}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

// openWindow arayüzü Edge/Chrome "uygulama penceresi" olarak açar; yoksa varsayılan tarayıcıyı kullanır.
func openWindow(url string) {
	if b, ok := findBrowser(); ok {
		prof := filepath.Join(dataDir, "window")
		cmd := exec.Command(b, "--app="+url, "--user-data-dir="+prof, "--window-size=1180,800",
			"--no-first-run", "--no-default-browser-check", "--disable-features=Translate",
			"--lang=tr")
		if err := cmd.Start(); err == nil {
			return
		}
	}
	_ = openURL(url)
}

func openURL(u string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
}

func openPath(p string, reveal bool) error {
	if reveal {
		cmd := exec.Command("explorer")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + p + `"`}
		return cmd.Start()
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
}

func pickFolder(initial string) (string, error) {
	ps := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$f = New-Object System.Windows.Forms.FolderBrowserDialog;` +
		`$f.Description = 'İndirme klasörünü seçin';` +
		`$f.SelectedPath = '` + strings.ReplaceAll(initial, "'", "''") + `';` +
		`$f.ShowNewFolderButton = $true;` +
		`$o = New-Object System.Windows.Forms.Form; $o.TopMost = $true;` +
		`if ($f.ShowDialog($o) -eq 'OK') { [Console]::OutputEncoding=[Text.Encoding]::UTF8; Write-Output $f.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("klasör seçici açılamadı")
	}
	return strings.TrimSpace(string(out)), nil
}

func showError(title, msg string) {
	u := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	u.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}

// runInstaller güncelleme kurulumunu bağımsız bir süreç olarak başlatır.
func runInstaller(path string) error {
	cmd := exec.Command(path, "/S", "/LAUNCH")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | NEW_PROCESS_GROUP
	return cmd.Start()
}
