package main

// Язык интерфейса: английский, русский или казахский (порядок EN → RU → KZ).
// Настройка «» (авто) — по языку интерфейса Windows: kk → казахский, ru → русский, иначе английский.

import "sync/atomic"

var lang atomic.Value // "en" | "ru" | "kk"

func init() { lang.Store("en") }

// L возвращает текст на текущем языке программы
func L(ru, en, kk string) string {
	switch lang.Load().(string) {
	case "ru":
		return ru
	case "kk":
		return kk
	}
	return en
}

func currentLang() string { return lang.Load().(string) }

func validLang(p string) bool { return p == "en" || p == "ru" || p == "kk" }

// Применяет настройку языка: "en", "ru", "kk" или "" (авто)
func applyLang(pref string) {
	if !validLang(pref) {
		pref = systemLang()
	}
	lang.Store(pref)
}

func setLang(pref string) {
	if !validLang(pref) {
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

// Msg — текст, который хранится дольше одного показа (ошибки, предупреждения).
// Переводится в момент показа, поэтому после смены языка сразу на новом языке.
type Msg func() string

func M(ru, en, kk string) Msg { return func() string { return L(ru, en, kk) } }

// Msg с непереводимым хвостом (текст системной ошибки, имена сетей)
func MS(ru, en, kk, tail string) Msg { return func() string { return L(ru, en, kk) + tail } }

func (m Msg) S() string {
	if m == nil {
		return ""
	}
	return m()
}
