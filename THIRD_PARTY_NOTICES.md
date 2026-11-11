# Сторонние компоненты

GradeStream распространяется под лицензией MIT (см. [LICENSE](LICENSE)). В программу и установщик входят компоненты других авторов:

| Компонент | Версия | Лицензия | Где используется |
|---|---|---|---|
| [cloudflared](https://github.com/cloudflare/cloudflared) — Cloudflare, Inc. | 2026.10.7, официальный подписанный файл | Apache License 2.0 | Публичная ссылка через Cloudflare Tunnel; лежит в установщике без изменений |
| [Pion WebRTC](https://github.com/pion/webrtc) и модули Pion (ice, dtls, srtp, sctp, rtp, rtcp, interceptor, …) | v4.2.26 | MIT | Передача по Wi-Fi (WebRTC) |
| [go-qrcode](https://github.com/skip2/go-qrcode) — Tom Harwood | 2020-06-17 | MIT | QR-код ссылки |
| [google/uuid](https://github.com/google/uuid) | 1.6.0 | BSD-3-Clause | Зависимость Pion |
| [wlynxg/anet](https://github.com/wlynxg/anet) | 0.0.5 | BSD-3-Clause | Зависимость Pion |
| [golang.org/x/net, crypto, sys, time](https://go.googlesource.com/) — The Go Authors | см. `app/go.mod` | BSD-3-Clause | Зависимости Pion |
| Go runtime и стандартная библиотека — The Go Authors | 1.27 | BSD-3-Clause | Вся программа |

Полные тексты лицензий — в репозиториях компонентов по ссылкам выше. cloudflared поставляется в неизменённом виде, исходный код: <https://github.com/cloudflare/cloudflared/tree/2026.10.7>.

Cloudflare Tunnel — сервис компании Cloudflare. Быстрые туннели без аккаунта (trycloudflare.com) Cloudflare предоставляет без гарантий доступности и на условиях [Cloudflare Online Services Terms of Use](https://www.cloudflare.com/website-terms/).
