package main

// GradeStream — трансляция Clean Feed клиенту на телефон.
// Захват и кодирование делает окно браузера (Edge/Chrome) через WebCodecs.
// Через интернет: видео идёт зрителям по WebSocket через туннель Cloudflare.
// Внутри сети: телефоны в той же Wi-Fi получают видео по WebRTC напрямую (lan.go).

import (
	"crypto/rand"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const appVersion = "2.12.0"

//go:embed web/*
var webFS embed.FS

var (
	hostKey = randHex(16)
	room    = randHex(5)
	port    int
	hub     = &Hub{viewers: map[string]*Viewer{}}
	dataDir string
)

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------- Хаб ----------------

type outMsg struct {
	op   byte
	data []byte
	at   time.Time // момент постановки в очередь (для тестовой имитации сети)
}

type Viewer struct {
	id        string
	dev       string
	ws        *WSConn
	ch        chan outMsg
	done      chan struct{}
	needKey   atomic.Bool
	bytes     atomic.Int64
	drops     atomic.Int64
	kbps      int64
	closed    atomic.Bool
	pcm       atomic.Bool // нет декодера Opus — получает облегчённый PCM
	wantAudio atomic.Bool // зритель включил звук
	lan       bool        // подключён по локальной сети — видео идёт по WebRTC
	rtc       *lanPeer
	rtcBytes  int64

	lastDeliver time.Time // для тестовой имитации сети
}

// GS_TEST_JITTER_MS — только для замеров: случайная задержка доставки зрителям 0…N мс
var testJitterUs = func() int64 { n, _ := strconv.Atoi(os.Getenv("GS_TEST_JITTER_MS")); return int64(n) * 1000 }()

type Hub struct {
	mu         sync.Mutex
	host       *WSConn
	viewers    map[string]*Viewer
	lastConfig string
	lastAudio  string
	lastPlay   string // режим подачи у зрителей: live | sync
	lastScopes string // разрешены ли зрителям скопы (waveform, парад, вектороскоп)
	live       bool
	lastKFReq  time.Time
	hostSeen   time.Time
	everHost   bool
}

func (h *Hub) sendHost(v any) {
	h.mu.Lock()
	hs := h.host
	h.mu.Unlock()
	if hs != nil {
		b, _ := json.Marshal(v)
		hs.WriteText(string(b))
	}
}

func (h *Hub) sendHostBinary(b []byte) {
	h.mu.Lock()
	hs := h.host
	h.mu.Unlock()
	if hs != nil {
		hs.WriteBinary(b)
	}
}

func (h *Hub) anyPCMViewer() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, v := range h.viewers {
		if !v.lan && v.pcm.Load() && v.wantAudio.Load() {
			return true
		}
	}
	return false
}

// Звук: Opus (3) — зрителям с декодером, PCM (4) — остальным; только тем, кто включил звук
func (h *Hub) broadcastAudio(data []byte, pcm bool) {
	h.mu.Lock()
	vs := make([]*Viewer, 0, len(h.viewers))
	for _, v := range h.viewers {
		if !v.lan && v.wantAudio.Load() && v.pcm.Load() == pcm {
			vs = append(vs, v)
		}
	}
	h.mu.Unlock()
	for _, v := range vs {
		if !v.enqueue(outMsg{op: opBinary, data: data}, false) {
			v.drops.Add(1)
		}
	}
	if !pcm {
		lanAudioPacket(data)
	}
}

func (h *Hub) requestKeyframe() {
	h.mu.Lock()
	if time.Since(h.lastKFReq) < 700*time.Millisecond {
		h.mu.Unlock()
		return
	}
	h.lastKFReq = time.Now()
	h.mu.Unlock()
	h.sendHost(map[string]any{"t": "kf"})
}

func (h *Hub) broadcastText(s string) {
	h.mu.Lock()
	vs := make([]*Viewer, 0, len(h.viewers))
	for _, v := range h.viewers {
		vs = append(vs, v)
	}
	h.mu.Unlock()
	for _, v := range vs {
		v.enqueue(outMsg{op: opText, data: []byte(s)}, true)
	}
}

func (v *Viewer) enqueue(m outMsg, important bool) bool {
	if v.closed.Load() {
		return false
	}
	if testJitterUs > 0 {
		m.at = time.Now()
	}
	select {
	case v.ch <- m:
		return true
	default:
		if important {
			// Для служебных сообщений ждём немного места
			select {
			case v.ch <- m:
				return true
			case <-time.After(200 * time.Millisecond):
			}
		}
		return false
	}
}

// Видео-кадр: [1 байт: 1=ключевой, 0=обычный][8 байт timestamp][данные H.264]
func (h *Hub) broadcastFrame(data []byte) {
	if len(data) < 9 {
		return
	}
	isKey := data[0] == 1
	h.mu.Lock()
	vs := make([]*Viewer, 0, len(h.viewers))
	for _, v := range h.viewers {
		if !v.lan {
			vs = append(vs, v)
		}
	}
	h.mu.Unlock()
	lanVideoFrame(data)
	needKF := false
	for _, v := range vs {
		if v.needKey.Load() && !isKey {
			needKF = true
			continue
		}
		// Если очередь зрителя растёт (медленная сеть) — сбрасываем до следующего ключевого кадра
		if !isKey && len(v.ch) > cap(v.ch)/2 {
			v.needKey.Store(true)
			v.drops.Add(1)
			needKF = true
			continue
		}
		if v.enqueue(outMsg{op: opBinary, data: data}, false) {
			if isKey {
				v.needKey.Store(false)
			}
		} else {
			v.needKey.Store(true)
			v.drops.Add(1)
			needKF = true
		}
	}
	if needKF {
		h.requestKeyframe()
	}
}

func (v *Viewer) writer() {
	for {
		var m outMsg
		select {
		case m = <-v.ch:
		case <-v.done:
			return
		}
		if testJitterUs > 0 { // только для замеров: имитация неровной сети (порядок сохраняется)
			at := m.at.Add(time.Duration(mrand.Int64N(testJitterUs)) * time.Microsecond)
			if at.Before(v.lastDeliver) {
				at = v.lastDeliver
			}
			v.lastDeliver = at
			time.Sleep(time.Until(at))
		}
		var err error
		if m.op == opBinary {
			err = v.ws.WriteBinary(m.data)
			v.bytes.Add(int64(len(m.data)))
		} else if m.op == opPing {
			err = v.ws.Ping()
		} else {
			err = v.ws.WriteText(string(m.data))
		}
		if err != nil {
			v.ws.Close()
			return
		}
	}
}

// ---------------- HTTP ----------------

func isLocal(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	// cloudflared тоже приходит с 127.0.0.1, но добавляет эти заголовки
	return r.Header.Get("Cf-Connecting-Ip") == "" && r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("Cf-Ray") == ""
}

func page(w http.ResponseWriter, name string, repl map[string]string) {
	b, err := webFS.ReadFile("web/" + name)
	if err != nil {
		http.Error(w, "missing", 500)
		return
	}
	s := string(b)
	for k, v := range repl {
		s = strings.ReplaceAll(s, k, v)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, s)
}

func deviceName(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "Macintosh"):
		return "Mac"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	}
	return L("устройство", "device")
}

func routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/__gs_ping", func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "gradestream "+hostKey)
	})

	mux.HandleFunc("/host", func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) || r.URL.Query().Get("key") != hostKey {
			http.NotFound(w, r)
			return
		}
		page(w, "host.html", map[string]string{"__HOST_KEY__": hostKey, "__VERSION__": appVersion})
	})

	mux.HandleFunc("/v/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/v/") != room {
			w.WriteHeader(404)
			page(w, "gone.html", nil)
			return
		}
		page(w, "view.html", map[string]string{"__ROOM__": room, "__LAN__": "false"})
	})

	mux.HandleFunc("/icon.png", serveIcon)
	mux.HandleFunc("/api/qr", handleQR)
	mux.HandleFunc("/api/snapshot", handleSnapshot)
	mux.HandleFunc("/api/open-folder", handleOpenFolder)
	mux.HandleFunc("/api/snap-folder", handleSnapFolder)

	mux.HandleFunc("/ws/host", func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) || r.URL.Query().Get("key") != hostKey {
			http.NotFound(w, r)
			return
		}
		ws, err := wsUpgrade(w, r)
		if err != nil {
			return
		}
		hub.mu.Lock()
		if hub.host != nil {
			hub.host.Close()
		}
		hub.host = ws
		hub.everHost = true
		hub.hostSeen = time.Now()
		hub.mu.Unlock()
		log.Println("host connected")
		sendState()
		for {
			op, data, err := ws.ReadMessage()
			if err != nil {
				break
			}
			hub.mu.Lock()
			hub.hostSeen = time.Now()
			hub.mu.Unlock()
			if op == opBinary {
				if len(data) > 0 && data[0] <= 1 {
					hub.broadcastFrame(data)
				} else if len(data) > 0 && data[0] == 3 {
					hub.broadcastAudio(data, false)
				}
				continue
			}
			var m map[string]any
			if json.Unmarshal(data, &m) != nil {
				continue
			}
			switch m["t"] {
			case "config":
				if d, ok := m["description"].(string); ok {
					setH264Config(d)
				}
				hub.mu.Lock()
				hub.lastConfig = string(data)
				hub.live = true
				for _, v := range hub.viewers {
					v.needKey.Store(true)
				}
				hub.mu.Unlock()
				hub.broadcastText(string(data))
			case "live":
				on, _ := m["on"].(bool)
				hub.mu.Lock()
				hub.live = on
				if !on {
					hub.lastConfig = ""
				}
				hub.mu.Unlock()
				hub.broadcastText(string(data))
			case "pause":
				hub.broadcastText(string(data))
			case "audio":
				hub.mu.Lock()
				hub.lastAudio = string(data)
				hub.mu.Unlock()
				hub.broadcastText(string(data))
			case "clock": // сверка часов окна трансляции с часами программы
				b, _ := json.Marshal(map[string]any{"t": "clock", "c": m["c"], "s": nowUs()})
				hub.mu.Lock()
				hs := hub.host
				hub.mu.Unlock()
				if hs != nil {
					hs.WriteText(string(b))
				}
			case "playout":
				hub.mu.Lock()
				hub.lastPlay = string(data)
				hub.mu.Unlock()
				hub.broadcastText(string(data))
			case "scopes":
				hub.mu.Lock()
				hub.lastScopes = string(data)
				hub.mu.Unlock()
				hub.broadcastText(string(data))
			case "diag": // что умеет браузер окна трансляции — для разбора проблем
				log.Println("host browser:", string(data))
			case "lang":
				lg, _ := m["lang"].(string)
				setLang(lg)
			case "mode":
				md, _ := m["mode"].(string)
				go setMode(md)
			case "audio-list", "audio-select":
				handleAudioCommand(m)
			case "monitors":
				hub.sendHost(map[string]any{"t": "monitors", "list": listMonitors()})
			case "overlay-mark":
				mon, _ := m["monitor"].(float64)
				x, _ := m["x"].(float64)
				y, _ := m["y"].(float64)
				text, _ := m["text"].(string)
				ttl, _ := m["ttl"].(float64)
				if ttl <= 0 {
					ttl = 20
				}
				if err := overlayShow(int(mon), int32(x), int32(y), text, time.Duration(ttl*float64(time.Second))); err != nil {
					hub.sendHost(map[string]any{"t": "overlay-error", "error": err.Error()})
				}
			case "overlay-clear":
				overlayClear()
			case "hidden":
				// Окно свернули во время эфира — один раз подскажем, где программа
				if !hiddenHintShown {
					hiddenHintShown = true
					trayNotify(L("Окно свёрнуто. Вернуть его — клик по значку GradeStream возле часов. Остановить или выйти — правый клик по значку.", "The window is minimized. Click the GradeStream icon near the clock to bring it back; right-click it to stop or exit."))
				}
			case "ping":
			}
		}
		hub.mu.Lock()
		if hub.host == ws {
			hub.host = nil
		}
		hub.mu.Unlock()
		ws.Close()
		log.Println("host disconnected")
		go func() {
			time.Sleep(5 * time.Second)
			hub.mu.Lock()
			reconnected := hub.host != nil
			hub.mu.Unlock()
			if !reconnected {
				stopAudio()
			}
		}()
	})

	mux.HandleFunc("/ws/view", func(w http.ResponseWriter, r *http.Request) { viewerWS(w, r, false) })

	return mux
}

func sendState() {
	type vInfo struct {
		ID    string `json:"id"`
		Dev   string `json:"dev"`
		Kbps  int64  `json:"kbps"`
		Drops int64  `json:"drops"`
		Queue int    `json:"queue"`
		Audio bool   `json:"audio"`
		PCM   bool   `json:"pcm"`
		Lan   bool   `json:"lan"`
		Rtc   bool   `json:"rtc"`
	}
	hub.mu.Lock()
	list := []vInfo{}
	for _, v := range hub.viewers {
		list = append(list, vInfo{v.id, v.dev, v.kbps, v.drops.Load(), len(v.ch), v.wantAudio.Load(), v.pcm.Load(), v.lan, v.rtc != nil && v.rtc.connected()})
	}
	live := hub.live && hub.host != nil
	hub.mu.Unlock()
	setTrayState(live, len(list))
	url, st, errText := tunnelInfo()
	link := ""
	if url != "" {
		link = url + "/v/" + room
	}
	lanL, lanE, lanW := lanState()
	hub.sendHost(map[string]any{"t": "state", "viewers": list, "link": link, "tunnel": st, "tunnelError": errText,
		"mode": currentMode(), "lanLinks": lanL, "lanError": lanE, "lanWarn": lanW, "lang": currentLang(), "langPref": langPref()})
}

func tickers() {
	statT := time.NewTicker(2 * time.Second)
	pingT := time.NewTicker(20 * time.Second)
	for {
		select {
		case <-statT.C:
			hub.mu.Lock()
			for _, v := range hub.viewers {
				b := v.bytes.Swap(0)
				if v.rtc != nil {
					n := v.rtc.bytesSent()
					b, v.rtcBytes = n-v.rtcBytes, n
				}
				v.kbps = b * 8 / 2000
			}
			hub.mu.Unlock()
			sendState()
		case <-pingT.C:
			hub.mu.Lock()
			for _, v := range hub.viewers {
				v.enqueue(outMsg{op: opPing, data: nil}, false)
			}
			if hub.host != nil {
				hub.host.Ping()
			}
			hub.mu.Unlock()
		}
	}
}

// ---------------- Запуск ----------------

func main() {
	setDPIAware()
	setProcessAppID()
	dataDir = appDataDir()
	os.MkdirAll(dataDir, 0o755)
	loadSettings()
	applyLang(langPref())
	if f, err := os.OpenFile(filepath.Join(dataDir, "log.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
	log.Printf("GradeStream %s start", appVersion)

	// Уже запущено? Тогда просто показываем окно существующей копии.
	if key := existingInstance(8787); key != "" {
		openHostWindow(fmt.Sprintf("http://127.0.0.1:%d/host?key=%s", 8787, key), true)
		return
	}

	var ln net.Listener
	var err error
	for p := 8787; p < 8800; p++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			port = p
			break
		}
	}
	if ln == nil {
		fatal(L("Не удалось открыть локальный порт: ", "Could not open a local port: ") + err.Error())
	}
	log.Printf("listening on 127.0.0.1:%d, room %s", port, room)

	srv := &http.Server{Handler: routes()}
	go srv.Serve(ln)
	go tickers()
	applyMode(currentMode())

	hostURL := fmt.Sprintf("http://127.0.0.1:%d/host?key=%s", port, hostKey)
	startTray(
		func() { // показать окно (или открыть заново, если его закрыли)
			if !showHostWindow() {
				openHostWindow(hostURL, true)
			}
		},
		func() { hub.sendHost(map[string]any{"t": "cmd", "action": "stop"}) },
		func() { shutdown() },
	)
	startBranding()
	browserExited := make(chan struct{})
	go func() {
		openHostWindow(hostURL, false)
		close(browserExited)
	}()

	// Завершение: окно закрыто, либо окно трансляции не на связи 30 секунд.
	start := time.Now()
	for {
		select {
		case <-browserExited:
			// Если браузер вернулся мгновенно (передал окно уже открытому процессу),
			// ориентируемся на связь со страницей.
			if time.Since(start) > 8*time.Second {
				shutdown()
				return
			}
			browserExited = nil
		case <-time.After(2 * time.Second):
		}
		hub.mu.Lock()
		ever, connected, seen := hub.everHost, hub.host != nil, hub.hostSeen
		hub.mu.Unlock()
		if ever && !connected && time.Since(seen) > 10*time.Second {
			shutdown()
			return
		}
		if !ever && time.Since(start) > 3*time.Minute {
			shutdown()
			return
		}
	}
}

func existingInstance(p int) string {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/__gs_ping", p))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	s := string(b)
	if strings.HasPrefix(s, "gradestream ") {
		return strings.TrimPrefix(s, "gradestream ")
	}
	return ""
}

var hiddenHintShown bool

func shutdown() {
	log.Println("shutdown")
	hub.broadcastText(`{"t":"live","on":false}`)
	removeTray()
	killBrowser()
	stopAudio()
	stopTunnel()
	stopLAN()
	os.Exit(0)
}

func fatal(msg string) {
	log.Println("FATAL:", msg)
	showError(msg)
	stopTunnel()
	os.Exit(1)
}

func viewerWS(w http.ResponseWriter, r *http.Request, lan bool) {
	if r.URL.Query().Get("room") != room {
		http.NotFound(w, r)
		return
	}
	ws, err := wsUpgrade(w, r)
	if err != nil {
		return
	}
	v := &Viewer{id: randHex(4), dev: deviceName(r.UserAgent()), ws: ws, ch: make(chan outMsg, 120), done: make(chan struct{}), lan: lan}
	v.needKey.Store(true)
	go v.writer()

	hub.mu.Lock()
	hub.viewers[v.id] = v
	cfg, live, acfg, pcfg, scfg := hub.lastConfig, hub.live, hub.lastAudio, hub.lastPlay, hub.lastScopes
	hub.mu.Unlock()
	log.Printf("viewer %s (%s) joined", v.id, v.dev)

	hello, _ := json.Marshal(map[string]any{"t": "hello", "id": v.id, "live": live})
	v.enqueue(outMsg{op: opText, data: hello}, true)
	if cfg != "" {
		v.enqueue(outMsg{op: opText, data: []byte(cfg)}, true)
		hub.requestKeyframe()
	}
	if acfg != "" {
		v.enqueue(outMsg{op: opText, data: []byte(acfg)}, true)
	}
	if pcfg != "" {
		v.enqueue(outMsg{op: opText, data: []byte(pcfg)}, true)
	}
	if scfg != "" {
		v.enqueue(outMsg{op: opText, data: []byte(scfg)}, true)
	}
	sendState()

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var m map[string]any
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m["t"] {
		case "rtc-start":
			if !v.lan {
				break
			}
			p, err := newLanPeer(v)
			if err != nil {
				b, _ := json.Marshal(map[string]any{"t": "rtc-error", "error": err.Error()})
				v.enqueue(outMsg{op: opText, data: b}, true)
				break
			}
			hub.mu.Lock()
			old := v.rtc
			v.rtc, v.rtcBytes = p, p.bytesSent()
			hub.mu.Unlock()
			if old != nil {
				old.close()
			}
		case "rtc-answer":
			hub.mu.Lock()
			p := v.rtc
			hub.mu.Unlock()
			if sdp, _ := m["sdp"].(string); p != nil && sdp != "" {
				if err := p.answer(sdp); err != nil {
					log.Println("rtc answer:", err)
				}
			}
		case "rtc-ice":
			hub.mu.Lock()
			p := v.rtc
			hub.mu.Unlock()
			if p != nil {
				raw, _ := json.Marshal(m["c"])
				p.addICE(raw)
			}
		case "kf":
			v.needKey.Store(true)
			hub.requestKeyframe()
		case "caps":
			opus, _ := m["opus"].(bool)
			v.pcm.Store(!opus)
		case "audio-want":
			on, _ := m["on"].(bool)
			v.wantAudio.Store(on)
			sendState()
		case "mark":
			m["from"] = v.id
			m["dev"] = v.dev
			if s, ok := m["text"].(string); ok && len([]rune(s)) > 300 {
				m["text"] = string([]rune(s)[:300])
			}
			hub.sendHost(m)
		}
	}
	v.closed.Store(true)
	hub.mu.Lock()
	delete(hub.viewers, v.id)
	rtcPeer := v.rtc
	v.rtc = nil
	hub.mu.Unlock()
	if rtcPeer != nil {
		rtcPeer.close()
	}
	close(v.done)
	ws.Close()
	log.Printf("viewer %s left", v.id)
	sendState()
}

func serveIcon(w http.ResponseWriter, r *http.Request) {
	b, _ := webFS.ReadFile("web/icon.png")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(b)
}

func mathFloat64(b []byte) float64 { return math.Float64frombits(binary.BigEndian.Uint64(b)) }
