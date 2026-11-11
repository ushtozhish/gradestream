package main

// Скриншоты меток клиента: снимок кадра, который клиент видел в момент метки,
// с нарисованной меткой и комментарием + сводка сессии.
// Папка выбирает колорист (по умолчанию Документы\GradeStream), внутри — подпапка сессии
// или файлы прямо в папке с датой в имени.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type snapEntry struct {
	N      int    `json:"n"`
	Time   string `json:"time"`
	Dev    string `json:"dev"`
	Text   string `json:"text"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Marked string `json:"marked"`
}

var (
	snapMu      sync.Mutex
	snapDir     string
	snapEntries []snapEntry
	sessionTime = time.Now()
)

var snapFlat bool // файлы прямо в выбранной папке (без подпапки сессии)

func sessionDir() (string, error) {
	snapMu.Lock()
	defer snapMu.Unlock()
	if snapDir != "" {
		return snapDir, nil
	}
	base, sub := snapSettings()
	d := base
	if sub {
		d = filepath.Join(base, "Сессия "+sessionTime.Format("2006-01-02 15-04"))
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	snapDir, snapFlat = d, !sub
	return d, nil
}

// Имена файлов: в общей папке добавляем дату, чтобы сессии не перезаписывали друг друга
func snapPrefix() string {
	if snapFlat {
		return sessionTime.Format("2006-01-02_15-04_")
	}
	return ""
}
func summaryName(ext string) string {
	if snapFlat {
		return "комментарии " + sessionTime.Format("2006-01-02 15-04") + ext
	}
	return "комментарии" + ext
}

// Смена папки: следующие снимки начнут новую сводку в новом месте
func resetSnapLocation() {
	snapMu.Lock()
	snapDir, snapEntries = "", nil
	snapMu.Unlock()
}

func decodeDataURL(s string) ([]byte, error) {
	if i := strings.Index(s, ","); i >= 0 {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) || r.URL.Query().Get("key") != hostKey || r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Dev    string `json:"dev"`
		Text   string `json:"text"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Marked string `json:"marked"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 96<<20))
	if err == nil {
		err = json.Unmarshal(body, &req)
	}
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	dir, err := sessionDir()
	if err != nil {
		jsonResp(w, map[string]any{"ok": false, "error": "не удалось создать папку: " + err.Error()})
		return
	}
	marked, err1 := decodeDataURL(req.Marked)
	if err1 != nil || len(marked) == 0 {
		jsonResp(w, map[string]any{"ok": false, "error": "повреждённое изображение"})
		return
	}

	snapMu.Lock()
	n := len(snapEntries) + 1
	now := time.Now()
	name := fmt.Sprintf("%s%03d_%s.png", snapPrefix(), n, now.Format("15-04-05"))
	for i := 2; ; i++ { // никогда не перезаписываем существующий снимок
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			break
		}
		name = fmt.Sprintf("%s%03d_%s_%d.png", snapPrefix(), n, now.Format("15-04-05"), i)
	}
	e := snapEntry{N: n, Time: now.Format("15:04:05"), Dev: req.Dev, Text: req.Text, X: req.X, Y: req.Y,
		Marked: name}
	snapEntries = append(snapEntries, e)
	entries := append([]snapEntry(nil), snapEntries...)
	snapMu.Unlock()

	if err := os.WriteFile(filepath.Join(dir, name), marked, 0o644); err != nil {
		jsonResp(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeSummary(dir, entries)
	jsonResp(w, map[string]any{"ok": true, "n": n, "file": name, "dir": dir})
}

func handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) || r.URL.Query().Get("key") != hostKey {
		http.NotFound(w, r)
		return
	}
	snapMu.Lock()
	d := snapDir
	snapMu.Unlock()
	target := d
	if d == "" {
		d, _ = snapSettings()
		os.MkdirAll(d, 0o755)
		target = d
	} else if r.URL.Query().Get("what") == "summary" {
		target = filepath.Join(d, summaryName(".html"))
	}
	openPath(target)
	jsonResp(w, map[string]any{"ok": true, "dir": d})
}

func jsonResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeSummary(dir string, entries []snapEntry) {
	// Текстовая сводка
	var t strings.Builder
	fmt.Fprintf(&t, "GradeStream — комментарии клиента\r\nСессия: %s\r\n\r\n", sessionTime.Format("02.01.2006 15:04"))
	for _, e := range entries {
		c := e.Text
		if c == "" {
			c = "(без комментария)"
		}
		fmt.Fprintf(&t, "№%d  %s  [%s]  точка %d,%d  —  %s\r\n     %s\r\n\r\n", e.N, e.Time, e.Dev, e.X, e.Y, c, e.Marked)
	}
	os.WriteFile(filepath.Join(dir, summaryName(".txt")), append([]byte{0xEF, 0xBB, 0xBF}, t.String()...), 0o644)

	// HTML-сводка со снимками
	var h strings.Builder
	h.WriteString(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Комментарии клиента</title><style>
body{margin:0;background:#111;color:#e8e8e8;font:15px/1.45 "Segoe UI",-apple-system,Roboto,sans-serif}
header{padding:18px 24px;border-bottom:1px solid #2c2c2c}h1{font-size:18px;margin:0}.m{color:#8c8c8c;font-size:13px}
main{padding:20px 24px;display:grid;gap:22px}
.e{display:grid;grid-template-columns:minmax(0,1fr) 320px;gap:16px;background:#1a1a1a;border:1px solid #2c2c2c;border-radius:8px;padding:12px}
@media(max-width:900px){.e{grid-template-columns:1fr}}
.e img{width:100%;display:block;border-radius:4px;background:#000}
.n{font-size:22px;font-weight:600;color:#e0a040}.c{font-size:17px;margin:8px 0 12px;white-space:pre-wrap}
a{color:#e0a040}label{display:flex;gap:8px;align-items:center;margin-top:14px;color:#8c8c8c;cursor:pointer}
.done{opacity:.45}</style></head><body>
<header><h1>Комментарии клиента</h1><div class="m">Сессия `)
	h.WriteString(sessionTime.Format("02.01.2006 15:04"))
	fmt.Fprintf(&h, ` · меток: %d · страница обновляется сама каждые 10 секунд</div></header><main>`, len(entries))
	for _, e := range entries {
		c := e.Text
		if c == "" {
			c = "(без комментария)"
		}
		fmt.Fprintf(&h, `<div class="e" id="e%d"><a href="%s" target="_blank"><img src="%s" loading="lazy"></a><div>
<div class="n">№%d</div><div class="m">%s · %s · точка %d, %d</div><div class="c">%s</div>
<label><input type="checkbox" data-n="%d"> Учтено</label></div></div>`,
			e.N, html.EscapeString(e.Marked), html.EscapeString(e.Marked), e.N, e.Time, html.EscapeString(e.Dev), e.X, e.Y,
			html.EscapeString(c), e.N)
	}
	h.WriteString(`</main><script>
const K='gs-done-'+location.pathname;let d={};try{d=JSON.parse(localStorage.getItem(K)||'{}')}catch{}
document.querySelectorAll('input[data-n]').forEach(cb=>{const n=cb.dataset.n;cb.checked=!!d[n];cb.closest('.e').classList.toggle('done',cb.checked);
cb.onchange=()=>{d[n]=cb.checked;try{localStorage.setItem(K,JSON.stringify(d))}catch{};cb.closest('.e').classList.toggle('done',cb.checked)}});
const y=sessionStorage.getItem('gs-y');if(y)scrollTo(0,+y);
setInterval(()=>{sessionStorage.setItem('gs-y',scrollY);location.reload()},10000);
</script></body></html>`)
	os.WriteFile(filepath.Join(dir, summaryName(".html")), []byte(h.String()), 0o644)
}

// ---------- Папка для скриншотов ----------

func snapFolderState() map[string]any {
	base, sub := snapSettings()
	setMu.Lock()
	custom := cfg.SnapBase != ""
	setMu.Unlock()
	snapMu.Lock()
	cur := snapDir
	snapMu.Unlock()
	return map[string]any{"ok": true, "base": base, "subfolder": sub, "custom": custom, "current": cur}
}

func handleSnapFolder(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) || r.URL.Query().Get("key") != hostKey {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Query().Get("action") {
	case "choose":
		base, _ := snapSettings()
		p, err := pickFolder("Куда сохранять скриншоты меток клиента", base)
		if err == errCancelled {
			jsonResp(w, snapFolderState())
			return
		}
		if err != nil {
			jsonResp(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		// Проверим, что в папку можно писать
		test := filepath.Join(p, ".gradestream-test")
		if err := os.WriteFile(test, []byte("ok"), 0o644); err != nil {
			jsonResp(w, map[string]any{"ok": false, "error": "в эту папку нельзя записывать: " + err.Error()})
			return
		}
		os.Remove(test)
		setMu.Lock()
		cfg.SnapBase = p
		saveSettings()
		setMu.Unlock()
		resetSnapLocation()
	case "reset":
		setMu.Lock()
		cfg.SnapBase = ""
		saveSettings()
		setMu.Unlock()
		resetSnapLocation()
	case "subfolder":
		on := r.URL.Query().Get("on") == "1"
		setMu.Lock()
		cfg.Subfolder = &on
		saveSettings()
		setMu.Unlock()
		resetSnapLocation()
	}
	jsonResp(w, snapFolderState())
}
