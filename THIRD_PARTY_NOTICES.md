# Third-party components

GradeStream is released under the MIT license (see [LICENSE](LICENSE)). The app and the installer include components by other authors:

| Component | Version | License | Used for |
|---|---|---|---|
| [cloudflared](https://github.com/cloudflare/cloudflared) — Cloudflare, Inc. | 2026.10.7, official signed binary | Apache License 2.0 | Public link via Cloudflare Tunnel; shipped unmodified inside the installer |
| [Pion WebRTC](https://github.com/pion/webrtc) and Pion modules (ice, dtls, srtp, sctp, rtp, rtcp, interceptor, …) | v4.2.26 | MIT | Wi-Fi streaming (WebRTC) |
| [go-qrcode](https://github.com/skip2/go-qrcode) — Tom Harwood | 2020-06-17 | MIT | QR code for the link |
| [google/uuid](https://github.com/google/uuid) | 1.6.0 | BSD-3-Clause | Pion dependency |
| [wlynxg/anet](https://github.com/wlynxg/anet) | 0.0.5 | BSD-3-Clause | Pion dependency |
| [golang.org/x/net, crypto, sys, time](https://go.googlesource.com/) — The Go Authors | see `app/go.mod` | BSD-3-Clause | Pion dependencies |
| Go runtime and standard library — The Go Authors | 1.27 | BSD-3-Clause | The whole app |

Full license texts are in each component's repository linked above. cloudflared is distributed unmodified; source code: <https://github.com/cloudflare/cloudflared/tree/2026.10.7>.

Cloudflare Tunnel is a Cloudflare service. Account-less quick tunnels (trycloudflare.com) are provided by Cloudflare with no uptime guarantee and under the [Cloudflare Online Services Terms of Use](https://www.cloudflare.com/website-terms/).
