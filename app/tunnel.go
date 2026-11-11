package main

// Публичная HTTPS-ссылка через Cloudflare Quick Tunnel (cloudflared лежит рядом с программой).

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	tunMu     sync.Mutex
	tunURL    string
	tunStatus = "starting" // starting | on | error | missing
	tunErr    string
	tunCmd    *exec.Cmd
	tunStop   bool
	tunLoop   bool // цикл запуска cloudflared работает
	tunRe     = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
)

func tunnelInfo() (string, string, string) {
	tunMu.Lock()
	defer tunMu.Unlock()
	return tunURL, tunStatus, tunErr
}

func cloudflaredPath() string {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	for _, name := range []string{"cloudflared.exe", "cloudflared"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p
	}
	return ""
}

func startTunnel(port int) {
	if os.Getenv("GS_NO_TUNNEL") != "" {
		tunMu.Lock()
		tunStatus, tunErr = "error", "туннель отключён"
		tunMu.Unlock()
		return
	}
	bin := cloudflaredPath()
	if bin == "" {
		tunMu.Lock()
		tunStatus, tunErr = "missing", "cloudflared.exe не найден рядом с программой — переустановите GradeStream"
		tunMu.Unlock()
		return
	}
	tunMu.Lock()
	tunStop = false
	if tunLoop { // уже работает (или останавливается — тогда продолжит работу)
		tunMu.Unlock()
		return
	}
	tunLoop = true
	tunMu.Unlock()
	go func() {
		defer func() {
			tunMu.Lock()
			tunLoop = false
			tunMu.Unlock()
		}()
		attempt := 0
		protocols := []string{"auto", "http2"} // http2 — запасной вариант, если UDP/QUIC закрыт
		for {
			tunMu.Lock()
			if tunStop {
				tunStatus, tunURL, tunErr = "off", "", ""
				tunMu.Unlock()
				return
			}
			tunStatus, tunURL = "starting", ""
			tunMu.Unlock()
			sendState()

			proto := protocols[attempt%len(protocols)]
			args := []string{"tunnel", "--no-autoupdate", "--protocol", proto, "--url", fmt.Sprintf("http://127.0.0.1:%d", port)}
			cmd := exec.Command(bin, args...)
			hideWindow(cmd)
			stderr, _ := cmd.StderrPipe()
			stdout, _ := cmd.StdoutPipe()
			if err := cmd.Start(); err != nil {
				tunMu.Lock()
				tunStatus, tunErr = "error", "не удалось запустить cloudflared: "+err.Error()
				tunMu.Unlock()
				log.Println(tunErr)
				return
			}
			afterStart(cmd.Process.Pid)
			tunMu.Lock()
			tunCmd = cmd
			tunMu.Unlock()
			log.Printf("cloudflared started (protocol %s)", proto)

			var lastLines []string
			var llMu sync.Mutex
			scan := func(r io.Reader) {
				sc := bufio.NewScanner(r)
				for sc.Scan() {
					line := sc.Text()
					llMu.Lock()
					lastLines = append(lastLines, line)
					if len(lastLines) > 8 {
						lastLines = lastLines[1:]
					}
					llMu.Unlock()
					if m := tunRe.FindString(line); m != "" {
						tunMu.Lock()
						if tunURL == "" {
							tunURL, tunStatus, tunErr = m, "on", ""
							log.Println("tunnel:", m)
						}
						tunMu.Unlock()
						sendState()
					}
				}
			}
			go scan(stdout)
			go scan(stderr)

			// Если за 40 секунд ссылка не появилась — пробуем другой протокол
			timer := time.AfterFunc(40*time.Second, func() {
				u, _, _ := tunnelInfo()
				if u == "" {
					log.Println("tunnel timeout, restarting")
					cmd.Process.Kill()
				}
			})
			cmd.Wait()
			timer.Stop()

			tunMu.Lock()
			stop := tunStop
			tunCmd = nil
			hadURL := tunURL != ""
			tunURL = ""
			llMu.Lock()
			tail := strings.Join(lastLines, "\n")
			llMu.Unlock()
			tunStatus = "error"
			tunErr = "Туннель Cloudflare не поднялся. Проверьте интернет и что антивирус/фаервол не блокирует cloudflared.exe. Пробую снова…"
			tunMu.Unlock()
			if stop {
				tunMu.Lock()
				tunStatus, tunErr = "off", ""
				tunMu.Unlock()
				sendState()
				return
			}
			log.Printf("cloudflared exited, last output:\n%s", tail)
			sendState()
			if !hadURL {
				attempt++
			}
			time.Sleep(3 * time.Second)
		}
	}()
}

func stopTunnel() {
	tunMu.Lock()
	tunStop = true
	c := tunCmd
	tunURL, tunStatus, tunErr = "", "off", ""
	tunMu.Unlock()
	if c != nil && c.Process != nil {
		c.Process.Kill()
	}
}
