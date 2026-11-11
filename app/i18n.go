package main

// Язык интерфейса: русский или английский.
// Настройка «auto» — по языку интерфейса Windows (русский для ru/kk/be/uk, иначе английский).

import "sync/atomic"

var langEN atomic.Bool

// L возвращает текст на текущем языке программы
func L(ru, en string) string {
	if langEN.Load() {
		return en
	}
	return ru
}

func currentLang() string {
	if langEN.Load() {
		return "en"
	}
	return "ru"
}

// Применяет настройку языка: "ru", "en" или "" (авто)
func applyLang(pref string) {
	switch pref {
	case "en":
		langEN.Store(true)
	case "ru":
		langEN.Store(false)
	default:
		langEN.Store(!systemPrefersRussian())
	}
}

func setLang(pref string) {
	if pref != "ru" && pref != "en" {
		pref = ""
	}
	setMu.Lock()
	cfg.Lang = pref
	saveSettings()
	setMu.Unlock()
	applyLang(pref)
	sendState()
}

func langPref() string {
	setMu.Lock()
	defer setMu.Unlock()
	return cfg.Lang
}
