package main

// Минимальная реализация WebSocket (RFC 6455) поверх net/http — без внешних библиотек.

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	opText   = 1
	opBinary = 2
	opClose  = 8
	opPing   = 9
	opPong   = 10
)

type WSConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (*WSConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket expected", 400)
		return nil, errors.New("not websocket")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "no key", 400)
		return nil, errors.New("no key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("no hijack")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(h[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	return &WSConn{c: conn, br: brw.Reader}, nil
}

// ReadMessage возвращает целое сообщение (склеивая фрагменты). Пинги отвечаются автоматически.
func (ws *WSConn) ReadMessage() (op byte, data []byte, err error) {
	var msgOp byte
	var buf []byte
	for {
		var hdr [2]byte
		if _, err = io.ReadFull(ws.br, hdr[:]); err != nil {
			return
		}
		fin := hdr[0]&0x80 != 0
		o := hdr[0] & 0x0f
		masked := hdr[1]&0x80 != 0
		n := uint64(hdr[1] & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err = io.ReadFull(ws.br, b[:]); err != nil {
				return
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err = io.ReadFull(ws.br, b[:]); err != nil {
				return
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > 64<<20 {
			err = errors.New("frame too large")
			return
		}
		var mask [4]byte
		if masked {
			if _, err = io.ReadFull(ws.br, mask[:]); err != nil {
				return
			}
		}
		payload := make([]byte, n)
		if _, err = io.ReadFull(ws.br, payload); err != nil {
			return
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch o {
		case opPing:
			ws.write(opPong, payload)
			continue
		case opPong:
			continue
		case opClose:
			ws.write(opClose, nil)
			err = io.EOF
			return
		case 0: // продолжение
			buf = append(buf, payload...)
		default:
			msgOp = o
			buf = payload
		}
		if fin {
			return msgOp, buf, nil
		}
	}
}

func (ws *WSConn) write(op byte, data []byte) error {
	ws.wmu.Lock()
	defer ws.wmu.Unlock()
	hdr := make([]byte, 0, 10)
	hdr = append(hdr, 0x80|op)
	n := len(data)
	switch {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 65536:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, b[:]...)
	}
	ws.c.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if _, err := ws.c.Write(hdr); err != nil {
		return err
	}
	_, err := ws.c.Write(data)
	return err
}

func (ws *WSConn) WriteText(s string) error   { return ws.write(opText, []byte(s)) }
func (ws *WSConn) WriteBinary(b []byte) error { return ws.write(opBinary, b) }
func (ws *WSConn) Ping() error                { return ws.write(opPing, nil) }
func (ws *WSConn) Close() error               { return ws.c.Close() }
