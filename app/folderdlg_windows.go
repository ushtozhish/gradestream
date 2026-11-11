//go:build windows

package main

// Стандартный диалог Windows «Выбор папки» (IFileOpenDialog).

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	clsidFileOpenDialog = guid("DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7")
	iidIFileOpenDialog  = guid("D57C7288-D4AD-4768-BE02-9D969532D960")
	iidIShellItem       = guid("43826D1E-E718-42EE-BC55-A1E261C37BFE")
	pSHCreateItem       = shell32.NewProc("SHCreateItemFromParsingName")
	pGetForeground      = user32.NewProc("GetForegroundWindow")
)

var errCancelled = errors.New("cancelled")

func pickFolder(title, start string) (string, error) {
	owner, _, _ := pGetForeground.Call() // окно программы — диалог откроется поверх него
	type res struct {
		p string
		e error
	}
	ch := make(chan res, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r, _, _ := procCoInit.Call(0, 2) // COINIT_APARTMENTTHREADED
		if r == 0 || r == 1 {
			defer procCoUninit.Call()
		}
		p, e := folderDialog(owner, title, start)
		ch <- res{p, e}
	}()
	x := <-ch
	return x.p, x.e
}

func folderDialog(owner uintptr, title, start string) (string, error) {
	var dlg uintptr
	r, _, _ := procCoCreate.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1, /*CLSCTX_INPROC_SERVER*/
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if err := hrErr(r, "FileOpenDialog"); err != nil {
		return "", err
	}
	defer release(dlg)
	var opts uint32
	vcall(dlg, 10, uintptr(unsafe.Pointer(&opts)))
	vcall(dlg, 9, uintptr(opts|0x20|0x40|0x800)) // PICKFOLDERS | FORCEFILESYSTEM | PATHMUSTEXIST
	if t, err := syscall.UTF16PtrFromString(title); err == nil {
		vcall(dlg, 17, uintptr(unsafe.Pointer(t)))
	}
	if ok, err := syscall.UTF16PtrFromString("Сохранять сюда"); err == nil {
		vcall(dlg, 18, uintptr(unsafe.Pointer(ok)))
	}
	if start != "" {
		if sp, err := syscall.UTF16PtrFromString(start); err == nil {
			var item uintptr
			if r, _, _ := pSHCreateItem.Call(uintptr(unsafe.Pointer(sp)), 0, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))); r == 0 {
				vcall(dlg, 12, item) // SetFolder
				release(item)
			}
		}
	}
	r = vcall(dlg, 3, owner) // Show
	if uint32(r) == 0x800704C7 {
		return "", errCancelled
	}
	if err := hrErr(r, "диалог выбора папки"); err != nil {
		return "", err
	}
	var item uintptr
	if err := hrErr(vcall(dlg, 20, uintptr(unsafe.Pointer(&item))), "GetResult"); err != nil {
		return "", err
	}
	defer release(item)
	var p uintptr
	if err := hrErr(vcall(item, 5, 0x80058000, uintptr(unsafe.Pointer(&p))), "GetDisplayName"); err != nil { // SIGDN_FILESYSPATH
		return "", err
	}
	s := wstr(p)
	procCoTaskMemFree.Call(p)
	if s == "" {
		return "", errors.New("выбрана не обычная папка на диске")
	}
	return s, nil
}
