//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	kernel32   = syscall.NewLazyDLL("kernel32.dll")
	msgBox     = user32.NewProc("MessageBoxW")
	createJob  = kernel32.NewProc("CreateJobObjectW")
	setJobInfo = kernel32.NewProc("SetInformationJobObject")
	assignJob  = kernel32.NewProc("AssignProcessToJobObject")
	jobHandle  uintptr
)

func appDataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "GradeStream")
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// Дочерний процесс умрёт вместе с программой, даже при аварийном выходе
func afterStart(pid int) { addToKillJob(pid) }

type basicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}
type ioCounters struct{ a, b, c, d, e, f uint64 }
type extLimit struct {
	Basic                 basicLimit
	Io                    ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func addToKillJob(pid int) {
	if jobHandle == 0 {
		h, _, _ := createJob.Call(0, 0)
		if h == 0 {
			return
		}
		var info extLimit
		info.Basic.LimitFlags = 0x2000 // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		r, _, _ := setJobInfo.Call(h, 9, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
		if r == 0 {
			return
		}
		jobHandle = h
	}
	ph, err := syscall.OpenProcess(0x0001|0x0100, false, uint32(pid)) // TERMINATE | SET_QUOTA
	if err != nil {
		return
	}
	assignJob.Call(jobHandle, uintptr(ph))
	syscall.CloseHandle(ph)
}

func showError(msg string) {
	t, _ := syscall.UTF16PtrFromString(msg)
	c, _ := syscall.UTF16PtrFromString("GradeStream")
	msgBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}

var verDirRe = regexp.MustCompile(`^(\d+)\.\d+\.\d+\.\d+$`)

// Версия установленного Edge/Chrome — по имени папки версии рядом с exe
func browserVersion(exe string) int {
	ents, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		return 0
	}
	best := 0
	for _, e := range ents {
		if m := verDirRe.FindStringSubmatch(e.Name()); e.IsDir() && m != nil {
			if v, _ := strconv.Atoi(m[1]); v > best {
				best = v
			}
		}
	}
	return best
}

// Выбирает браузер для окна трансляции: свежий Edge, иначе самый новый из Edge/Chrome
func findBrowser() string {
	var cands []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		b := os.Getenv(env)
		if b == "" {
			continue
		}
		cands = append(cands,
			filepath.Join(b, `Microsoft\Edge\Application\msedge.exe`),
			filepath.Join(b, `Google\Chrome\Application\chrome.exe`))
	}
	// Свежий Edge — в приоритете: он есть в каждой Windows, и его окно-приложение
	// не переписывает метку GradeStream так настойчиво, как Chrome.
	best, bestVer, edge, edgeVer := "", -1, "", -1
	for _, p := range cands {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		v := browserVersion(p)
		log.Printf("browser candidate: %s (version %d)", p, v)
		if strings.Contains(strings.ToLower(p), "msedge") && v > edgeVer {
			edge, edgeVer = p, v
		}
		if v > bestVer {
			best, bestVer = p, v
		}
	}
	if edge != "" && edgeVer >= minEdgeVersion {
		return edge
	}
	return best
}

const minEdgeVersion = 110

// Открывает окно трансляции в отдельном окне Edge/Chrome (режим приложения).
// Если detached=false — ждёт, пока окно закроют.
func openHostWindow(url string, detached bool) {
	b := findBrowser()
	if b == "" {
		log.Println("browser not found, using default handler")
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		if !detached {
			select {}
		}
		return
	}
	log.Println("browser:", b)
	cmd := exec.Command(b,
		"--app="+url,
		"--user-data-dir="+filepath.Join(dataDir, "browser"),
		"--no-first-run", "--no-default-browser-check",
		"--window-size=1360,900",
		"--disable-features=Translate",
		// Трансляция не должна замедляться, когда окно свёрнуто или перекрыто
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
	)
	if err := cmd.Start(); err != nil {
		log.Println("browser start error:", err)
		cmd = nil
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		if !detached {
			select {}
		}
		return
	}
	// Повторное открытие окна передаётся уже работающему процессу браузера и сразу завершается,
	// поэтому запоминаем только процесс первого запуска.
	if cmd != nil && !detached {
		browserMu.Lock()
		browserPID = cmd.Process.Pid
		browserMu.Unlock()
	}
	if !detached && cmd != nil {
		cmd.Wait()
	}
}

var (
	browserMu  sync.Mutex
	browserPID int
)

// Закрыть окно трансляции вместе с процессами браузера
func killBrowser() {
	browserMu.Lock()
	pid := browserPID
	browserMu.Unlock()
	if pid == 0 {
		return
	}
	c := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	hideWindow(c)
	c.Run()
}

var (
	shell32         = syscall.NewLazyDLL("shell32.dll")
	pGetKnownFolder = shell32.NewProc("SHGetKnownFolderPath")
	folderDocuments = [16]byte{0xD0, 0x9A, 0xD3, 0xFD, 0x8F, 0x23, 0xAF, 0x46, 0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7} // FDD39AD0-238F-46AF-ADB4-6C85480369C7
	pCoTaskMemFreeP = syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree")
)

// Папка «Документы» (учитывает перенос в OneDrive)
func documentsDir() string {
	var p uintptr
	r, _, _ := pGetKnownFolder.Call(uintptr(unsafe.Pointer(&folderDocuments[0])), 0, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 && p != 0 {
		n := 0
		for *(*uint16)(unsafe.Pointer(p + uintptr(n*2))) != 0 {
			n++
		}
		s := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
		pCoTaskMemFreeP.Call(p)
		if s != "" {
			return s
		}
	}
	return filepath.Join(os.Getenv("USERPROFILE"), "Documents")
}

func openPath(p string) {
	if strings.HasSuffix(strings.ToLower(p), ".html") {
		exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
		return
	}
	exec.Command("explorer.exe", p).Start()
}

// Wi-Fi/Ethernet помечена как «Общедоступная» сеть — входящие подключения телефонов закрыты
func networkProfileWarning() string {
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-NetConnectionProfile | Where-Object { $_.NetworkCategory -eq 'Public' } | ForEach-Object { $_.Name }`)
	hideWindow(c)
	out, err := c.Output()
	if err != nil {
		return ""
	}
	names := strings.TrimSpace(string(out))
	if names == "" {
		return ""
	}
	list := "«" + strings.ReplaceAll(names, "\r\n", "», «") + "»"
	return L("Сеть "+list+" помечена в Windows как общедоступная — телефоны могут не подключиться. Сделайте её частной: Параметры → Сеть и Интернет → Wi-Fi (или Ethernet) → свойства сети → «Частная сеть».",
		"The network "+list+" is marked Public in Windows — phones may fail to connect. Make it Private: Settings → Network & internet → Wi-Fi (or Ethernet) → network properties → Private network.")
}

// Русский интерфейс для русской, казахской, белорусской и украинской Windows
func systemPrefersRussian() bool {
	id, _, _ := kernel32.NewProc("GetUserDefaultUILanguage").Call()
	switch id & 0x3ff { // основной язык
	case 0x19, 0x3f, 0x23, 0x22:
		return true
	}
	return false
}
