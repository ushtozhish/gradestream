package main

// Звук: захват (WASAPI на Windows) → PCM 48 кГц стерео float32 →
// окно трансляции кодирует в Opus → зрителям.
// Для телефонов без декодера Opus сервер сам отправляет облегчённый PCM 24 кГц.

import (
	"encoding/binary"

	"log"
	"math"
	"sync"
	"time"
)

const audioRate = 48000

type AudioDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

type AudioApp struct {
	PID   uint32 `json:"pid"`
	Name  string `json:"name"`
	Title string `json:"title"`
}

// ---------- Пересэмплер в 48 кГц стерео ----------

type resampler struct {
	inRate float64
	pos    float64 // позиция следующего выходного сэмпла; 0 = последний сэмпл прошлого блока
	lastL  float32
	lastR  float32
	primed bool
}

// in: стерео interleaved float32 с частотой inRate → 48 кГц
func (r *resampler) process(in []float32) []float32 {
	if r.inRate == audioRate {
		return in
	}
	n := len(in) / 2
	if n == 0 {
		return nil
	}
	if !r.primed {
		r.lastL, r.lastR = in[0], in[1]
		r.pos = 1
		r.primed = true
	}
	ext := func(k int) (float32, float32) {
		if k == 0 {
			return r.lastL, r.lastR
		}
		return in[2*(k-1)], in[2*(k-1)+1]
	}
	step := r.inRate / audioRate
	out := make([]float32, 0, int(float64(n)/step+4)*2)
	for {
		i := int(r.pos)
		if i+1 > n {
			break
		}
		f := float32(r.pos - float64(i))
		l0, r0 := ext(i)
		l1, r1 := ext(i + 1)
		out = append(out, l0+(l1-l0)*f, r0+(r1-r0)*f)
		r.pos += step
	}
	r.pos -= float64(n)
	r.lastL, r.lastR = in[2*(n-1)], in[2*(n-1)+1]
	return out
}

// ---------- Раздача PCM ----------

type audioSink struct {
	mu  sync.Mutex
	buf []float32 // 48 кГц стерео interleaved
}

var sink = &audioSink{}

const blockFrames = 960 // 20 мс

// Принимает уже 48 кГц стерео.
func (s *audioSink) push(pcm []float32) {
	s.mu.Lock()
	s.buf = append(s.buf, pcm...)
	var blocks [][]float32
	for len(s.buf) >= blockFrames*2 {
		b := make([]float32, blockFrames*2)
		copy(b, s.buf)
		s.buf = s.buf[blockFrames*2:]
		blocks = append(blocks, b)
	}
	s.mu.Unlock()
	for _, b := range blocks {
		sendPCMBlock(b)
	}
}

func (s *audioSink) reset() {
	s.mu.Lock()
	s.buf = nil
	s.mu.Unlock()
}

// В окно трансляции: [2][8 байт резерв][float32 LE interleaved]
// Зрителям без Opus: [4][8 байт резерв][int16 LE interleaved, 24 кГц стерео]
func sendPCMBlock(b []float32) {
	msg := make([]byte, 9+len(b)*4)
	msg[0] = 2
	for i, v := range b {
		binary.LittleEndian.PutUint32(msg[9+i*4:], math.Float32bits(v))
	}
	hub.sendHostBinary(msg)

	if !hub.anyPCMViewer() {
		return
	}
	frames := len(b) / 2 / 2
	p := make([]byte, 9+frames*4)
	p[0] = 4
	for i := 0; i < frames; i++ {
		l := (b[4*i] + b[4*i+2]) / 2
		r := (b[4*i+1] + b[4*i+3]) / 2
		binary.LittleEndian.PutUint16(p[9+i*4:], uint16(f2s(l)))
		binary.LittleEndian.PutUint16(p[11+i*4:], uint16(f2s(r)))
	}
	hub.broadcastAudio(p, true)
}

func f2s(v float32) int16 {
	if v > 1 {
		v = 1
	} else if v < -1 {
		v = -1
	}
	return int16(v * 32767)
}

// ---------- Управление источником ----------

var (
	audMu     sync.Mutex
	audStop   chan struct{}
	audDone   chan struct{}
	audSource string
)

func stopAudio() {
	audMu.Lock()
	st, dn := audStop, audDone
	audStop, audDone, audSource = nil, nil, ""
	audMu.Unlock()
	if st != nil {
		close(st)
		select {
		case <-dn:
		case <-time.After(2 * time.Second):
		}
	}
	sink.reset()
}

// kind: none | device | app | test
func selectAudio(kind, id string, pid uint32) (string, error) {
	stopAudio()
	if kind == "none" || kind == "" {
		return "без звука", nil
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	ready := make(chan error, 1)
	var desc string
	switch kind {
	case "test":
		desc = "тестовый сигнал 1 кГц"
		go testTone(stop, done, ready)
	case "device":
		desc = "устройство вывода"
		go captureDevice(id, stop, done, ready)
	case "app":
		desc = "приложение"
		go captureApp(pid, stop, done, ready)
	default:
		return "", errUnsupported
	}
	if err := <-ready; err != nil {
		<-done
		return "", err
	}
	audMu.Lock()
	audStop, audDone, audSource = stop, done, kind
	audMu.Unlock()
	log.Printf("audio started: %s %s %d", kind, id, pid)
	return desc, nil
}

// Синус 1 кГц, −18 dBFS, с паузами — проверить, что звук доходит до клиента
func testTone(stop, done chan struct{}, ready chan error) {
	defer close(done)
	ready <- nil
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	phase := 0.0
	n := 0
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		b := make([]float32, blockFrames*2)
		on := (n/25)%2 == 0 // полсекунды тон, полсекунды тишина
		for i := 0; i < blockFrames; i++ {
			v := float32(0)
			if on {
				v = float32(0.125 * math.Sin(phase))
			}
			phase += 2 * math.Pi * 1000 / audioRate
			b[2*i], b[2*i+1] = v, v
		}
		n++
		sink.push(b)
	}
}

func handleAudioCommand(m map[string]any) {
	switch m["t"] {
	case "audio-list":
		devs, derr := listAudioDevices()
		apps, aerr := listAudioApps()
		resp := map[string]any{"t": "audio-list", "devices": devs, "apps": apps}
		if derr != nil {
			resp["deviceError"] = derr.Error()
		}
		if aerr != nil {
			resp["appError"] = aerr.Error()
		}
		hub.sendHost(resp)
	case "audio-select":
		kind, _ := m["kind"].(string)
		id, _ := m["id"].(string)
		pidF, _ := m["pid"].(float64)
		go func() {
			desc, err := selectAudio(kind, id, uint32(pidF))
			st := map[string]any{"t": "audio-state", "kind": kind, "ok": err == nil, "desc": desc}
			if err != nil {
				st["error"] = err.Error()
				log.Println("audio error:", err)
			}
			hub.sendHost(st)
		}()
	}
}
