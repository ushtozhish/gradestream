//go:build windows

package main

// Установщик GradeStream.
// Ставит программу для текущего пользователя (права администратора не нужны):
//   %LOCALAPPDATA%\Programs\GradeStream
// Регистрирует её в «Параметры → Приложения → Установленные приложения»,
// создаёт ярлыки в меню «Пуск» и на рабочем столе.
// Запуск с ключом /uninstall — удаление.

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appName    = "GradeStream"
	appVersion = "2.10.1"
	publisher  = "GradeStream"
	regPath    = `Software\Microsoft\Windows\CurrentVersion\Uninstall\GradeStream`
)

//go:embed payload/GradeStream.exe
var appExe []byte

//go:embed payload/cloudflared.exe
var cfExe []byte

//go:embed payload/gradestream.ico
var appIco []byte

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	msgBox       = user32.NewProc("MessageBoxW")
	regCreateKey = advapi32.NewProc("RegCreateKeyExW")
	regSetValue  = advapi32.NewProc("RegSetValueExW")
	regCloseKey  = advapi32.NewProc("RegCloseKey")
	regDeleteKey = advapi32.NewProc("RegDeleteKeyW")
)

const (
	mbOK         = 0x0
	mbYesNo      = 0x4
	mbIconError  = 0x10
	mbIconQuest  = 0x20
	mbIconInfo   = 0x40
	idYes        = 6
	hkeyCurUser  = 0x80000001
	keyAllAccess = 0xF003F
	regSZ        = 1
	regDWORD     = 4
)

func box(text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(appName)
	r, _, _ := msgBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return int(r)
}

func installDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", appName)
}

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func ps(script string, env ...string) error {
	cmd := hidden(exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script))
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Закрыть запущенную программу и её туннель (только наши процессы из папки установки)
func stopRunning(dir string) {
	ps(`Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Id -ne [int]$env:GS_SELF -and $_.Path -and $_.Path.StartsWith($env:GS_DIR, [StringComparison]::OrdinalIgnoreCase) } | Stop-Process -Force -ErrorAction SilentlyContinue`,
		"GS_DIR="+dir+`\`, fmt.Sprintf("GS_SELF=%d", os.Getpid()))
	// Окно трансляции (Edge в режиме приложения с нашим профилем)
	ps(`Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -and $_.CommandLine.Contains($env:GS_PROF) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`,
		"GS_PROF="+filepath.Join(os.Getenv("LOCALAPPDATA"), appName, "browser"))
	time.Sleep(700 * time.Millisecond)
}

// ---------- Реестр ----------

func regSetStr(h uintptr, name, val string) error {
	n, _ := syscall.UTF16PtrFromString(name)
	v, _ := syscall.UTF16FromString(val)
	r, _, _ := regSetValue.Call(h, uintptr(unsafe.Pointer(n)), 0, regSZ, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueEx %s: %d", name, r)
	}
	return nil
}

func regSetDword(h uintptr, name string, val uint32) error {
	n, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := regSetValue.Call(h, uintptr(unsafe.Pointer(n)), 0, regDWORD, uintptr(unsafe.Pointer(&val)), 4)
	if r != 0 {
		return fmt.Errorf("RegSetValueEx %s: %d", name, r)
	}
	return nil
}

func registerUninstall(dir string, sizeKB uint32) error {
	p, _ := syscall.UTF16PtrFromString(regPath)
	var h uintptr
	r, _, _ := regCreateKey.Call(hkeyCurUser, uintptr(unsafe.Pointer(p)), 0, 0, 0, keyAllAccess, 0, uintptr(unsafe.Pointer(&h)), 0)
	if r != 0 {
		return fmt.Errorf("RegCreateKeyEx: %d", r)
	}
	defer regCloseKey.Call(h)
	un := filepath.Join(dir, "uninstall.exe")
	for _, kv := range [][2]string{
		{"DisplayName", appName},
		{"DisplayVersion", appVersion},
		{"Publisher", publisher},
		{"DisplayIcon", filepath.Join(dir, "GradeStream.exe") + ",0"},
		{"InstallLocation", dir},
		{"UninstallString", `"` + un + `" /uninstall`},
		{"QuietUninstallString", `"` + un + `" /uninstall /quiet`},
		{"InstallDate", time.Now().Format("20060102")},
		{"Comments", L("Трансляция Clean Feed клиенту по ссылке", "Stream a Clean Feed to a client via a link")},
	} {
		if err := regSetStr(h, kv[0], kv[1]); err != nil {
			return err
		}
	}
	regSetDword(h, "NoModify", 1)
	regSetDword(h, "NoRepair", 1)
	regSetDword(h, "EstimatedSize", sizeKB)
	return nil
}

// ---------- Ярлыки ----------

const shortcutPS = `
$ws = New-Object -ComObject WScript.Shell
foreach ($folder in @([Environment]::GetFolderPath('Programs'), [Environment]::GetFolderPath('Desktop'))) {
  if (-not $folder) { continue }
  $s = $ws.CreateShortcut((Join-Path $folder 'GradeStream.lnk'))
  $s.TargetPath = $env:GS_EXE
  $s.WorkingDirectory = $env:GS_DIR
  $s.IconLocation = $env:GS_EXE + ',0'
  $s.Description = 'Трансляция Clean Feed клиенту'
  $s.Save()
}`

const removeShortcutsPS = `
foreach ($folder in @([Environment]::GetFolderPath('Programs'), [Environment]::GetFolderPath('Desktop'))) {
  if ($folder) { Remove-Item -LiteralPath (Join-Path $folder 'GradeStream.lnk') -Force -ErrorAction SilentlyContinue }
}`

// ---------- Установка ----------

func install(quiet bool) {
	dir := installDir()
	if !quiet {
		msg := fmt.Sprintf(L("Установить %s %s?\n\nПапка: %s\nПрава администратора не нужны.\n\nПрограмма появится в меню «Пуск», на рабочем столе и в списке установленных приложений Windows.",
			"Install %s %s?\n\nFolder: %s\nNo administrator rights needed.\n\nThe app will appear in the Start menu, on the desktop and in Windows Installed apps."), appName, appVersion, dir)
		if box(msg, mbYesNo|mbIconQuest) != idYes {
			return
		}
	}
	if os.Getenv("LOCALAPPDATA") == "" {
		box(L("Не найдена папка LOCALAPPDATA.", "LOCALAPPDATA folder not found."), mbIconError)
		return
	}
	stopRunning(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		box(L("Не удалось создать папку:\n", "Could not create the folder:\n")+err.Error(), mbIconError)
		return
	}
	self, _ := os.Executable()
	selfBytes, err := os.ReadFile(self)
	if err != nil {
		box(L("Не удалось прочитать установщик:\n", "Could not read the installer:\n")+err.Error(), mbIconError)
		return
	}
	files := map[string][]byte{
		"GradeStream.exe": appExe,
		"cloudflared.exe": cfExe,
		"gradestream.ico": appIco,
	}
	// uninstall.exe — копия установщика; не перезаписываем, если запущены из него же
	unPath := filepath.Join(dir, "uninstall.exe")
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(unPath)) {
		files["uninstall.exe"] = selfBytes
	}
	var total int64
	for name, data := range files {
		p := filepath.Join(dir, name)
		var werr error
		for i := 0; i < 10; i++ { // файл может быть ещё занят закрывающимся процессом
			if werr = os.WriteFile(p, data, 0o755); werr == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if werr != nil {
			box(L("Не удалось записать ", "Could not write ")+name+":\n"+werr.Error()+L("\n\nЗакройте GradeStream и попробуйте снова.", "\n\nClose GradeStream and try again."), mbIconError)
			return
		}
		total += int64(len(data))
	}
	if err := registerUninstall(dir, uint32(total/1024)); err != nil {
		box(L("Программа скопирована, но не зарегистрирована в Windows:\n", "The app was copied but not registered in Windows:\n")+err.Error(), mbIconError)
	}
	exe := filepath.Join(dir, "GradeStream.exe")
	if err := ps(shortcutPS, "GS_EXE="+exe, "GS_DIR="+dir); err != nil {
		box(L("Не удалось создать ярлыки (программу можно запускать из папки установки):\n", "Could not create shortcuts (you can run the app from its install folder):\n")+err.Error(), mbIconError)
	}
	if quiet {
		return
	}
	if box(appName+L(" установлен.\n\nЗапустить сейчас?", " is installed.\n\nLaunch it now?"), mbYesNo|mbIconInfo) == idYes {
		c := exec.Command(exe)
		c.Dir = dir
		c.Start()
	}
}

// ---------- Удаление ----------

func uninstall(quiet bool) {
	dir := installDir()
	if !quiet && box(L("Удалить ", "Remove ")+appName+L(" с этого компьютера?", " from this computer?"), mbYesNo|mbIconQuest) != idYes {
		return
	}
	stopRunning(dir)
	ps(removeShortcutsPS)
	p, _ := syscall.UTF16PtrFromString(regPath)
	regDeleteKey.Call(hkeyCurUser, uintptr(unsafe.Pointer(p)))
	for _, n := range []string{"GradeStream.exe", "cloudflared.exe", "gradestream.ico"} {
		os.Remove(filepath.Join(dir, n))
	}
	os.RemoveAll(filepath.Join(os.Getenv("LOCALAPPDATA"), appName)) // профиль окна и журнал
	// uninstall.exe сейчас запущен — удаляем его и папку чуть позже
	hidden(exec.Command("cmd.exe", "/c", "ping 127.0.0.1 -n 3 >nul & rmdir /s /q \""+dir+"\"")).Start()
	if !quiet {
		box(appName+L(" удалён.", " has been removed."), mbOK|mbIconInfo)
	}
}

func main() {
	args := strings.ToLower(strings.Join(os.Args[1:], " "))
	quiet := strings.Contains(args, "/quiet") || strings.Contains(" "+args+" ", " /s ")
	if strings.Contains(args, "/uninstall") {
		uninstall(quiet)
		return
	}
	// Запуск самого uninstall.exe без ключей тоже считаем удалением
	self, _ := os.Executable()
	if strings.EqualFold(filepath.Base(self), "uninstall.exe") {
		uninstall(quiet)
		return
	}
	install(quiet)
}

// Язык окон установщика — по языку интерфейса Windows
var ruUI = func() bool {
	id, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	switch id & 0x3ff {
	case 0x19, 0x3f, 0x23, 0x22: // ru, kk, be, uk
		return true
	}
	return false
}()

func L(ru, en string) string {
	if ruUI {
		return ru
	}
	return en
}
