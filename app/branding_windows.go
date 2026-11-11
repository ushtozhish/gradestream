//go:build windows

package main

// Своё лицо у окна трансляции: окно Edge/Chrome получает идентификатор приложения GradeStream
// (AppUserModelID) и иконку GradeStream. Windows показывает его отдельной кнопкой
// на панели задач со значком GradeStream, а не браузера; закреплённая кнопка запускает GradeStream.exe.
// Браузер (особенно Chrome) может позже переписать идентификатор своего окна-приложения —
// поэтому метка проверяется каждые 2 секунды и возвращается. Ярлыки GradeStream в «Пуске»
// и на рабочем столе получают тот же идентификатор: Windows связывает окно с ярлыком и его иконкой.

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const appUserModelID = "GradeStream.App"

var (
	iidIPropertyStore       = guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99")
	pkeyAppUserModelFmt     = guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3")
	pSHGetPropStoreForWnd   = shell32.NewProc("SHGetPropertyStoreForWindow")
	pSetCurrentProcessAUMID = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	pSendMessage            = user32.NewProc("SendMessageW")
	clsidShellLink          = guid("00021401-0000-0000-C000-000000000046")
	iidIShellLinkW          = guid("000214F9-0000-0000-C000-000000000046")
	iidIPersistFile         = guid("0000010B-0000-0000-C000-000000000046")
	brandBig, brandSmall    uintptr
)

func setProcessAppID() {
	id, _ := syscall.UTF16PtrFromString(appUserModelID)
	pSetCurrentProcessAUMID.Call(uintptr(unsafe.Pointer(id)))
}

func setWindowProp(store uintptr, pid uint32, value string) error {
	key := struct {
		Fmt GUID
		Pid uint32
	}{pkeyAppUserModelFmt, pid}
	v, _ := syscall.UTF16PtrFromString(value)
	var pv [24]byte
	*(*uint16)(unsafe.Pointer(&pv[0])) = 31 // VT_LPWSTR
	*(*uintptr)(unsafe.Pointer(&pv[8])) = uintptr(unsafe.Pointer(v))
	r := vcall(store, 6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&pv[0])))
	return hrErr(r, "SetValue")
}

// Текущий идентификатор приложения у окна
func windowAppID(hwnd uintptr) string {
	var store uintptr
	r, _, _ := pSHGetPropStoreForWnd.Call(hwnd, uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&store)))
	if r != 0 || store == 0 {
		return ""
	}
	defer release(store)
	return getStoreString(store, 5)
}

func getStoreString(store uintptr, pid uint32) string {
	key := struct {
		Fmt GUID
		Pid uint32
	}{pkeyAppUserModelFmt, pid}
	var pv [24]byte
	if vcall(store, 5, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&pv[0]))) != 0 {
		return ""
	}
	defer procPropVarClear.Call(uintptr(unsafe.Pointer(&pv[0])))
	if *(*uint16)(unsafe.Pointer(&pv[0])) != 31 {
		return ""
	}
	return wstr(*(*uintptr)(unsafe.Pointer(&pv[8])))
}

func brandWindow(hwnd uintptr) {
	exe, _ := os.Executable()
	var store uintptr
	r, _, _ := pSHGetPropStoreForWnd.Call(hwnd, uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&store)))
	if r != 0 || store == 0 {
		log.Printf("branding: property store 0x%X", uint32(r))
		return
	}
	defer release(store)
	setWindowProp(store, 2, `"`+exe+`"`)                            // RelaunchCommand
	setWindowProp(store, 4, "GradeStream")                          // RelaunchDisplayNameResource
	setWindowProp(store, 3, exe+",0")                               // RelaunchIconResource
	if err := setWindowProp(store, 5, appUserModelID); err != nil { // ID
		log.Println("branding:", err)
	}
	vcall(store, 7) // Commit
}

func setWindowIcons(hwnd uintptr) {
	if brandBig == 0 {
		hinst, _, _ := pGetModuleHandle.Call(0)
		cx, _, _ := pGetSystemMetrics.Call(11) // SM_CXICON
		cy, _, _ := pGetSystemMetrics.Call(12)
		sx, _, _ := pGetSystemMetrics.Call(49) // SM_CXSMICON
		sy, _, _ := pGetSystemMetrics.Call(50)
		brandBig, _, _ = pLoadImage.Call(hinst, 1, 1, cx, cy, 0)
		brandSmall, _, _ = pLoadImage.Call(hinst, 1, 1, sx, sy, 0)
	}
	if brandBig != 0 {
		pSendMessage.Call(hwnd, 0x0080, 1, brandBig) // WM_SETICON, ICON_BIG
	}
	if brandSmall != 0 {
		pSendMessage.Call(hwnd, 0x0080, 0, brandSmall) // ICON_SMALL
	}
}

// Следит за окном трансляции: брендирует новое окно и возвращает метку, если браузер её перезаписал.
func startBranding() {
	go ensureShortcutsAppID()
	go func() {
		fin := comInit() // свойства окна — это COM; поток должен быть инициализирован
		defer fin()
		var last uintptr
		for {
			findOnce.Do(func() { findCB = syscall.NewCallback(enumFind) })
			findMu.Lock()
			findHwnd = 0
			pEnumWindows.Call(findCB, 0)
			h := findHwnd
			findMu.Unlock()
			if h != 0 {
				if id := windowAppID(h); h != last || id != appUserModelID {
					brandWindow(h)
					if h == last {
						log.Printf("branding restored (browser had set %q)", id)
					} else {
						log.Println("branding applied")
					}
					last = h
				}
				setWindowIcons(h)
			}
			if h == 0 {
				time.Sleep(300 * time.Millisecond)
			} else {
				time.Sleep(2 * time.Second)
			}
		}
	}()
}

// Ярлыки GradeStream получают тот же AppUserModelID, что и окно трансляции
func ensureShortcutsAppID() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, _ := procCoInit.Call(0, 2); r == 0 || r == 1 { // COINIT_APARTMENTTHREADED
		defer procCoUninit.Call()
	}
	var paths []string
	for _, dir := range []string{
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`),
		filepath.Join(os.Getenv("USERPROFILE"), "Desktop"),
		filepath.Join(os.Getenv("USERPROFILE"), `OneDrive\Desktop`),
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar`),
	} {
		paths = append(paths, filepath.Join(dir, "GradeStream.lnk"))
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := setShortcutAppID(p); err != nil {
			log.Printf("shortcut %s: %v", p, err)
		}
	}
}

func setShortcutAppID(path string) error {
	var link uintptr
	r, _, _ := procCoCreate.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1,
		uintptr(unsafe.Pointer(&iidIShellLinkW)), uintptr(unsafe.Pointer(&link)))
	if err := hrErr(r, "ShellLink"); err != nil {
		return err
	}
	defer release(link)
	var pf uintptr
	if err := hrErr(vcall(link, 0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&pf))), "IPersistFile"); err != nil {
		return err
	}
	defer release(pf)
	p, _ := syscall.UTF16PtrFromString(path)
	if err := hrErr(vcall(pf, 5, uintptr(unsafe.Pointer(p)), 2 /*STGM_READWRITE*/), "Load"); err != nil {
		return err
	}
	var store uintptr
	if err := hrErr(vcall(link, 0, uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&store))), "IPropertyStore"); err != nil {
		return err
	}
	defer release(store)
	if getStoreString(store, 5) == appUserModelID {
		return nil // уже помечен
	}
	if err := setWindowProp(store, 5, appUserModelID); err != nil {
		return err
	}
	if err := hrErr(vcall(store, 7), "Commit"); err != nil {
		return err
	}
	if err := hrErr(vcall(pf, 6, 0, 1), "Save"); err != nil {
		return err
	}
	log.Println("shortcut tagged:", path)
	return nil
}
