//go:build !windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

func networkProfileWarning() string { return "" }

func systemPrefersRussian() bool {
	l := os.Getenv("LANG")
	return len(l) >= 2 && (l[:2] == "ru" || l[:2] == "kk" || l[:2] == "be" || l[:2] == "uk")
}
