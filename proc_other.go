//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func openWindow(url string) {
	if os.Getenv("NOVA_NO_BROWSER") != "" {
		return
	}
	_ = openURL(url)
}

func openURL(u string) error { return exec.Command("xdg-open", u).Start() }

func openPath(p string, reveal bool) error {
	if reveal {
		p = filepath.Dir(p)
	}
	return exec.Command("xdg-open", p).Start()
}

func pickFolder(initial string) (string, error) {
	return "", errors.New("bu sistemde klasör seçici yok")
}

func showError(title, msg string) { fmt.Fprintln(os.Stderr, title+": "+msg) }
