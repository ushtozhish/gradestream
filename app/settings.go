package main

// Настройки, которые переживают перезапуск программы.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	SnapBase  string `json:"snapBase"`  // папка для скриншотов; пусто — Документы\GradeStream
	Subfolder *bool  `json:"subfolder"` // подпапка на каждую сессию (по умолчанию да)
	Mode      string `json:"mode"`      // internet | lan | both
	Lang      string `json:"lang"`      // ru | en | "" — по языку Windows
}

var (
	setMu sync.Mutex
	cfg   Settings
)

func settingsPath() string { return filepath.Join(dataDir, "settings.json") }

func loadSettings() {
	setMu.Lock()
	defer setMu.Unlock()
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &cfg)
	}
}

func saveSettings() {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(settingsPath(), b, 0o644)
}

func defaultSnapBase() string { return filepath.Join(documentsDir(), "GradeStream") }

func snapSettings() (base string, sub bool) {
	setMu.Lock()
	defer setMu.Unlock()
	base = cfg.SnapBase
	if base == "" {
		base = defaultSnapBase()
	}
	sub = cfg.Subfolder == nil || *cfg.Subfolder
	return
}
