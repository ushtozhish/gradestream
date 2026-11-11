package main

// Режим «внутри сети»: телефоны в той же Wi-Fi открывают http://<IP компьютера>:порт/v/<комната>.
// Видео и звук идут по WebRTC напрямую по локальной сети (без интернета и без Cloudflare):
// кадры H.264 из окна трансляции упаковываются один раз и раздаются всем телефонам.
// Страница на http:// не получает WebCodecs (только для HTTPS), а WebRTC работает и без HTTPS.

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	qrcode "github.com/skip2/go-qrcode"
)

const h264Fmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

var (
	lanMu     sync.Mutex
	lanOn     bool
	lanPort   int
	lanSrv    *http.Server
	lanMux    *ice.MultiUDPMuxDefault
	lanAPI    *webrtc.API
	lanVideo  *webrtc.TrackLocalStaticRTP
	lanAudio  *webrtc.TrackLocalStaticRTP
	lanVPack  rtp.Packetizer
	lanASeq   uint16
	lanSendMu sync.Mutex
	// счётчики для отчётов отправителя
	lanVPkts, lanVOctets, lanAPkts, lanAOctets atomic.Uint32
	lanErr                                     string
	lanWarn                                    string
	h264Params                                 struct {
		sync.Mutex
		sps, pps [][]byte
		nalLen   int
	}
	lanAnyIP = os.Getenv("GS_LAN_ANY") != ""
	lanBytes atomic.Int64 // отправлено каждому телефону в сети (поток у всех один)
)

// ---------- Адреса компьютера в локальной сети ----------

func isPrivateV4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if lanAnyIP { // только для проверки на тестовом стенде
		return true
	}
	return ip.IsPrivate() || (ip[0] == 100 && ip[1]&0xC0 == 64) // 100.64/10 тоже бывает в локальных сетях
}

// Сначала адрес, через который идёт основной маршрут (обычно Wi-Fi/Ethernet), затем остальные
func lanIPs() []string {
	primary := ""
	if c, err := net.Dial("udp4", "8.8.8.8:80"); err == nil { // пакеты не отправляются, только выбор маршрута
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok && isPrivateV4(a.IP) {
			primary = a.IP.String()
		}
		c.Close()
	}
	var out []string
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && isPrivateV4(n.IP) {
				s := n.IP.To4().String()
				if s != primary {
					out = append(out, s)
				}
			}
		}
	}
	sort.Strings(out)
	if primary != "" {
		out = append([]string{primary}, out...)
	}
	return out
}

func lanLinks() []string {
	lanMu.Lock()
	on, p := lanOn, lanPort
	lanMu.Unlock()
	if !on {
		return nil
	}
	var l []string
	for _, ip := range lanIPs() {
		l = append(l, fmt.Sprintf("http://%s:%d/v/%s", ip, p, room))
	}
	return l
}

// ---------- Включение / выключение ----------

func lanRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/v/") != room {
			w.WriteHeader(404)
			page(w, "gone.html", nil)
			return
		}
		page(w, "view.html", map[string]string{"__ROOM__": room, "__LAN__": "true"})
	})
	mux.HandleFunc("/ws/view", func(w http.ResponseWriter, r *http.Request) { viewerWS(w, r, true) })
	mux.HandleFunc("/icon.png", serveIcon)
	return mux
}

func startLAN() error {
	lanMu.Lock()
	defer lanMu.Unlock()
	if lanOn {
		return nil
	}
	var ln net.Listener
	var err error
	for p := 8790; p < 8800; p++ {
		ln, err = net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", p))
		if err != nil {
			continue
		}
		mux, merr := ice.NewMultiUDPMuxFromPort(p,
			ice.UDPMuxFromPortWithNetworks(ice.NetworkTypeUDP4),
			ice.UDPMuxFromPortWithIPFilter(func(ip net.IP) bool { return isPrivateV4(ip) }))
		if merr != nil {
			ln.Close()
			err = merr
			continue
		}
		lanPort, lanMux = p, mux
		break
	}
	if lanMux == nil {
		lanErr = L("не удалось открыть порт в локальной сети: ", "could not open a port on the local network: ") + fmt.Sprint(err)
		return errors.New(lanErr)
	}
	if err := setupLanAPI(); err != nil {
		ln.Close()
		lanMux.Close()
		lanMux = nil
		lanErr = err.Error()
		return err
	}
	lanSrv = &http.Server{Handler: lanRoutes(), ReadHeaderTimeout: 10 * time.Second}
	go lanSrv.Serve(ln)
	lanOn, lanErr = true, ""
	log.Printf("LAN mode on, port %d, addresses %v", lanPort, lanIPs())
	go checkNetworkProfile()
	return nil
}

func stopLAN() {
	lanMu.Lock()
	if !lanOn {
		lanMu.Unlock()
		return
	}
	lanOn = false
	srv, mux := lanSrv, lanMux
	lanSrv, lanMux = nil, nil
	lanMu.Unlock()
	// Закрыть телефоны, подключённые по сети
	hub.mu.Lock()
	for _, v := range hub.viewers {
		if v.lan {
			v.ws.Close()
		}
	}
	hub.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
	if mux != nil {
		mux.Close()
	}
	log.Println("LAN mode off")
}

func setupLanAPI() error {
	m := &webrtc.MediaEngine{}
	fb := []webrtc.RTCPFeedback{{Type: "nack"}, {Type: "nack", Parameter: "pli"}, {Type: "ccm", Parameter: "fir"}, {Type: "goog-remb"}}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264Fmtp, RTCPFeedback: fb},
		PayloadType:        102,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return err
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1"},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}
	// Только повторная отправка потерянных пакетов (NACK). Отчёты отправителя (SR) шлём сами:
	// стандартные привязывают RTP-время к моменту отправки, а нам нужно время захвата —
	// по нему браузер совмещает звук с картинкой.
	ir := &interceptor.Registry{}
	if err := webrtc.ConfigureNack(m, ir); err != nil {
		return err
	}
	se := webrtc.SettingEngine{}
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	se.SetICEUDPMux(lanMux)
	se.SetICETimeouts(6*time.Second, 20*time.Second, 2*time.Second)
	lanAPI = webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se))
	var err error
	if lanVideo == nil {
		lanVideo, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264Fmtp}, "video", "gradestream")
		if err != nil {
			return err
		}
		lanAudio, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "gradestream")
		if err != nil {
			return err
		}
		lanVPack = rtp.NewPacketizer(1200, 102, 0, &codecs.H264Payloader{}, rtp.NewRandomSequencer(), 90000)
	}
	return nil
}

// Сеть Wi-Fi помечена как «Общедоступная» — Windows по умолчанию закрывает входящие подключения
func checkNetworkProfile() {
	w := networkProfileWarning()
	lanMu.Lock()
	lanWarn = w
	lanMu.Unlock()
	if w != "" {
		log.Println("network warning:", w)
	}
	sendState()
}

// ---------- WebRTC для одного телефона ----------

type lanPeer struct {
	pc *webrtc.PeerConnection
}

func newLanPeer(v *Viewer) (*lanPeer, error) {
	lanMu.Lock()
	api, on := lanAPI, lanOn
	lanMu.Unlock()
	if !on || api == nil {
		return nil, errors.New(L("режим внутри сети выключен", "Wi-Fi mode is off"))
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{}) // без STUN — только локальная сеть
	if err != nil {
		return nil, err
	}
	var senders [2]*webrtc.RTPSender
	for i, tr := range []webrtc.TrackLocal{lanVideo, lanAudio} {
		sender, err := pc.AddTransceiverFromTrack(tr, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
		if err == nil {
			senders[i] = sender.Sender()
		}
		if err != nil {
			pc.Close()
			return nil, err
		}
		go func(s *webrtc.RTPSender) { // запросы ключевого кадра от телефона
			for {
				pkts, _, err := s.ReadRTCP()
				if err != nil {
					return
				}
				for _, p := range pkts {
					switch p.(type) {
					case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
						hub.requestKeyframe()
					}
				}
			}
		}(sender.Sender())
	}
	go senderReports(pc, senders)
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		b, _ := json.Marshal(map[string]any{"t": "rtc-ice", "c": c.ToJSON()})
		v.enqueue(outMsg{op: opText, data: b}, true)
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("viewer %s webrtc: %s", v.id, s)
		if s == webrtc.PeerConnectionStateConnected {
			hub.requestKeyframe()
		}
		sendState()
	})
	offer, err := pc.CreateOffer(nil)
	if err == nil {
		err = pc.SetLocalDescription(offer)
	}
	if err != nil {
		pc.Close()
		return nil, err
	}
	b, _ := json.Marshal(map[string]any{"t": "rtc-offer", "sdp": pc.LocalDescription().SDP})
	v.enqueue(outMsg{op: opText, data: b}, true)
	return &lanPeer{pc: pc}, nil
}

func (p *lanPeer) answer(sdp string) error {
	return p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
}

func (p *lanPeer) addICE(raw json.RawMessage) {
	var c webrtc.ICECandidateInit
	if json.Unmarshal(raw, &c) == nil && c.Candidate != "" && !strings.Contains(c.Candidate, ".local") {
		p.pc.AddICECandidate(c)
	}
}

func (p *lanPeer) connected() bool {
	return p.pc.ConnectionState() == webrtc.PeerConnectionStateConnected
}

func (p *lanPeer) bytesSent() int64 { return lanBytes.Load() }

func (p *lanPeer) close() { p.pc.Close() }

// ---------- Кадры из окна трансляции → WebRTC ----------

// avcC из decoderConfig.description: длина префикса NAL, SPS, PPS
func setH264Config(descB64 string) {
	d, err := base64.StdEncoding.DecodeString(descB64)
	h264Params.Lock()
	defer h264Params.Unlock()
	h264Params.sps, h264Params.pps, h264Params.nalLen = nil, nil, 4
	if err != nil || len(d) < 7 || d[0] != 1 {
		return
	}
	h264Params.nalLen = int(d[4]&3) + 1
	i := 5
	n := int(d[i] & 0x1f)
	i++
	for k := 0; k < n && i+2 <= len(d); k++ {
		l := int(binary.BigEndian.Uint16(d[i:]))
		i += 2
		if i+l > len(d) {
			return
		}
		h264Params.sps = append(h264Params.sps, d[i:i+l])
		i += l
	}
	if i >= len(d) {
		return
	}
	n = int(d[i])
	i++
	for k := 0; k < n && i+2 <= len(d); k++ {
		l := int(binary.BigEndian.Uint16(d[i:]))
		i += 2
		if i+l > len(d) {
			return
		}
		h264Params.pps = append(h264Params.pps, d[i:i+l])
		i += l
	}
}

var startCode = []byte{0, 0, 0, 1}

// Кадр AVCC (NAL с префиксом длины) → Annex B; к ключевому кадру добавляются SPS/PPS
func avccToAnnexB(b []byte, key bool) []byte {
	h264Params.Lock()
	nl, sps, pps := h264Params.nalLen, h264Params.sps, h264Params.pps
	h264Params.Unlock()
	if nl == 0 {
		nl = 4
	}
	out := make([]byte, 0, len(b)+64)
	if key {
		for _, s := range sps {
			out = append(append(out, startCode...), s...)
		}
		for _, p := range pps {
			out = append(append(out, startCode...), p...)
		}
	}
	for i := 0; i+nl <= len(b); {
		l := 0
		for k := 0; k < nl; k++ {
			l = l<<8 | int(b[i+k])
		}
		i += nl
		if l <= 0 || i+l > len(b) {
			break
		}
		out = append(append(out, startCode...), b[i:i+l]...)
		i += l
	}
	return out
}

func anyLanPeer() bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for _, v := range hub.viewers {
		if v.rtc != nil {
			return true
		}
	}
	return false
}

func clampDur(d float64, def time.Duration) time.Duration {
	if d <= 0 || d > 500000 {
		return def
	}
	return time.Duration(d) * time.Microsecond
}

// msg: [1 байт тип][8 байт timestamp, мкс][данные]
// RTP-время из времени захвата (мкс по часам программы): одна шкала для звука и видео
func rtpTime(us int64, clock int64) uint32 { return uint32(us * clock / 1_000_000) }

func lanVideoFrame(msg []byte) {
	if lanVideo == nil || !anyLanPeer() {
		return
	}
	ts := int64(mathFloat64(msg[1:9]))
	data := avccToAnnexB(msg[9:], msg[0] == 1)
	lanSendMu.Lock()
	pkts := lanVPack.Packetize(data, 0)
	lanSendMu.Unlock()
	for _, p := range pkts {
		p.Timestamp = rtpTime(ts, 90000)
		lanVideo.WriteRTP(p)
		lanVPkts.Add(1)
		lanVOctets.Add(uint32(len(p.Payload)))
	}
	lanBytes.Add(int64(len(data)))
}

func lanAudioPacket(msg []byte) {
	if lanAudio == nil || !anyLanPeer() {
		return
	}
	ts := int64(mathFloat64(msg[1:9]))
	lanSendMu.Lock()
	lanASeq++
	seq := lanASeq
	lanSendMu.Unlock()
	p := &rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: seq, Timestamp: rtpTime(ts, 48000)},
		Payload: append([]byte(nil), msg[9:]...)}
	lanAudio.WriteRTP(p)
	lanAPkts.Add(1)
	lanAOctets.Add(uint32(len(p.Payload)))
	lanBytes.Add(int64(len(msg) - 9))
}

// Отчёты отправителя: «этому RTP-времени соответствует такое-то время по часам программы».
// Браузер по ним сводит звук и видео к общей шкале и выдаёт их синхронно.
func senderReports(pc *webrtc.PeerConnection, senders [2]*webrtc.RTPSender) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	clocks := [2]int64{90000, 48000}
	for range t.C {
		st := pc.ConnectionState()
		if st == webrtc.PeerConnectionStateClosed || st == webrtc.PeerConnectionStateFailed {
			return
		}
		if st != webrtc.PeerConnectionStateConnected {
			continue
		}
		now := time.Now()
		us := now.UnixMicro()
		ntp := uint64(now.Unix()+2208988800)<<32 | uint64(now.Nanosecond())*(1<<32)/1_000_000_000
		var reports []rtcp.Packet
		for i, snd := range senders {
			if snd == nil {
				continue
			}
			enc := snd.GetParameters().Encodings
			if len(enc) == 0 {
				continue
			}
			pk, oc := lanVPkts.Load(), lanVOctets.Load()
			if i == 1 {
				pk, oc = lanAPkts.Load(), lanAOctets.Load()
			}
			reports = append(reports, &rtcp.SenderReport{SSRC: uint32(enc[0].SSRC), NTPTime: ntp, RTPTime: rtpTime(us, clocks[i]), PacketCount: pk, OctetCount: oc})
		}
		if len(reports) > 0 {
			pc.WriteRTCP(reports)
		}
	}
}

// ---------- QR-код ссылки ----------

func handleQR(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) || r.URL.Query().Get("key") != hostKey {
		http.NotFound(w, r)
		return
	}
	png, err := qrcode.Encode(r.URL.Query().Get("text"), qrcode.Medium, 440)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

// ---------- Режим трансляции ----------

func currentMode() string {
	setMu.Lock()
	defer setMu.Unlock()
	if cfg.Mode == "lan" || cfg.Mode == "both" {
		return cfg.Mode
	}
	return "internet"
}

// internet — только ссылка через интернет; lan — только Wi-Fi; both — обе ссылки одновременно
// (кадр кодируется один раз и уходит обоими путями).
func applyMode(mode string) {
	wantNet, wantLAN := mode != "lan", mode != "internet"
	if wantNet {
		startTunnel(port)
	} else {
		stopTunnel()
		// Телефоны, подключённые через интернет, отключаем — ссылка больше не работает
		hub.mu.Lock()
		for _, v := range hub.viewers {
			if !v.lan {
				v.ws.Close()
			}
		}
		hub.mu.Unlock()
	}
	if wantLAN {
		if err := startLAN(); err != nil {
			log.Println("LAN error:", err)
		}
	} else {
		stopLAN()
	}
	sendState()
}

func setMode(mode string) {
	if mode != "lan" && mode != "both" {
		mode = "internet"
	}
	setMu.Lock()
	changed := cfg.Mode != mode
	cfg.Mode = mode
	saveSettings()
	setMu.Unlock()
	if changed || (mode != "internet") != lanActive() {
		applyMode(mode)
	}
}

func lanActive() bool {
	lanMu.Lock()
	defer lanMu.Unlock()
	return lanOn
}

func lanState() (links []string, errText, warn string) {
	lanMu.Lock()
	errText, warn = lanErr, lanWarn
	lanMu.Unlock()
	return lanLinks(), errText, warn
}
