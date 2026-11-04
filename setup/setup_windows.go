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

const createNoWindow = 0x08000000

var uiProc *os.Process

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func ps(script string) ([]byte, error) {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[i*2], b[i*2+1] = byte(c), byte(c>>8)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	hide(cmd)
	return cmd.CombinedOutput()
}

func psq(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func existingDirRaw() string {
	out, err := ps(`[Console]::OutputEncoding=[Text.Encoding]::UTF8; (Get-ItemProperty -LiteralPath 'HKCU:\` + RegKey + `' -ErrorAction SilentlyContinue).InstallLocation`)
	if err != nil {
		return ""
	}
	d := strings.TrimSpace(string(out))
	if d == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(d, ExeName)); err != nil {
		return ""
	}
	return d
}

func stopRunning() {
	k := exec.Command("taskkill", "/F", "/IM", ExeName)
	hide(k)
	_ = k.Run()
}

func registerApp(exe, dir string, desktop bool, sizeKB int64) error {
	script := `$ErrorActionPreference='Stop'
$exe=` + psq(exe) + `; $dir=` + psq(dir) + `; $n='NOVA İndirici'
$sh=New-Object -ComObject WScript.Shell
function MK($p){ $s=$sh.CreateShortcut($p); $s.TargetPath=$exe; $s.WorkingDirectory=$dir; $s.IconLocation="$exe,0"; $s.Description='Video ve ses indirici'; $s.Save() }
MK (Join-Path ([Environment]::GetFolderPath('Programs')) "$n.lnk")
$d=Join-Path ([Environment]::GetFolderPath('Desktop')) "$n.lnk"
if (` + map[bool]string{true: "$true", false: "$false"}[desktop] + `) { MK $d } elseif (Test-Path -LiteralPath $d) { Remove-Item -LiteralPath $d -Force }
$k='HKCU:\` + RegKey + `'
New-Item -Path $k -Force | Out-Null
$v=@{ DisplayName=$n; DisplayVersion='` + AppVersion + `'; Publisher='Samet Şahin'; DisplayIcon="$exe,0"; InstallLocation=$dir;
 UninstallString="` + "`" + `"$exe` + "`" + `" --uninstall"; QuietUninstallString="` + "`" + `"$exe` + "`" + `" --uninstall /S"; URLInfoAbout='https://www.instagram.com/xsametsahinx/' }
foreach($e in $v.GetEnumerator()){ New-ItemProperty -Path $k -Name $e.Key -Value $e.Value -PropertyType String -Force | Out-Null }
foreach($e in @{NoModify=1;NoRepair=1;EstimatedSize=` + fmt.Sprint(sizeKB) + `}.GetEnumerator()){ New-ItemProperty -Path $k -Name $e.Key -Value $e.Value -PropertyType DWord -Force | Out-Null }
`
	out, err := ps(script)
	if err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func pickFolder(initial string) (string, error) {
	out, err := ps(`Add-Type -AssemblyName System.Windows.Forms
$f=New-Object System.Windows.Forms.FolderBrowserDialog
$f.Description='NOVA İndirici nereye kurulsun?'
$f.SelectedPath=` + psq(initial) + `
$o=New-Object System.Windows.Forms.Form; $o.TopMost=$true
if($f.ShowDialog($o) -eq 'OK'){ [Console]::OutputEncoding=[Text.Encoding]::UTF8; Write-Output $f.SelectedPath }`)
	if err != nil {
		return "", fmt.Errorf("klasör seçici açılamadı")
	}
	return strings.TrimSpace(string(out)), nil
}

func findBrowser() string {
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		b := os.Getenv(env)
		if b == "" {
			continue
		}
		for _, rel := range []string{`Microsoft\Edge\Application\msedge.exe`, `Google\Chrome\Application\chrome.exe`} {
			p := filepath.Join(b, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func openWindow(url string) bool {
	b := findBrowser()
	if b == "" {
		return false
	}
	prof := filepath.Join(os.TempDir(), "NovaSetupUI")
	cmd := exec.Command(b, "--app="+url, "--user-data-dir="+prof, "--window-size=640,600",
		"--no-first-run", "--no-default-browser-check", "--disable-features=Translate", "--lang=tr")
	if cmd.Start() != nil {
		return false
	}
	uiProc = cmd.Process
	return true
}

func closeWindow() {
	if uiProc != nil {
		_ = uiProc.Kill()
	}
}

func launch(exe string) {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	_ = cmd.Start()
}

func msg(title, text string, flags uintptr) int {
	u := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := u.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

// Tarayıcı bulunamazsa basit pencerelerle kurulum.
func fallbackInstall(dir string) {
	if msg(AppName+" Kurulum", AppName+" "+AppVersion+" bilgisayarınıza kurulsun mu?\n\nKonum: "+normalizeDir(dir), 0x04|0x40) != 6 {
		return
	}
	exe, err := Install(dir, true)
	if err != nil {
		msg(AppName+" Kurulum", "Kurulum başarısız:\n"+err.Error(), 0x10)
		return
	}
	if msg(AppName+" Kurulum", "Kurulum tamamlandı. NOVA İndirici şimdi açılsın mı?", 0x04|0x40) == 6 {
		launch(exe)
	}
}
