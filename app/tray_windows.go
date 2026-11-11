//go:build windows

package main

// Значок GradeStream в области уведомлений (возле часов):
// клик — показать окно трансляции, правый клик — меню (показать / остановить / выход).

import (
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	pShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	pLoadImage           = user32.NewProc("LoadImageW")
	pGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	pCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	pAppendMenu          = user32.NewProc("AppendMenuW")
	pTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	pDestroyMenu         = user32.NewProc("DestroyMenu")
	pGetCursorPos        = user32.NewProc("GetCursorPos")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pRegisterWindowMsg   = user32.NewProc("RegisterWindowMessageW")
	pEnumWindows         = user32.NewProc("EnumWindows")
	pGetClassName        = user32.NewProc("GetClassNameW")
	pGetWindowText       = user32.NewProc("GetWindowTextW")
	pIsIconic            = user32.NewProc("IsIconic")
	pIsWindowVisible     = user32.NewProc("IsWindowVisible")
	pGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
)

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        GUID
	BalloonIcon     uintptr
}

const (
	trayCallback = wmApp + 10
	trayUpdate   = wmApp + 11
	trayBalloon  = wmApp + 12
	nimAdd       = 0
	nimModify    = 1
	nimDelete    = 2
	nifMessage   = 0x1
	nifIcon      = 0x2
	nifTip       = 0x4
	nifInfo      = 0x10
	cmdShow      = 1
	cmdStop      = 2
	cmdExit      = 3
)

var (
	trayMu         sync.Mutex
	trayHwnd       uintptr
	trayIcon       uintptr
	trayTip        = "GradeStream"
	trayBalloonTxt string
	trayTaskbarMsg uintptr
	trayLive       bool
	trayOnShow     func()
	trayOnStop     func()
	trayOnExit     func()
)

func setUTF16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	for i := range dst {
		dst[i] = 0
	}
	copy(dst, u)
}

func trayData(flags uint32) notifyIconData {
	var nid notifyIconData
	nid.Size = uint32(unsafe.Sizeof(nid))
	nid.Wnd = trayHwnd
	nid.ID = 1
	nid.Flags = flags
	nid.CallbackMessage = trayCallback
	nid.Icon = trayIcon
	trayMu.Lock()
	setUTF16(nid.Tip[:], trayTip)
	trayMu.Unlock()
	return nid
}

func trayAdd() {
	nid := trayData(nifMessage | nifIcon | nifTip)
	pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

func trayMenu(hwnd uintptr) {
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(id uintptr, text string, flags uintptr) {
		t, _ := syscall.UTF16PtrFromString(text)
		pAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(t)))
	}
	add(cmdShow, L("Показать окно GradeStream", "Show GradeStream window"), 0)
	trayMu.Lock()
	live := trayLive
	trayMu.Unlock()
	if live {
		add(cmdStop, L("Остановить трансляцию", "Stop streaming"), 0)
	} else {
		add(cmdStop, L("Остановить трансляцию", "Stop streaming"), 0x1|0x2) // MF_GRAYED|MF_DISABLED
	}
	add(0, "", 0x800) // MF_SEPARATOR
	add(cmdExit, L("Выход (закрыть трансляцию и программу)", "Exit (stop streaming and close)"), 0)
	var pt [2]int32
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu, 0x0100|0x0002|0x0020, uintptr(pt[0]), uintptr(pt[1]), 0, hwnd, 0) // RETURNCMD|RIGHTBUTTON|BOTTOMALIGN
	pDestroyMenu.Call(menu)
	pPostMessage.Call(hwnd, 0, 0, 0) // WM_NULL — чтобы меню корректно закрывалось
	switch cmd {
	case cmdShow:
		go trayOnShow()
	case cmdStop:
		go trayOnStop()
	case cmdExit:
		go trayOnExit()
	}
}

func trayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch {
	case msg == trayCallback:
		switch lparam & 0xFFFF {
		case 0x0202, 0x0203: // левая кнопка: отпустили / двойной клик
			go trayOnShow()
		case 0x0205, 0x007B: // правая кнопка / контекстное меню
			trayMenu(hwnd)
		case 0x0405: // клик по уведомлению
			go trayOnShow()
		}
		return 0
	case msg == trayUpdate:
		nid := trayData(nifTip)
		pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
		return 0
	case msg == trayBalloon:
		nid := trayData(nifInfo)
		trayMu.Lock()
		setUTF16(nid.Info[:], trayBalloonTxt)
		trayMu.Unlock()
		setUTF16(nid.InfoTitle[:], L("GradeStream продолжает трансляцию", "GradeStream is still streaming"))
		nid.InfoFlags = 0x1 // NIIF_INFO
		pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
		return 0
	case trayTaskbarMsg != 0 && msg == trayTaskbarMsg: // перезапуск Проводника — вернуть значок
		trayAdd()
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

func startTray(onShow, onStop, onExit func()) {
	trayOnShow, trayOnStop, trayOnExit = onShow, onStop, onExit
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		hinst, _, _ := pGetModuleHandle.Call(0)
		cls, _ := syscall.UTF16PtrFromString("GradeStreamTray")
		wc := struct {
			Size, Style                     uint32
			WndProc                         uintptr
			ClsExtra, WndExtra              int32
			Instance, Icon, Cursor, BgBrush uintptr
			MenuName, ClassName             *uint16
			IconSm                          uintptr
		}{WndProc: syscall.NewCallback(trayWndProc), Instance: hinst, ClassName: cls}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, 0, 0, hinst, 0)
		if hwnd == 0 {
			log.Println("tray: window failed")
			close(ready)
			return
		}
		tb, _ := syscall.UTF16PtrFromString("TaskbarCreated")
		trayTaskbarMsg, _, _ = pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(tb)))
		cx, _, _ := pGetSystemMetrics.Call(49) // SM_CXSMICON
		cy, _, _ := pGetSystemMetrics.Call(50)
		trayIcon, _, _ = pLoadImage.Call(hinst, 1, 1 /*IMAGE_ICON*/, cx, cy, 0)
		trayHwnd = hwnd
		trayAdd()
		close(ready)
		var msg [48]byte
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
	}()
	<-ready
}

func setTrayState(live bool, viewers int) {
	tip := L("GradeStream — не транслируется", "GradeStream — not streaming")
	if live {
		tip = L("GradeStream — в эфире, зрителей: ", "GradeStream — live, viewers: ") + strconv.Itoa(viewers)
	}
	trayMu.Lock()
	changed := tip != trayTip || live != trayLive
	trayTip, trayLive = tip, live
	h := trayHwnd
	trayMu.Unlock()
	if changed && h != 0 {
		pPostMessage.Call(h, trayUpdate, 0, 0)
	}
}

func trayNotify(text string) {
	trayMu.Lock()
	trayBalloonTxt = text
	h := trayHwnd
	trayMu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, trayBalloon, 0, 0)
	}
}

func removeTray() {
	if trayHwnd != 0 {
		nid := trayData(0)
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}
}

// ---------- Найти и показать окно трансляции ----------

var (
	findMu   sync.Mutex
	findHwnd uintptr
	findCB   uintptr
	findOnce sync.Once
)

func enumFind(hwnd, lparam uintptr) uintptr {
	var cls [64]uint16
	pGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), 64)
	if syscall.UTF16ToString(cls[:]) != "Chrome_WidgetWin_1" {
		return 1
	}
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		return 1
	}
	// Только окна браузера, запущенного GradeStream (у него отдельный профиль и процесс)
	browserMu.Lock()
	bp := browserPID
	browserMu.Unlock()
	if bp != 0 {
		var pid uint32
		pGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if int(pid) != bp {
			return 1
		}
	}
	var title [256]uint16
	pGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), 256)
	if strings.HasPrefix(syscall.UTF16ToString(title[:]), "GradeStream") {
		findHwnd = hwnd
		return 0
	}
	return 1
}

// true — окно найдено и показано
func showHostWindow() bool {
	findOnce.Do(func() { findCB = syscall.NewCallback(enumFind) })
	findMu.Lock()
	findHwnd = 0
	pEnumWindows.Call(findCB, 0)
	h := findHwnd
	findMu.Unlock()
	if h == 0 {
		return false
	}
	if ic, _, _ := pIsIconic.Call(h); ic != 0 {
		pShowWindow.Call(h, 9) // SW_RESTORE
	} else {
		pShowWindow.Call(h, 5) // SW_SHOW
	}
	pSetForegroundWindow.Call(h)
	return true
}
