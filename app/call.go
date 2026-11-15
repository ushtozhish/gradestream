package main

// Голосовой звонок: ведущий и зрители разговаривают друг с другом поверх трансляции.
//
// Каждый участник с включённым микрофоном шлёт блоки голоса по 20 мс:
//   [5][номер блока, 2 байта BE][уровень 0–255][µ-law, 16 кГц, моно, 320 отсчётов]
// Программа ничего не декодирует и не смешивает — только раздаёт блок всем остальным
// участникам звонка, добавив, от кого он:
//   [5][длина id][id][номер блока][уровень][µ-law]
// Смешивает каждый участник у себя (Web Audio), поэтому никто не слышит самого себя.
// Тишину не передаём: блоки идут, только пока человек говорит, — трафик почти нулевой.

import (
	"encoding/json"
	"strings"
	"sync"
)

const voiceOp = 5

var call = struct {
	sync.Mutex
	on      bool // ведущий включил звонок
	hostIn  bool // ведущий слушает звонок
	hostMic bool // у ведущего включён микрофон
}{}

func callOn() bool {
	call.Lock()
	defer call.Unlock()
	return call.on
}

type callPerson struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Dev  string `json:"dev"`
	Mic  bool   `json:"mic"`
	Lan  bool   `json:"lan,omitempty"`
	Host bool   `json:"host,omitempty"`
}

// Имя участника для меток и списка: введённое при входе в звонок или тип устройства
func (v *Viewer) display() string {
	if v.name != "" {
		return v.name
	}
	return v.devName()
}

func callStateMsg() []byte {
	call.Lock()
	on, hin, hmic := call.on, call.hostIn, call.hostMic
	call.Unlock()
	people := []callPerson{}
	if on && hin {
		people = append(people, callPerson{ID: "host", Name: L("Ведущий", "Host", "Жүргізуші"), Mic: hmic, Host: true})
	}
	hub.mu.Lock()
	for _, v := range hub.viewers {
		if v.inCall.Load() {
			people = append(people, callPerson{ID: v.id, Name: v.name, Dev: v.dev, Mic: v.mic.Load(), Lan: v.lan})
		}
	}
	hub.mu.Unlock()
	secure := ""
	if u, st, _ := tunnelInfo(); u != "" && st == "on" {
		secure = u + "/v/" + room
	}
	b, _ := json.Marshal(map[string]any{"t": "call-state", "on": on, "people": people, "secure": secure})
	return b
}

// Состояние звонка — всем зрителям и ведущему
func broadcastCallState() {
	b := callStateMsg()
	hub.broadcastText(string(b))
	hub.mu.Lock()
	hs := hub.host
	hub.mu.Unlock()
	if hs != nil {
		hs.WriteText(string(b))
	}
}

// Блок голоса от участника from ("host" — ведущий) → остальным участникам звонка
func relayVoice(from string, data []byte) {
	if len(data) < 5 || len(data) > 1400 || data[0] != voiceOp || !callOn() {
		return
	}
	call.Lock()
	hostIn, hostMic := call.hostIn, call.hostMic
	call.Unlock()
	out := make([]byte, 0, len(data)+len(from)+1)
	out = append(out, voiceOp, byte(len(from)))
	out = append(out, from...)
	out = append(out, data[1:]...)

	hub.mu.Lock()
	var to []*Viewer
	allowed := from == "host" && hostIn && hostMic
	for _, v := range hub.viewers {
		if v.id == from {
			allowed = v.inCall.Load() && v.mic.Load()
			continue
		}
		if v.inCall.Load() {
			to = append(to, v)
		}
	}
	hs := hub.host
	hub.mu.Unlock()
	if !allowed {
		return
	}
	for _, v := range to {
		if !v.enqueue(outMsg{op: opBinary, data: out}, false) {
			v.drops.Add(1)
		}
	}
	if from != "host" && hostIn && hs != nil {
		hs.WriteBinary(out)
	}
}

func cleanName(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == '<' || r == '>' {
			return -1
		}
		return r
	}, s))
	if r := []rune(s); len(r) > 32 {
		s = string(r[:32])
	}
	return s
}

// Команды зрителя: войти/выйти из звонка, микрофон
func viewerCallCommand(v *Viewer, m map[string]any) {
	switch m["t"] {
	case "call-join":
		on, _ := m["on"].(bool)
		if on && !callOn() {
			on = false
		}
		if n, ok := m["name"].(string); ok {
			hub.mu.Lock()
			v.name = cleanName(n)
			hub.mu.Unlock()
		}
		v.inCall.Store(on)
		if !on {
			v.mic.Store(false)
		}
	case "call-mic":
		on, _ := m["on"].(bool)
		v.mic.Store(on && v.inCall.Load())
	default:
		return
	}
	broadcastCallState()
	sendState()
}

// Команды ведущего: включить звонок, свой микрофон, выключить микрофон зрителю или всем
func hostCallCommand(m map[string]any) {
	switch m["t"] {
	case "call":
		on, _ := m["on"].(bool)
		call.Lock()
		call.on, call.hostIn = on, on
		if !on {
			call.hostMic = false
		}
		call.Unlock()
		if !on {
			hub.mu.Lock()
			for _, v := range hub.viewers {
				v.inCall.Store(false)
				v.mic.Store(false)
			}
			hub.mu.Unlock()
		}
	case "call-mic":
		on, _ := m["on"].(bool)
		call.Lock()
		call.hostMic = on && call.on
		call.Unlock()
	case "call-mute":
		id, _ := m["id"].(string)
		b, _ := json.Marshal(map[string]any{"t": "call-mic-off"})
		hub.mu.Lock()
		var vs []*Viewer
		for _, v := range hub.viewers {
			if (id == "" || v.id == id) && v.mic.Load() {
				v.mic.Store(false)
				vs = append(vs, v)
			}
		}
		hub.mu.Unlock()
		for _, v := range vs {
			v.enqueue(outMsg{op: opText, data: b}, true)
		}
	default:
		return
	}
	broadcastCallState()
	sendState()
}
