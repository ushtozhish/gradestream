//go:build windows

package main

// Захват звука через WASAPI без внешних библиотек:
//  • loopback любого устройства вывода (то, что на нём играет);
//  • process loopback — звук одного приложения и его дочерних процессов (Windows 10 2004+).

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var errUnsupported = errors.New("unknown audio source")

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

func guid(s string) GUID {
	s = strings.Trim(s, "{}")
	var g GUID
	var b [16]byte
	h := strings.ReplaceAll(s, "-", "")
	for i := 0; i < 16; i++ {
		fmt.Sscanf(h[2*i:2*i+2], "%02x", &b[i])
	}
	g.Data1 = binary.BigEndian.Uint32(b[0:4])
	g.Data2 = binary.BigEndian.Uint16(b[4:6])
	g.Data3 = binary.BigEndian.Uint16(b[6:8])
	copy(g.Data4[:], b[8:16])
	return g
}

var (
	clsidMMDeviceEnumerator = guid("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidIMMDeviceEnumerator  = guid("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidIAudioClient         = guid("1CB9AD4C-DBFA-4c32-B178-C2F568A703B2")
	iidIAudioCaptureClient  = guid("C8ADBD64-E71E-48a0-A4DE-185C395CD317")
	iidIUnknown             = guid("00000000-0000-0000-C000-000000000046")
	iidCompletionHandler    = guid("41D949AB-9862-444A-80F6-C261334DA5EB")
	iidIAgileObject         = guid("94EA2B94-E9CC-49E0-C0FF-EE64CA8F5B90")
	pkeyFriendlyNameFmt     = guid("A45C254E-DF1C-4EFD-8020-67D146A850E0")

	ole32             = syscall.NewLazyDLL("ole32.dll")
	procCoInit        = ole32.NewProc("CoInitializeEx")
	procCoUninit      = ole32.NewProc("CoUninitialize")
	procCoCreate      = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree = ole32.NewProc("CoTaskMemFree")
	procPropVarClear  = ole32.NewProc("PropVariantClear")
	mmdevapi          = syscall.NewLazyDLL("mmdevapi.dll")
	procActivateAsync = mmdevapi.NewProc("ActivateAudioInterfaceAsync")
	procCreateEvent   = kernel32.NewProc("CreateEventW")
	procWaitSingle    = kernel32.NewProc("WaitForSingleObject")
)

const (
	clsctxAll         = 0x17
	streamLoopback    = 0x00020000
	streamEventCB     = 0x00040000
	streamAutoConvert = 0x80000000
	streamSrcQuality  = 0x08000000
	bufSilent         = 0x2
)

// Вызов метода COM-объекта по номеру в таблице виртуальных функций
func vcall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func release(obj uintptr) {
	if obj != 0 {
		vcall(obj, 2)
	}
}

func hrErr(r uintptr, what string) error {
	if int32(r) < 0 {
		return fmt.Errorf("%s: 0x%08X", what, uint32(r))
	}
	return nil
}

func comInit() func() {
	runtime.LockOSThread()
	r, _, _ := procCoInit.Call(0, 0) // COINIT_MULTITHREADED
	ok := r == 0 || r == 1
	return func() {
		if ok {
			procCoUninit.Call()
		}
		runtime.UnlockOSThread()
	}
}

func wstr(p uintptr) string {
	if p == 0 {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Pointer(p + uintptr(n*2))) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
}

func newEnumerator() (uintptr, error) {
	var en uintptr
	r, _, _ := procCoCreate.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&en)))
	return en, hrErr(r, "MMDeviceEnumerator")
}

func deviceID(d uintptr) string {
	var p uintptr
	if vcall(d, 5, uintptr(unsafe.Pointer(&p))) != 0 {
		return ""
	}
	s := wstr(p)
	procCoTaskMemFree.Call(p)
	return s
}

func endpointName(d uintptr) string {
	var store uintptr
	if vcall(d, 4, 0, uintptr(unsafe.Pointer(&store))) != 0 {
		return ""
	}
	defer release(store)
	key := struct {
		Fmt GUID
		Pid uint32
	}{pkeyFriendlyNameFmt, 14}
	var pv [24]byte
	if vcall(store, 5, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&pv[0]))) != 0 {
		return ""
	}
	defer procPropVarClear.Call(uintptr(unsafe.Pointer(&pv[0])))
	if binary.LittleEndian.Uint16(pv[0:2]) != 31 { // VT_LPWSTR
		return ""
	}
	return wstr(*(*uintptr)(unsafe.Pointer(&pv[8])))
}

func listAudioDevices() ([]AudioDevice, error) {
	done := comInit()
	defer done()
	en, err := newEnumerator()
	if err != nil {
		return nil, err
	}
	defer release(en)
	defID := ""
	var def uintptr
	if vcall(en, 4, 0 /*eRender*/, 1 /*eMultimedia*/, uintptr(unsafe.Pointer(&def))) == 0 {
		defID = deviceID(def)
		release(def)
	}
	var col uintptr
	if err := hrErr(vcall(en, 3, 0 /*eRender*/, 1 /*ACTIVE*/, uintptr(unsafe.Pointer(&col))), "EnumAudioEndpoints"); err != nil {
		return nil, err
	}
	defer release(col)
	var n uint32
	vcall(col, 3, uintptr(unsafe.Pointer(&n)))
	out := []AudioDevice{}
	for i := uint32(0); i < n; i++ {
		var d uintptr
		if vcall(col, 4, uintptr(i), uintptr(unsafe.Pointer(&d))) != 0 {
			continue
		}
		id := deviceID(d)
		out = append(out, AudioDevice{ID: id, Name: endpointName(d), Default: id == defID})
		release(d)
	}
	return out, nil
}

// Приложения с открытыми окнами (Resolve, плееры, браузеры…)
func listAudioApps() ([]AudioApp, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Process | Where-Object { $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle } | Select-Object @{n='pid';e={$_.Id}}, @{n='name';e={$_.ProcessName}}, @{n='title';e={$_.MainWindowTitle}} | ConvertTo-Json -Compress`)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(L("список приложений: %v", "app list: %v"), err)
	}
	s := strings.TrimSpace(string(out))
	apps := []AudioApp{}
	if s == "" {
		return apps, nil
	}
	if strings.HasPrefix(s, "{") {
		s = "[" + s + "]"
	}
	if err := json.Unmarshal([]byte(s), &apps); err != nil {
		return nil, fmt.Errorf(L("список приложений: %v", "app list: %v"), err)
	}
	filtered := apps[:0]
	for _, a := range apps {
		low := strings.ToLower(a.Name)
		if low == "gradestream" || low == "explorer" || low == "textinputhost" || low == "applicationframehost" {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered, nil
}

// ---------- Формат и чтение пакетов ----------

type wfmt struct {
	tag      uint16
	channels int
	rate     int
	bits     int
	block    int
	float    bool
}

func parseFormat(p uintptr) wfmt {
	b := unsafe.Slice((*byte)(unsafe.Pointer(p)), 40)
	f := wfmt{
		tag:      binary.LittleEndian.Uint16(b[0:]),
		channels: int(binary.LittleEndian.Uint16(b[2:])),
		rate:     int(binary.LittleEndian.Uint32(b[4:])),
		block:    int(binary.LittleEndian.Uint16(b[12:])),
		bits:     int(binary.LittleEndian.Uint16(b[14:])),
	}
	switch f.tag {
	case 3:
		f.float = true
	case 0xFFFE: // WAVE_FORMAT_EXTENSIBLE: SubFormat.Data1 = 3 (float) или 1 (PCM)
		f.float = binary.LittleEndian.Uint32(b[24:]) == 3
	}
	return f
}

// Перевод пакета в стерео float32
func toStereo(f wfmt, data uintptr, frames int, silent bool) []float32 {
	out := make([]float32, frames*2)
	if silent || data == 0 || frames == 0 {
		return out
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(data)), frames*f.block)
	bps := f.bits / 8
	sample := func(fr, ch int) float32 {
		o := fr*f.block + ch*bps
		switch {
		case f.float && bps == 4:
			return math.Float32frombits(binary.LittleEndian.Uint32(raw[o:]))
		case bps == 2:
			return float32(int16(binary.LittleEndian.Uint16(raw[o:]))) / 32768
		case bps == 3:
			v := int32(raw[o]) | int32(raw[o+1])<<8 | int32(int8(raw[o+2]))<<16
			return float32(v) / 8388608
		case bps == 4:
			return float32(int32(binary.LittleEndian.Uint32(raw[o:]))) / 2147483648
		}
		return 0
	}
	for i := 0; i < frames; i++ {
		l := sample(i, 0)
		r := l
		if f.channels > 1 {
			r = sample(i, 1)
		}
		out[2*i], out[2*i+1] = l, r
	}
	return out
}

func drain(capt uintptr, f wfmt, rs *resampler) error {
	for {
		var n uint32
		if err := hrErr(vcall(capt, 5, uintptr(unsafe.Pointer(&n))), "GetNextPacketSize"); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		var data uintptr
		var frames, flags uint32
		if err := hrErr(vcall(capt, 3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)), 0, 0), "GetBuffer"); err != nil {
			return err
		}
		pcm := toStereo(f, data, int(frames), flags&bufSilent != 0)
		vcall(capt, 4, uintptr(frames))
		if out := rs.process(pcm); len(out) > 0 {
			sink.push(out)
		}
	}
}

// ---------- Loopback устройства ----------

func captureDevice(id string, stop, done chan struct{}, ready chan error) {
	defer close(done)
	fin := comInit()
	defer fin()
	fail := func(err error) { ready <- err }

	en, err := newEnumerator()
	if err != nil {
		fail(err)
		return
	}
	defer release(en)
	var dev uintptr
	if id == "" {
		err = hrErr(vcall(en, 4, 0, 1, uintptr(unsafe.Pointer(&dev))), L("устройство по умолчанию", "default device"))
	} else {
		idp, _ := syscall.UTF16PtrFromString(id)
		err = hrErr(vcall(en, 5, uintptr(unsafe.Pointer(idp)), uintptr(unsafe.Pointer(&dev))), L("устройство не найдено", "device not found"))
	}
	if err != nil {
		fail(err)
		return
	}
	defer release(dev)
	var client uintptr
	if err := hrErr(vcall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))), "Activate"); err != nil {
		fail(err)
		return
	}
	defer release(client)
	var pwfx uintptr
	if err := hrErr(vcall(client, 8, uintptr(unsafe.Pointer(&pwfx))), "GetMixFormat"); err != nil {
		fail(err)
		return
	}
	f := parseFormat(pwfx)
	r := vcall(client, 3, 0, streamLoopback, 2000000, 0, pwfx, 0)
	procCoTaskMemFree.Call(pwfx)
	if err := hrErr(r, "Initialize loopback"); err != nil {
		fail(err)
		return
	}
	var capt uintptr
	if err := hrErr(vcall(client, 14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capt))), "GetService"); err != nil {
		fail(err)
		return
	}
	defer release(capt)
	if err := hrErr(vcall(client, 10), "Start"); err != nil {
		fail(err)
		return
	}
	defer vcall(client, 11)
	ready <- nil
	rs := &resampler{inRate: float64(f.rate)}
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if err := drain(capt, f, rs); err != nil {
			hub.sendHost(map[string]any{"t": "audio-state", "ok": false, "error": L("Устройство отключилось: ", "Device disconnected: ") + err.Error()})
			return
		}
	}
}

// ---------- Process loopback (одно приложение) ----------

type completionHandler struct {
	vtbl *[4]uintptr
}

var (
	handlerOnce sync.Once
	handlerVtbl [4]uintptr
	handlerObj  completionHandler
	activateCh  = make(chan uintptr, 1)
)

func guidEq(p uintptr, g *GUID) bool {
	return *(*GUID)(unsafe.Pointer(p)) == *g
}

func initHandler() {
	handlerOnce.Do(func() {
		handlerVtbl[0] = syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
			if guidEq(riid, &iidIUnknown) || guidEq(riid, &iidCompletionHandler) || guidEq(riid, &iidIAgileObject) {
				*(*uintptr)(unsafe.Pointer(ppv)) = this
				return 0
			}
			*(*uintptr)(unsafe.Pointer(ppv)) = 0
			return 0x80004002 // E_NOINTERFACE
		})
		handlerVtbl[1] = syscall.NewCallback(func(this uintptr) uintptr { return 1 })
		handlerVtbl[2] = syscall.NewCallback(func(this uintptr) uintptr { return 1 })
		handlerVtbl[3] = syscall.NewCallback(func(this, op uintptr) uintptr {
			select {
			case activateCh <- op:
			default:
			}
			return 0
		})
		handlerObj.vtbl = &handlerVtbl
	})
}

func captureApp(pid uint32, stop, done chan struct{}, ready chan error) {
	defer close(done)
	fin := comInit()
	defer fin()
	fail := func(err error) { ready <- err }
	if pid == 0 {
		fail(errors.New(L("не выбрано приложение", "no application selected")))
		return
	}
	if procActivateAsync.Find() != nil {
		fail(errors.New(L("звук отдельного приложения требует Windows 10 версии 2004 или новее", "per-application audio requires Windows 10 version 2004 or later")))
		return
	}
	initHandler()
	select { // сбросить старый результат
	case <-activateCh:
	default:
	}

	params := struct {
		ActivationType uint32 // AUDIOCLIENT_ACTIVATION_TYPE_PROCESS_LOOPBACK
		TargetPID      uint32
		Mode           uint32 // INCLUDE_TARGET_PROCESS_TREE
	}{1, pid, 0}
	var pv [24]byte
	binary.LittleEndian.PutUint16(pv[0:], 65) // VT_BLOB
	binary.LittleEndian.PutUint32(pv[8:], uint32(unsafe.Sizeof(params)))
	*(*uintptr)(unsafe.Pointer(&pv[16])) = uintptr(unsafe.Pointer(&params))

	path, _ := syscall.UTF16PtrFromString(`VAD\Process_Loopback`)
	var op uintptr
	r, _, _ := procActivateAsync.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&iidIAudioClient)),
		uintptr(unsafe.Pointer(&pv[0])), uintptr(unsafe.Pointer(&handlerObj)), uintptr(unsafe.Pointer(&op)))
	defer runtime.KeepAlive(&params)
	defer runtime.KeepAlive(&pv)
	if err := hrErr(r, "ActivateAudioInterfaceAsync"); err != nil {
		fail(err)
		return
	}
	defer release(op)
	select {
	case <-activateCh:
	case <-time.After(5 * time.Second):
		fail(errors.New(L("Windows не ответила на запрос захвата звука приложения", "Windows did not respond to the application audio capture request")))
		return
	}
	var hrAct uintptr
	var unk uintptr
	vcall(op, 3, uintptr(unsafe.Pointer(&hrAct)), uintptr(unsafe.Pointer(&unk)))
	if err := hrErr(uintptr(uint32(hrAct)), L("активация звука приложения", "application audio activation")); err != nil {
		fail(err)
		return
	}
	var client uintptr
	r = vcall(unk, 0, uintptr(unsafe.Pointer(&iidIAudioClient)), uintptr(unsafe.Pointer(&client)))
	release(unk)
	if err := hrErr(r, "IAudioClient"); err != nil {
		fail(err)
		return
	}
	defer release(client)

	// Захват в 16 бит 48 кГц стерео (Windows конвертирует сама)
	wfx := [18]byte{}
	binary.LittleEndian.PutUint16(wfx[0:], 1)
	binary.LittleEndian.PutUint16(wfx[2:], 2)
	binary.LittleEndian.PutUint32(wfx[4:], 48000)
	binary.LittleEndian.PutUint32(wfx[8:], 48000*4)
	binary.LittleEndian.PutUint16(wfx[12:], 4)
	binary.LittleEndian.PutUint16(wfx[14:], 16)
	f := wfmt{tag: 1, channels: 2, rate: 48000, bits: 16, block: 4}
	r = vcall(client, 3, 0, streamLoopback|streamEventCB|streamAutoConvert|streamSrcQuality, 2000000, 0, uintptr(unsafe.Pointer(&wfx[0])), 0)
	if err := hrErr(r, "Initialize (application)"); err != nil {
		fail(err)
		return
	}
	evt, _, _ := procCreateEvent.Call(0, 0, 0, 0)
	if evt == 0 {
		fail(errors.New("CreateEvent"))
		return
	}
	defer syscall.CloseHandle(syscall.Handle(evt))
	if err := hrErr(vcall(client, 13, evt), "SetEventHandle"); err != nil {
		fail(err)
		return
	}
	var capt uintptr
	if err := hrErr(vcall(client, 14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capt))), "GetService"); err != nil {
		fail(err)
		return
	}
	defer release(capt)
	if err := hrErr(vcall(client, 10), "Start"); err != nil {
		fail(err)
		return
	}
	defer vcall(client, 11)
	ready <- nil
	rs := &resampler{inRate: 48000}
	for {
		select {
		case <-stop:
			return
		default:
		}
		procWaitSingle.Call(evt, 100)
		if err := drain(capt, f, rs); err != nil {
			hub.sendHost(map[string]any{"t": "audio-state", "ok": false, "error": L("Приложение закрыто или звук недоступен: ", "Application closed or audio unavailable: ") + err.Error()})
			return
		}
	}
}
