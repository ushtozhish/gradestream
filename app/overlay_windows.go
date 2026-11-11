//go:build windows

package main

// Метки клиента поверх монитора с Clean Feed.
// Прозрачное окно «поверх всех», сквозь которое проходят клики, и которое
// Windows исключает из захвата экрана (WDA_EXCLUDEFROMCAPTURE, Windows 10 2004+):
// колорист видит метки прямо на картинке, клиенту они не уходят.

import (
	"errors"
	"log"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
	shcore               = syscall.NewLazyDLL("shcore.dll")
	pRegisterClassEx     = user32.NewProc("RegisterClassExW")
	pCreateWindowEx      = user32.NewProc("CreateWindowExW")
	pDefWindowProc       = user32.NewProc("DefWindowProcW")
	pGetMessage          = user32.NewProc("GetMessageW")
	pTranslateMessage    = user32.NewProc("TranslateMessage")
	pDispatchMessage     = user32.NewProc("DispatchMessageW")
	pPostMessage         = user32.NewProc("PostMessageW")
	pShowWindow          = user32.NewProc("ShowWindow")
	pSetWindowPos        = user32.NewProc("SetWindowPos")
	pSetLayeredAttrs     = user32.NewProc("SetLayeredWindowAttributes")
	pSetDisplayAffinity  = user32.NewProc("SetWindowDisplayAffinity")
	pBeginPaint          = user32.NewProc("BeginPaint")
	pEndPaint            = user32.NewProc("EndPaint")
	pFillRect            = user32.NewProc("FillRect")
	pDrawText            = user32.NewProc("DrawTextW")
	pInvalidateRect      = user32.NewProc("InvalidateRect")
	pSetTimer            = user32.NewProc("SetTimer")
	pEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	pSetDpiCtx           = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
	pCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	pCreatePen           = gdi32.NewProc("CreatePen")
	pCreateFont          = gdi32.NewProc("CreateFontW")
	pSelectObject        = gdi32.NewProc("SelectObject")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
	pGetStockObject      = gdi32.NewProc("GetStockObject")
	pEllipse             = gdi32.NewProc("Ellipse")
	pMoveTo              = gdi32.NewProc("MoveToEx")
	pLineTo              = gdi32.NewProc("LineTo")
	pSetBkMode           = gdi32.NewProc("SetBkMode")
	pSetTextColor        = gdi32.NewProc("SetTextColor")
	pGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	_                    = shcore
)

const (
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wmPaint         = 0x000F
	wmTimer         = 0x0113
	wmApp           = 0x8000
	keyColor        = 0x00FF00FF // пурпурный — прозрачный фон (COLORREF 0x00BBGGRR)
	wdaExcludeCap   = 0x11
)

type Monitor struct {
	Index   int    `json:"index"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	W       int32  `json:"w"`
	H       int32  `json:"h"`
	Primary bool   `json:"primary"`
	Name    string `json:"name"`
}

type overlayMark struct {
	x, y    int32 // в пикселях монитора
	text    string
	expires time.Time
}

var (
	ovMu      sync.Mutex
	ovHwnd    uintptr
	ovMon     *Monitor
	ovMarks   []overlayMark
	ovReady   = make(chan error, 1)
	ovOnce    sync.Once
	ovExclude bool
)

func setDPIAware() {
	// Координаты мониторов в реальных пикселях — как у захвата экрана
	if pSetDpiCtx.Find() == nil {
		if r, _, _ := pSetDpiCtx.Call(^uintptr(3)); r != 0 { // PER_MONITOR_AWARE_V2 = -4
			return
		}
	}
	pSetProcessDPIAware.Call()
}

var (
	monMu    sync.Mutex
	monList  []Monitor
	monCB    uintptr
	monCBOne sync.Once
)

func monitorEnumProc(hmon, hdc, lprc, lparam uintptr) uintptr {
	var mi struct {
		Size    uint32
		Monitor [4]int32
		Work    [4]int32
		Flags   uint32
		Device  [32]uint16
	}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := pGetMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&mi))); r != 0 {
		monList = append(monList, Monitor{
			Index: len(monList), X: mi.Monitor[0], Y: mi.Monitor[1],
			W: mi.Monitor[2] - mi.Monitor[0], H: mi.Monitor[3] - mi.Monitor[1],
			Primary: mi.Flags&1 != 0, Name: syscall.UTF16ToString(mi.Device[:]),
		})
	}
	return 1
}

func listMonitors() []Monitor {
	monCBOne.Do(func() { monCB = syscall.NewCallback(monitorEnumProc) })
	monMu.Lock()
	defer monMu.Unlock()
	monList = nil
	pEnumDisplayMonitors.Call(0, 0, monCB, 0)
	return append([]Monitor(nil), monList...)
}

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

func overlayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps [72]byte
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps[0])))
		ovMu.Lock()
		mon := ovMon
		marks := append([]overlayMark(nil), ovMarks...)
		ovMu.Unlock()
		if mon != nil {
			full := [4]int32{0, 0, mon.W, mon.H}
			bg, _, _ := pCreateSolidBrush.Call(keyColor)
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), bg)
			pDeleteObject.Call(bg)
			penOuter, _, _ := pCreatePen.Call(0, 7, rgb(20, 20, 20))
			penInner, _, _ := pCreatePen.Call(0, 4, rgb(224, 160, 64))
			box, _, _ := pCreateSolidBrush.Call(rgb(20, 20, 20))
			fname, _ := syscall.UTF16PtrFromString("Segoe UI")
			font, _, _ := pCreateFont.Call(^uintptr(29), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 3 /*NONANTIALIASED*/, 0, uintptr(unsafe.Pointer(fname)))
			nullBrush, _, _ := pGetStockObject.Call(5)
			oldBrush, _, _ := pSelectObject.Call(hdc, nullBrush)
			oldFont, _, _ := pSelectObject.Call(hdc, font)
			pSetBkMode.Call(hdc, 1) // TRANSPARENT
			for _, m := range marks {
				x, y := m.x-mon.X, m.y-mon.Y
				for _, pen := range []uintptr{penOuter, penInner} {
					pSelectObject.Call(hdc, pen)
					pEllipse.Call(hdc, uintptr(x-26), uintptr(y-26), uintptr(x+27), uintptr(y+27))
					pMoveTo.Call(hdc, uintptr(x-40), uintptr(y), 0)
					pLineTo.Call(hdc, uintptr(x-30), uintptr(y))
					pMoveTo.Call(hdc, uintptr(x+30), uintptr(y), 0)
					pLineTo.Call(hdc, uintptr(x+41), uintptr(y))
					pMoveTo.Call(hdc, uintptr(x), uintptr(y-40), 0)
					pLineTo.Call(hdc, uintptr(x), uintptr(y-30))
					pMoveTo.Call(hdc, uintptr(x), uintptr(y+30), 0)
					pLineTo.Call(hdc, uintptr(x), uintptr(y+41))
				}
				if m.text != "" {
					t, _ := syscall.UTF16FromString(m.text)
					r := [4]int32{x + 46, y - 20, x + 46 + 900, y + 20}
					// Вычислить размер текста
					pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x400|0x20|0x800) // CALCRECT|SINGLELINE|NOPREFIX
					bgR := [4]int32{r[0] - 8, r[1] - 4, r[2] + 8, r[3] + 4}
					if bgR[2] > mon.W { // не вылезать за край — переносим текст влево от метки
						w := bgR[2] - bgR[0]
						bgR[0], bgR[2] = x-46-w, x-46
						r[0], r[2] = bgR[0]+8, bgR[2]-8
					}
					pFillRect.Call(hdc, uintptr(unsafe.Pointer(&bgR)), box)
					pSetTextColor.Call(hdc, rgb(255, 255, 255))
					pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x20|0x800)
				}
			}
			pSelectObject.Call(hdc, oldBrush)
			pSelectObject.Call(hdc, oldFont)
			pDeleteObject.Call(penOuter)
			pDeleteObject.Call(penInner)
			pDeleteObject.Call(box)
			pDeleteObject.Call(font)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps[0])))
		return 0
	case wmTimer:
		ovMu.Lock()
		now := time.Now()
		kept := ovMarks[:0]
		for _, m := range ovMarks {
			if now.Before(m.expires) {
				kept = append(kept, m)
			}
		}
		changed := len(kept) != len(ovMarks)
		ovMarks = kept
		empty := len(ovMarks) == 0
		ovMu.Unlock()
		if changed {
			applyOverlay(hwnd, empty)
		} else if !empty {
			// Resolve тоже держит окно поверх — периодически поднимаем своё
			pSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x0001|0x0002|0x0010) // HWND_TOPMOST, NOSIZE|NOMOVE|NOACTIVATE
		}
		return 0
	case wmApp + 1:
		ovMu.Lock()
		empty := len(ovMarks) == 0
		ovMu.Unlock()
		applyOverlay(hwnd, empty)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

func applyOverlay(hwnd uintptr, empty bool) {
	ovMu.Lock()
	mon := ovMon
	ovMu.Unlock()
	if empty || mon == nil {
		pShowWindow.Call(hwnd, 0) // SW_HIDE
		return
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(mon.X), uintptr(mon.Y), uintptr(mon.W), uintptr(mon.H), 0x0010|0x0040) // NOACTIVATE|SHOWWINDOW
	pInvalidateRect.Call(hwnd, 0, 1)
}

func overlayThread() {
	runtime.LockOSThread()
	hinst, _, _ := pGetModuleHandle.Call(0)
	cls, _ := syscall.UTF16PtrFromString("GradeStreamOverlay")
	wc := struct {
		Size, Style                     uint32
		WndProc                         uintptr
		ClsExtra, WndExtra              int32
		Instance, Icon, Cursor, BgBrush uintptr
		MenuName, ClassName             *uint16
		IconSm                          uintptr
	}{WndProc: syscall.NewCallback(overlayWndProc), Instance: hinst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		ovReady <- errors.New("RegisterClassEx: " + err.Error())
		return
	}
	hwnd, _, err := pCreateWindowEx.Call(wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(cls)), 0, wsPopup, 0, 0, 100, 100, 0, 0, hinst, 0)
	if hwnd == 0 {
		ovReady <- errors.New("CreateWindowEx: " + err.Error())
		return
	}
	pSetLayeredAttrs.Call(hwnd, keyColor, 0, 1) // LWA_COLORKEY
	r, _, _ := pSetDisplayAffinity.Call(hwnd, wdaExcludeCap)
	ovMu.Lock()
	ovHwnd = hwnd
	ovExclude = r != 0
	ovMu.Unlock()
	if r == 0 {
		log.Println("overlay: WDA_EXCLUDEFROMCAPTURE not supported")
	}
	pSetTimer.Call(hwnd, 1, 500, 0)
	ovReady <- nil
	var msg [48]byte
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
}

func ensureOverlay() error {
	ovOnce.Do(func() { go overlayThread() })
	select {
	case err := <-ovReady:
		ovReady <- err // оставить для следующих вызовов
		return err
	case <-time.After(3 * time.Second):
		return errors.New("окно меток не создалось")
	}
}

// Показать метку в пикселях виртуального экрана на выбранном мониторе
func overlayShow(monIdx int, x, y int32, text string, ttl time.Duration) error {
	if err := ensureOverlay(); err != nil {
		return err
	}
	ovMu.Lock()
	if !ovExclude {
		ovMu.Unlock()
		return errors.New("эта версия Windows не умеет прятать окно от захвата — метки поверх экрана выключены, чтобы клиент их не увидел (нужна Windows 10 2004+)")
	}
	ovMu.Unlock()
	mons := listMonitors()
	if monIdx < 0 || monIdx >= len(mons) {
		return errors.New("монитор не найден")
	}
	m := mons[monIdx]
	ovMu.Lock()
	ovMon = &m
	ovMarks = append(ovMarks, overlayMark{x: m.X + x, y: m.Y + y, text: text, expires: time.Now().Add(ttl)})
	if len(ovMarks) > 12 {
		ovMarks = ovMarks[len(ovMarks)-12:]
	}
	h := ovHwnd
	ovMu.Unlock()
	pPostMessage.Call(h, wmApp+1, 0, 0)
	return nil
}

func overlayClear() {
	ovMu.Lock()
	ovMarks = nil
	h := ovHwnd
	ovMu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmApp+1, 0, 0)
	}
}
