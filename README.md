<p align="center"><img src="docs/img/icon.png" width="80" alt=""></p>

<h1 align="center">GradeStream</h1>

<p align="center"><b>Stream a DaVinci Resolve Clean Feed to your client's phone via a link — nothing to install on the viewer's side.</b><br>
For colorists and editors: show a grade during a remote session or to everyone in the room, and get feedback as markers placed right on the frame.</p>

<p align="center">Windows 10/11 · free · open source (MIT) · English / Русский<br>
<a href="https://github.com/ushtozhish/gradestream/actions/workflows/build-and-measure.yml"><img src="https://github.com/ushtozhish/gradestream/actions/workflows/build-and-measure.yml/badge.svg" alt="Build and measure"></a><br>
<a href="https://github.com/ushtozhish/gradestream/releases/latest"><b>⬇ Download the installer</b></a></p>

![GradeStream control window: live preview with client markers, internet and Wi-Fi links with a QR code](docs/img/control-window.png)

<p align="center"><img src="docs/img/client-phone.png" width="760" alt="What the client sees on an iPhone: the picture with Turn sound on, Mark a point, Fullscreen and Hide buttons"><br>
<sub>What the client sees — just a link opened in Safari or Chrome.</sub></p>

> Screenshots are taken from the real interface on a test bench. Links, audio devices, monitors and the viewer list in them are sample data; the picture is an original synthetic frame.

## Contents

- [Why GradeStream](#why-gradestream)
- [Remote editing review: sound in sync, smooth playback](#remote-editing-review-sound-in-sync-smooth-playback)
- [Tour of the app](#tour-of-the-app)
  - [1. Control window](#1-control-window) · [2. Internet, Wi-Fi or both](#2-internet-wi-fi-or-both) · [3. Links and QR code](#3-links-and-qr-code) · [4. Always 1920×1080](#4-always-19201080) · [5. Quality](#5-quality) · [6. Audio](#6-audio) · [7. What the client sees](#7-what-the-client-sees) · [8. Scopes for the client](#8-scopes-for-the-client) · [9. Client markers](#9-client-markers) · [10. Marker screenshots and the comment summary](#10-marker-screenshots-and-the-comment-summary) · [11. Viewers](#11-viewers) · [12. Tray icon, language, closing](#12-tray-icon-language-closing)
- [Settings reference](#settings-reference)
- [How it works](#how-it-works) · [Technical specification](#technical-specification) · [Color: what to trust](#color-what-to-trust-and-what-not-to)
- [Installation](#installation) · [Quick start](#quick-start) · [Security and privacy](#security-and-privacy) · [Building from source](#building-from-source) · [Limitations](#limitations)

## Why GradeStream

- **Zero friction for the client.** One link, opened on a phone during a WhatsApp/Zoom call. No app, no account, no plug-ins.
- **Built for grading reviews.** Clean Feed monitor in, a fixed 1920×1080 picture out, color-tagged so the decoder reconstructs the RGB values Resolve outputs ([measured](#color-what-to-trust-and-what-not-to): mean ΔE2000 0.22).
- **Feedback lands on the frame.** The client taps the image and types a comment; you see it on top of your Clean Feed monitor, and a screenshot of the exact frame they meant is saved with the note.
- **Remote or in the room.** Over the internet via Cloudflare Tunnel, or straight over Wi-Fi with no internet at all — or both at once.
- **Also for editing reviews.** "Smooth & in sync" playback keeps sound in sync with the picture and frames evenly paced, so a remote director can judge cuts and timing ([measured](#remote-editing-review-sound-in-sync-smooth-playback)).

## Remote editing review: sound in sync, smooth playback

A director or producer can watch the edit remotely with sound: open the link, tap **Turn sound on**, and pick **Playback → Smooth & in sync** in GradeStream.

**How it works.** Every frame and every 20 ms of audio is stamped with its capture time on one clock (audio: the Windows audio engine's capture timestamps where it provides them; video: the moment of capture). The viewer estimates how late things arrive and how much that varies, and plays everything at *capture time + the same delay*:

- the **same delay** for sound and picture → they stay in sync;
- a **buffer sized to the network's jitter** (0.12–0.7 s, adapts automatically) → frames come out evenly, so the rhythm of cuts isn't distorted by the network;
- on Wi-Fi, the browser's own WebRTC lip-sync does the same, driven by sender reports that carry the capture clock.

**Measured** (test bench: the control window flashes white at the start of every second while the test tone beeps at the same moment; the viewer logs when the flash is drawn and when the beep reaches the audio output; network jitter is simulated by delaying each message 0–120 ms):

| | Sound vs picture (negative = sound ahead) | Frame pacing at 25 fps* |
|---|---|---|
| Internet, Smooth & in sync, no jitter | median −13…+17 ms, events −64…+29 ms | deviation median 9 ms |
| Internet, Smooth & in sync, jitter 0–120 ms | median −12…−16 ms, events −66…+1 ms | deviation median 8–10 ms, **19%** of intervals >60 ms |
| Internet, Lowest latency, jitter 0–120 ms | — (no sound) | deviation median 25–27 ms, **33–36%** of intervals >60 ms |
| Wi-Fi (WebRTC), Smooth & in sync | −4…−11 ms (browser's estimate**) | as source |
| Wi-Fi (WebRTC), Lowest latency | +17…+27 ms (browser's estimate**) | as source |

<sub>* Deviation of display intervals from 40 ms. On this bench even without any jitter 14–15% of intervals exceed 60 ms because of the synthetic source and a 60 Hz display, so 19% under jitter is close to that floor, while "Lowest latency" doubles it. ** `estimatedPlayoutTimestamp` from WebRTC stats, not a flash/beep measurement.</sub>

For reference, ITU-R BT.1359 puts the threshold of *detectability* at about 45 ms of sound ahead and 125 ms of sound behind the picture. All medians above are inside it. The measurements run automatically on GitHub on every change — see the latest results under [Actions → Build and measure](https://github.com/ushtozhish/gradestream/actions/workflows/build-and-measure.yml) (job summary).

**What this is good for:** judging the cut, pacing, performance, sound-to-picture sync, music edits, rough and fine cut reviews; several people watching at once and leaving markers on frames.

**What it is not:**

- **Not frame-accurate playback.** GradeStream captures the screen. If the monitor's refresh rate isn't a multiple of the timeline frame rate (e.g. a 23.976/24p timeline on a 60 Hz monitor), the monitor itself already shows uneven cadence (3:2 pulldown), and the stream inherits it. For the most even motion set the Clean Feed monitor to a multiple of the timeline rate where possible (48/72/120 Hz for 24p, 50/100 Hz for 25p) and GradeStream's frame rate to the timeline rate.
- **Not for judging a final mix.** Opus at 128–192 kbit/s is clean, but phone speakers and earbuds are not a mixing environment.
- **Not real-time collaboration.** Viewers watch and comment; editing still happens on your machine.
- **The sync buffer adds delay.** With Smooth & in sync, viewers see the picture 0.12–0.7 s later than with Lowest latency. Keep Lowest latency for interactive grading.
- **Real-network results not measured.** Above is a simulated jitter of 0–120 ms. On a poor mobile connection, losses make TCP stall; pauses longer than the buffer still show as freezes.

Versions before 2.11 played sound and picture independently, without synchronization, so sound lagged behind the picture.

## Tour of the app

### 1. Control window

The control window opens when you start GradeStream. The large preview shows **exactly what the client receives** — the 1920×1080 frame after cropping/scaling. Client markers appear on it too.

- **Choose screen** — pick the monitor (or a window) to stream. Choose "Entire screen" → your Clean Feed monitor.
- **Pause** — freezes the stream; the client keeps the last frame. **Stop** ends the stream.
- **Fullscreen** (or double-click the preview) — enlarges the preview.
- The top bar shows the status (live / paused / not streaming), the browser engine and capture method, the version, and the **RU/EN** language switch.

### 2. Internet, Wi-Fi or both

<img src="docs/img/panel-modes.png" width="370" align="right" alt="How viewers watch: Internet, Wi-Fi, Both">

| Mode | For | How it travels | Expected latency* |
|---|---|---|---|
| **Internet** | a client in another city | Cloudflare Tunnel (HTTPS) | ≈0.2–0.5 s on a good connection |
| **Wi-Fi** | everyone in the same room | WebRTC, directly over the local network; no internet needed | ≈0.1–0.25 s |
| **Both** | a room plus remote viewers | both at once; each frame is encoded only once | as above |

<sub>* Estimates from bench measurements plus typical network delay — see [Latency](#latency).</sub>

Switching modes only disconnects people when it has to (e.g. leaving "Both" for "Wi-Fi" drops internet viewers), and the app asks first.

<br clear="right">

### 3. Links and QR code

<table><tr>
<td width="50%" valign="top"><img src="docs/img/panel-internet-link.png" alt="Internet link panel"><br><b>Internet link</b> — a random <code>https://…trycloudflare.com/v/…</code> address. Copy it and send it to the client. It works while the app is open and changes on every launch.</td>
<td width="50%" valign="top"><img src="docs/img/panel-wifi-link.png" alt="Wi-Fi link panel with QR code"><br><b>Wi-Fi link</b> — <code>http://&lt;PC IP&gt;:8790/v/…</code> with a QR code. If the PC has several network adapters, pick the address of the Wi-Fi the phones are on.</td>
</tr></table>

Click the QR code to show it full-screen — handy for a room full of people:

<p align="center"><img src="docs/img/qr-fullscreen.png" width="640" alt="Full-screen QR code"></p>

### 4. Always 1920×1080

The client always gets a 1920×1080 frame. If your monitor isn't 1080p, GradeStream asks how to convert it and remembers the answer for that resolution (change it later with **change** under the preview).

<p align="center"><img src="docs/img/format-dialog.png" width="560" alt="Resolution dialog: crop to 16:9 and downscale, or fit with black bars"></p>

| Your monitor | Options (★ recommended) |
|---|---|
| 1920×1080 | used as is |
| 4K, 1440p and other 16:9 | ★ downscale to 1920×1080 · 1:1 center crop (pixel-for-pixel, for checking grain and noise) |
| 720p and smaller 16:9 | ★ upscale to 1920×1080 |
| 1920×1200 | ★ crop the 60-px bars top and bottom (Resolve centers a 16:9 Clean Feed there) · fit with bars |
| 2560×1600, ultrawide, others | ★ crop to 16:9 from the center → scale to 1920×1080 · fit with bars |

### 5. Quality

<img src="docs/img/panel-quality.png" width="370" align="right" alt="Quality panel: bitrate, frame rate, codec info">

- **Bitrate** — 4 / 8 / 12 / 16 / 24 Mbit/s. 8–12 suits a phone; use 4 for a weak connection, 16–24 when gradients and fine detail matter and the network allows.
- **Frames/s** — 12 / 24 / 25 / 30 / 50 / 60. At 12 fps each frame gets about twice the bits of 25 fps: cleaner shadows and gradients while you tweak, choppier playback. Change it live.
- **Playback** — **Lowest latency** (default, for grading: frames are shown the moment they arrive) or **Smooth & in sync** (for editing reviews: a 0.12–0.7 s buffer, even frame pacing, sound in sync). Sound is always synced when a viewer turns it on.
- The line below shows the codec and whether the **hardware** encoder (NVENC / Quick Sync / AMF) is in use.

<br clear="right">

### 6. Audio

<img src="docs/img/panel-audio.png" width="370" align="right" alt="Audio panel: source selector, bitrate and level meter">

- **No audio** — all bandwidth goes to the picture (default).
- **Output device** — everything playing on a Windows output device (e.g. your speakers).
- **Single application** — only one app and its child processes, e.g. only Resolve: no messenger pings or browser sounds reach the client (Windows 10 2004+).
- **Test tone** — a 1 kHz beep at the start of every second, on the same clock as the picture: check that sound reaches the client and that it lands together with what's on screen.
- **Audio bitrate** 64 / 128 / 192 kbit/s (Opus), plus a level meter. The choice is remembered; an app is found again by name after a restart.

The client hears sound after tapping **Turn sound on** (phones only allow audio after a tap). Over the internet, no audio data is sent to a client until they tap it; on Wi-Fi the audio travels with the video and is simply unmuted.

<br clear="right">

### 7. What the client sees

<p align="center"><img src="docs/img/client-phone-mark.png" width="760" alt="Client's phone in marker mode: Tap a spot on the frame"></p>

| Button | What it does |
|---|---|
| **Turn sound on** | starts audio (shown only when you stream audio) |
| **Mark a point** | then tap the frame and type a comment — it reaches you instantly |
| **Fullscreen** | full-screen picture; on iPhone it uses the system video player |
| **Scopes** | waveform, parade, vectorscope, histogram — [see below](#8-scopes-for-the-client) (shown when you allow it) |
| **i** | live stats: resolution, fps, bitrate (and latency on Wi-Fi) |
| **Hide** | hides the buttons; tap the screen to bring them back |

The page follows the language of the client's phone (English or Russian). With the internet link it also keeps the phone screen from dimming.

### 8. Scopes for the client

The client can open **waveform, RGB parade, vectorscope and histogram** right on the phone — the button **Scopes** in the bottom bar. You decide whether clients get them: **Quality → Client scopes → Allowed / Off** (switching off closes them on every phone at once).

Two layouts, one tap apart:

| Big picture (scopes in a corner) | Big scopes (picture in a small window) |
|---|---|
| <img src="docs/img/scopes-corner.png" width="420" alt="Waveform and vectorscope in a small window over the picture"> | <img src="docs/img/scopes-big.png" width="420" alt="Waveform, parade and vectorscope full screen, picture in a small window"> |

- **Big picture** — a small window over the frame with any set of scopes; drag it by its top row, sizes S / M / L / XL.
- **Big scopes** — scopes fill the screen, the picture shrinks to a small window (on the right when the phone is sideways, on top when upright). Markers can still be placed on the small picture.
- Waveform in **Y** (Rec.709 luma) or **RGB** overlay; vectorscope at **1× / 2× / 4×** with 75% color-bar targets, 100% dots and the skin-tone line; trace brightness in three steps. The 0–1023 scale matches Resolve.

<p align="center"><img src="docs/img/scopes-big-portrait.png" width="300" alt="Big scopes with the phone upright"></p>

**What the scopes show, honestly.** They are computed on the phone from **the picture that arrived** — 8-bit, after H.264 — from a 512×576 copy of the frame, up to 10 times a second (less often on a slow phone). So they describe what the client actually receives, not Resolve's internal 32-bit image:

- levels, balance and saturation agree with the source within about 1%: in the automated test (75% color bars + gray ramp, both paths) bar levels on the waveform are off by at most 1.2 of 255 and vectorscope traces sit within 2.2 px of the targets (the target box is ±5 px);
- tiny isolated highlights may be averaged away by the downscaled copy;
- smooth synthetic gradients show fine horizontal combing — that is the real distribution of 8-bit levels after compression, which Resolve's scopes (fed before compression) don't have;
- for grading decisions use Resolve's own scopes; the client scopes are for talking about the same picture with the same tools.

### 9. Client markers

<p align="center"><img src="docs/img/preview-markers.png" width="760" alt="Preview with two client markers and their comments"></p>

<img src="docs/img/panel-markers.png" width="370" align="right" alt="Client markers panel: overlay, monitor, screenshot folder and marker list">

When the client places a marker you get:

- a **beep** and the marker with its comment on the preview;
- the same marker **on top of your Clean Feed monitor** for 20 s. The overlay window is excluded from screen capture, so the client never sees it in the stream. Pick the right monitor in **Monitor** (it is auto-detected by resolution). **Clear markers from screen** removes them at once;
- a line in the marker list with time, device, pixel position and comment.

<br clear="right">

### 10. Marker screenshots and the comment summary

Every marker saves a PNG of **the frame the client was actually looking at** — the app keeps the last ~4.5 s of frames and compensates for each viewer's latency — with the marker, the number and the comment burned in:

![Saved marker screenshot](docs/img/marker-snapshot.png)

All markers of a session are collected in `comments.html` (with "Done" checkboxes, auto-refreshing every 10 s, so you can keep it open during the session) and `comments.txt`:

![Comment summary page](docs/img/comments-summary.png)

- **Choose folder…** — where to save (remembered; default `Documents\GradeStream`).
- **A separate subfolder for each session** — on by default; off writes into the chosen folder with the date in file names. Nothing is ever overwritten.
- **Open folder** / **All comments** — open the folder or the summary page.

### 11. Viewers

<img src="docs/img/panel-viewers.png" width="370" align="right" alt="Viewers panel: three devices with path and bitrate">

Each viewer is listed with the device type, how they're connected (via internet / Wi-Fi direct), the bitrate they actually receive, and whether they turned the sound on. If a client's connection can't keep up, a warning suggests lowering the bitrate.

<br clear="right">

### 12. Tray icon, language, closing

- GradeStream has its **own taskbar icon** (even though it uses the Edge/Chrome engine) and an icon **near the clock**: click to bring the window back; right-click for **Show window / Stop streaming / Exit**. The tooltip shows "live, viewers: N".
- Minimizing during a live stream shows a one-time notification about where the app went.
- Closing the window while live asks for confirmation; **Exit** stops the stream, the tunnel and audio capture in one go.
- **RU/EN** in the top bar switches the language without interrupting the stream. By default the app follows the Windows language.

## Settings reference

All settings are remembered between launches.

| Setting | Where | Values | Default |
|---|---|---|---|
| How viewers watch | panel "How viewers watch" | Internet · Wi-Fi · Both | Internet |
| Resolution conversion | dialog after choosing a screen; **change** under the preview | as is · downscale · upscale · 1:1 center crop · crop to 16:9 · fit with bars | recommended option for your monitor |
| Bitrate | Quality | 4 / 8 / 12 / 16 / 24 Mbit/s | 8 Mbit/s |
| Frames per second | Quality | 12 / 24 / 25 / 30 / 50 / 60 | 25 |
| Playback on viewers | Quality | Lowest latency · Smooth & in sync | Lowest latency |
| Client scopes | Quality | Allowed · Off | Allowed |
| Audio source | Audio | none · output device · single application · test tone | none |
| Audio bitrate | Audio | 64 / 128 / 192 kbit/s | 128 kbit/s |
| Markers on top of the Clean Feed | Client markers | on / off · monitor | on · auto-detected |
| Save a screenshot of every marker | Client markers | on / off | on |
| Screenshot folder | Client markers → Choose folder… | any folder | `Documents\GradeStream` |
| Subfolder per session | Client markers | on / off | on |
| Wi-Fi address | Wi-Fi link (shown when the PC has several networks) | the PC's local IP addresses | the main network |
| Interface language | RU/EN in the top bar | English · Русский | Windows language |

## How it works

```
┌──────────────────── your PC (Windows) ────────────────────┐
│ Clean Feed monitor ── screen capture (Windows Graphics     │
│                       Capture, 8-bit RGB)                  │
│        ↓                                                   │
│ GradeStream window (Edge/Chrome engine):                   │
│   crop/scale → 1920×1080 frame → H.264 (WebCodecs,         │
│   hardware encoder if available) ; audio → Opus            │
│        ↓                                                   │
│ GradeStream.exe — serves the stream to viewers             │
└──────┬───────────────────────────────────────┬─────────────┘
       │ Internet                              │ Wi-Fi
       ↓                                       ↓
 cloudflared → Cloudflare → HTTPS       WebRTC directly over
 (WebSocket, TCP)                       the local network (UDP)
       ↓                                       ↓
 phone: WebCodecs decoder               phone: built-in
 → canvas                               WebRTC player
```

- Each frame is encoded **once** and sent to all viewers on both paths, so the load barely grows with the number of viewers.
- There is no server of ours on the internet: the server is the app on your PC. Cloudflare only makes it reachable through an HTTPS link (no router ports to open).
- On the local network the page opens at `http://<PC IP>:8790/...`. Browsers expose WebCodecs only over HTTPS, so on Wi-Fi the video goes over WebRTC instead: it works without HTTPS and has the lowest latency.

## Technical specification

### Video

| Parameter | Value |
|---|---|
| Output resolution | always 1920×1080 |
| Frame rate | 12, 24, 25, 30, 50, 60 |
| Codec | H.264. Internet mode — High Profile; Wi-Fi and Both modes — Constrained Baseline (guaranteed to play over WebRTC on any phone) |
| Bit depth and chroma | 8-bit, YCbCr 4:2:0 |
| Bitrate | 4, 8, 12, 16, 24 Mbit/s (selectable) |
| Color tagging of the stream | BT.709 primaries, BT.709 matrix, **full range** (0–255), **sRGB** transfer (IEC 61966-2-1) |
| Keyframes | at least every 3 s and on request when a viewer joins |
| Congestion | extra frames are dropped until the next keyframe (latency does not build up) |

### Audio

| Parameter | Value |
|---|---|
| Source | a Windows output device (WASAPI loopback) or one application with its child processes (process loopback, Windows 10 2004+) |
| Codec | Opus, 48 kHz, stereo, 64/128/192 kbit/s |
| Phones without an Opus decoder | uncompressed PCM 24 kHz (internet mode) |

### Latency

**Measured** (test bench, one machine, software encoder, no real network; [details](docs/MEASUREMENTS.md)). From drawing a frame to showing it at the viewer, in the default **Lowest latency** playback (**Smooth & in sync** adds its 0.12–0.7 s buffer on top):

| Path | Median | Range |
|---|---|---|
| Internet path without the network itself (encode → server → WebSocket → decode) | 80–125 ms (over five runs) | 34–185 ms |
| Wi-Fi path (encode → WebRTC → jitter buffer → decode) | 150 ms | 122–173 ms |

**Not measured**, but added in real use:

- Windows screen capture — usually 1–2 frames (17–33 ms at 60 Hz).
- Display on the phone screen — 1–2 frames.
- **Wi-Fi:** the network itself adds a few milliseconds. Expect roughly 0.1–0.25 s from monitor to phone.
- **Internet:** the trip to the nearest Cloudflare data center from you and from the client, plus TCP. On a good connection expect 0.2–0.5 s. On a poor mobile network TCP retransmits lost packets, and the picture may freeze until the next keyframe.

There are no "monitor to a client's phone in another city" numbers here — we have not measured that.

### Color: what to trust and what not to

**Measurement.** A 41-patch chart went through the real pipeline: gray ramp 0–255, primaries and secondaries at 100% and 75%, 12 ColorChecker patches. Result for both paths ([all patches](docs/MEASUREMENTS.md)):

| | Internet | Wi-Fi |
|---|---|---|
| Mean ΔE2000 | 0.22 | 0.22 |
| Max ΔE2000 | 0.75 (cyan 75%) | 0.75 |
| Gray neutrality (max chroma) | 0.01 | 0.01 |
| Black / white | 0 / 255 — no range compression | same |

The just-noticeable difference for a human is about ΔE2000 ≈ 1.0. The residual error is rounding in the RGB → 8-bit YCbCr → RGB conversion, not compression. On flat areas H.264 at these bitrates loses almost nothing, and full range with explicit color tagging keeps the decoder from getting levels or the matrix wrong.

**You can trust:**

- **Signal values.** The RGB values Resolve sends to the Clean Feed monitor reach the phone's decoder with an error below the visibility threshold on flat areas.
- **Relationships:** white balance, gray neutrality, skin tones, relative saturation, before/after differences.
- **The capture.** It takes the frame before the GPU/monitor calibration LUT. The stream carries the values Resolve outputs, not how your monitor displays them — which is correct for judging the signal.

**You cannot trust:**

1. **The phone screen as a reference.** This is the biggest source of mismatch — bigger than the whole transmission. Phones aren't calibrated; True Tone, Night Shift, auto-brightness and power saving change white point and brightness; the client's viewing environment can be anything.
2. **Gamma.** The stream is tagged sRGB, and the phone displays it with the sRGB curve (≈2.2), not BT.1886 (2.4) like a reference monitor in a dark room. So shadows and low midtones look lighter and softer on the phone. That's how display works, not a transmission error.
3. **HDR and 10-bit.** Only 8-bit SDR is transmitted. If HDR or "Automatically manage color for apps" is enabled in Windows for the Clean Feed monitor, the captured values are already converted — turn these off for that monitor.
4. **Fine detail and gradients.** 4:2:0 halves color resolution: thin colored edges, titles and fine saturated texture get softer. Smooth gradients (skies, vignettes) can band at low bitrates — that's 8-bit plus compression, not the grade. Grain and noise in motion get smoothed. On pause, the picture sharpens with each keyframe.
5. **Scale.** From a 4K monitor the client sees 1080p. Use the 1:1 center-crop option to check detail.

**Not tested** (to be clear): Windows hardware encoders (NVENC, Quick Sync, AMF), decoding in Safari on iPhone and Chrome on Android, capture of a real Windows screen. The measurement was done in Chromium with a software encoder on a synthetic chart.

**Bottom line.** The color that reaches the phone can be trusted **as a signal**: in our measurements the error is below the visibility threshold. The phone screen cannot be trusted **as a reference display**. For final color approval you need a calibrated monitor or a rendered file. GradeStream is for discussing changes in real time, not for final sign-off.

### Check it yourself

1. Put a gray ramp and color patches with known values on the Clean Feed output in Resolve.
2. Ask the client to place a marker. GradeStream saves a PNG of exactly what went into the encoder — check with a color picker that the values match the source. This verifies the capture.
3. Take a screenshot on the phone. Open it in a color-managed app (e.g. Photoshop: "Convert to Profile → sRGB") and compare values. This verifies the whole chain, including the phone's decoder.

## Installation

1. Download `GradeStream-Setup-<version>.zip` from [Releases](https://github.com/ushtozhish/gradestream/releases/latest), unzip it and run the installer.
2. If Windows shows "Windows protected your PC", click "More info" → "Run anyway". The installer isn't signed with a developer certificate.
3. No admin rights needed. The app installs to `%LOCALAPPDATA%\Programs\GradeStream` and appears in the Start menu, on the desktop and in "Installed apps", where you can uninstall it.

Requires Microsoft Edge (preinstalled on Windows) or Google Chrome — GradeStream uses their engine for capture and encoding. If the version is too old, the app tells you what to update.

## Quick start

1. In Resolve: **Workspace → Video Clean Feed →** the monitor for the client.
2. In GradeStream, choose a mode: "Internet", "Wi-Fi" or "Both".
3. Click "Choose screen" → "Entire screen" → the Clean Feed monitor. Keep the GradeStream window itself on another monitor.
4. Send the client the link, or show the QR code to people in the room.

**Wi-Fi mode.**

- The first time, Windows asks to allow GradeStream through the firewall — click "Allow".
- If the network is marked "Public" in Windows, the app warns you: switch it to "Private".
- Guest networks with client isolation (hotels, cafés) won't work.

## Security and privacy

- The link contains a random key and stops working when the app is closed. A new link is generated on every launch.
- The control window opens only on your PC.
- **Internet mode:** the stream is encrypted (HTTPS) on both legs, but not end-to-end — Cloudflare decrypts it to forward it. Video is never recorded or stored. For material under a strict NDA, keep this in mind or use Wi-Fi mode.
- Cloudflare's account-less quick tunnels come **with no uptime guarantee**. They usually work fine but occasionally fail to start or drop. The app reconnects on its own, but the link changes.
- **Wi-Fi mode:** the stream never leaves the local network. WebRTC transport is encrypted (DTLS-SRTP).

## Building from source

Requires [Go 1.27+](https://go.dev/dl/) and internet access.

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File build\build.ps1
```
```bash
# Linux / macOS (cross-compiling for Windows)
./build/build.sh
```

The script downloads the official cloudflared, verifies Cloudflare's signature, builds `app/` and `installer/`, and puts the installer in `dist/`.

| Folder | Contents |
|---|---|
| `app/` | the app: server, tunnel, WebRTC, audio capture (WASAPI), on-screen markers, tray; pages `web/host.html` (control window) and `web/view.html` (viewer) |
| `installer/` | per-user installer (no admin rights), "Installed apps" registration, shortcuts |
| `tests/` | measurements: `colortest.js` (color, ΔE2000), `avtest.js` (sound/picture sync, frame pacing), `run-all.sh` (runs everything) — also run by GitHub Actions on every change |
| `tools/mkicon.py` | icon and Windows resource generation |

## Limitations

- Windows 10/11 only (audio capture, on-screen markers and the tray use Windows APIs).
- One monitor or one window is streamed.
- 8-bit SDR, 1080p output only.
- On-screen markers require Windows 10 2004+ and "Entire screen" capture. If Resolve shows the Clean Feed in exclusive fullscreen, the marker overlay can't appear on top of it.

## License

[MIT](LICENSE). Third-party components (cloudflared — Apache 2.0, Pion WebRTC — MIT, and others) are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
