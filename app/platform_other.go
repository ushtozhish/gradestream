//go:build !windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func appDataDir() string       { return filepath.Join(os.TempDir(), "gradestream") }
func hideWindow(cmd *exec.Cmd) {}
func afterStart(pid int)       {}
func showError(msg string)     { log.Println("ERROR:", msg) }
func openHostWindow(url string, detached bool) {
	log.Println("HOST URL:", url)
	if !detached {
		select {}
	}
}

func documentsDir() string { h, _ := os.UserHomeDir(); return filepath.Join(h, "Documents") }
func openPath(p string)    { log.Println("OPEN:", p) }

// Не Windows: для проверки на тестовом стенде имя «общедоступной» сети можно задать переменной окружения
func publicNetworks() string { return os.Getenv("GS_TEST_PUBLIC_NET") }

func systemLang() string {
	l := strings.ToLower(os.Getenv("LANG"))
	switch {
	case strings.HasPrefix(l, "kk"):
		return "kk"
	case strings.HasPrefix(l, "ru"):
		return "ru"
	}
	return "en"
}
