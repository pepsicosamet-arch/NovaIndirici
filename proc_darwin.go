//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

var macBrowsers = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
}

// openWindow arayüzü Chrome/Edge "uygulama penceresi" olarak açar; yoksa varsayılan tarayıcı.
func openWindow(url string) {
	if os.Getenv("NOVA_NO_BROWSER") != "" {
		return
	}
	home, _ := os.UserHomeDir()
	for _, b := range macBrowsers {
		for _, p := range []string{b, filepath.Join(home, b)} {
			if _, err := os.Stat(p); err == nil {
				cmd := exec.Command(p, "--app="+url, "--user-data-dir="+filepath.Join(dataDir, "window"),
					"--window-size=1180,800", "--no-first-run", "--no-default-browser-check", "--lang=tr")
				if cmd.Start() == nil {
					return
				}
			}
		}
	}
	_ = openURL(url)
}

func openURL(u string) error { return exec.Command("open", u).Start() }

func openPath(p string, reveal bool) error {
	if reveal {
		return exec.Command("open", "-R", p).Start()
	}
	return exec.Command("open", p).Start()
}

func osaQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func pickFolder(initial string) (string, error) {
	script := `POSIX path of (choose folder with prompt "İndirme klasörünü seçin"`
	if st, err := os.Stat(initial); err == nil && st.IsDir() {
		script += ` default location (POSIX file ` + osaQuote(initial) + `)`
	}
	script += `)`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) { // kullanıcı iptal etti
			return "", nil
		}
		return "", errors.New("klasör seçici açılamadı")
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/"), nil
}

func showError(title, msg string) {
	_ = exec.Command("osascript", "-e", `display alert `+osaQuote(title)+` message `+osaQuote(msg)+` as critical`).Run()
}

func runInstaller(path string) error { return errors.New("desteklenmiyor") }
