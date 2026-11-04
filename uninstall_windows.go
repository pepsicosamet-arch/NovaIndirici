//go:build windows

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const installFolderName = "NOVA Indirici"

func psEncoded(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[i*2], b[i*2+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func runPS(script string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", psEncoded(script))
	hideWindow(cmd)
	return cmd.Run()
}

func askYesNo(title, msg string) bool {
	u := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	r, _, _ := u.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x04|0x20) // YESNO|QUESTION
	return r == 6
}

func showInfo(title, msg string) {
	u := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	u.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}

// handleCLI kaldırma modunu çalıştırır; işlendiyse true döner.
func handleCLI() bool {
	if len(os.Args) < 2 || os.Args[1] != "--uninstall" {
		return false
	}
	silent := len(os.Args) > 2 && strings.EqualFold(os.Args[2], "/S")
	if !silent && !askYesNo(AppName+" Kaldır", AppName+" bilgisayarınızdan kaldırılsın mı?\n\nİndirdiğiniz dosyalar silinmez.") {
		return true
	}
	self, _ := os.Executable()
	installDir := filepath.Dir(self)

	k := exec.Command("taskkill", "/F", "/IM", filepath.Base(self), "/FI", fmt.Sprintf("PID ne %d", os.Getpid()))
	hideWindow(k)
	_ = k.Run()

	_ = runPS(`$ErrorActionPreference='SilentlyContinue'
$n='NOVA İndirici'
Remove-Item -LiteralPath (Join-Path ([Environment]::GetFolderPath('Desktop')) "$n.lnk") -Force
Remove-Item -LiteralPath (Join-Path ([Environment]::GetFolderPath('Programs')) "$n.lnk") -Force
Remove-Item -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\NovaIndirici' -Recurse -Force`)

	base, _ := os.UserCacheDir()
	data := filepath.Join(base, "NovaIndirici")
	_ = os.RemoveAll(data)

	// Kendini silme: yalnızca özel kurulum klasörünü kaldır, başka klasöre dokunma.
	var target string
	if strings.EqualFold(filepath.Base(installDir), installFolderName) {
		target = fmt.Sprintf(`rmdir /s /q "%s"`, installDir)
	} else {
		target = fmt.Sprintf(`del /f /q "%s"`, self)
	}
	line := fmt.Sprintf(`cmd.exe /c ping 127.0.0.1 -n 3 >nul & %s & rmdir /s /q "%s"`, target, data)
	c := exec.Command("cmd.exe")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: line, HideWindow: true, CreationFlags: createNoWindow}
	_ = c.Start()
	if !silent {
		showInfo(AppName, AppName+" kaldırıldı.")
	}
	return true
}
