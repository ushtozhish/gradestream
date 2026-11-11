# Color and latency measurements

How the numbers in the README were obtained. You can repeat the measurement with [`tests/colortest.js`](../tests/colortest.js).

## Test bench

- The real GradeStream pipeline: control window → scale/crop into a 1920×1080 frame → H.264 (WebCodecs) → GradeStream server → viewers.
- Two viewers at once ("Both" mode): over the internet path (WebSocket + WebCodecs) and over the Wi-Fi path (WebRTC).
- Instead of a Windows screen capture, a test chart is fed into the control window as a video stream (`canvas.captureStream`).
- Chromium 153 on Linux, **software** H.264 encoder, Constrained Baseline profile (`avc1.42e02a`), 50 fps, 8 Mbit/s. One machine — no network between sender and viewers.
- Chart: gray ramp 0–255 (17 steps), primaries and secondaries at 100% and 75%, 12 ColorChecker patches (sRGB). Each patch is 240×150 px; the central 60×40 px of the decoded frame is averaged.
- Error is ΔE2000 between the source and received sRGB value. The human just-noticeable difference is about 1.0.

## Summary

| Path | Mean ΔE2000 | Max ΔE2000 | Gray: max chroma (a\*b\*) | Black / white out | Latency (median, range) |
|---|---|---|---|---|---|
| Internet (WebCodecs) | 0.22 | 0.75 | 0.01 | 0,0,0 / 255,255,255 | 107 ms (69–146) |
| Wi-Fi (WebRTC) | 0.22 | 0.75 | 0.01 | 0,0,0 / 255,255,255 | 150 ms (122–173) |

Latency here is from drawing a frame with a binary counter to reading that counter from the frame at the viewer, on one machine. Real network time (Wi-Fi, internet, Cloudflare) is **not** included. Across five runs the internet-path median was 78–125 ms.

Runs at 4 and 16 Mbit/s gave the same ΔE on the internet path (0.22 / 0.75): on flat static areas bitrate barely affects color.

## Per patch (8 Mbit/s)

| Patch | Source RGB | Internet: RGB | ΔE | Wi-Fi: RGB | ΔE |
|---|---|---|---|---|---|
| gray 0 | 0, 0, 0 | 0, 0, 0 | 0.00 | 0, 0, 0 | 0.00 |
| gray 16 | 16, 16, 16 | 16, 16, 16 | 0.00 | 16, 16, 16 | 0.00 |
| gray 32 | 32, 32, 32 | 32, 32, 32 | 0.00 | 32, 32, 32 | 0.00 |
| gray 48 | 48, 48, 48 | 48, 48, 48 | 0.00 | 48, 48, 48 | 0.00 |
| gray 64 | 64, 64, 64 | 64, 64, 64 | 0.00 | 64, 64, 64 | 0.00 |
| gray 80 | 80, 80, 80 | 80, 80, 80 | 0.00 | 80, 80, 80 | 0.00 |
| gray 96 | 96, 96, 96 | 96, 96, 96 | 0.00 | 96, 96, 96 | 0.00 |
| gray 112 | 112, 112, 112 | 112, 112, 112 | 0.00 | 112, 112, 112 | 0.00 |
| gray 128 | 128, 128, 128 | 128, 128, 128 | 0.00 | 128, 128, 128 | 0.00 |
| gray 144 | 144, 144, 144 | 144, 144, 144 | 0.00 | 144, 144, 144 | 0.00 |
| gray 160 | 160, 160, 160 | 160, 160, 160 | 0.00 | 160, 160, 160 | 0.00 |
| gray 176 | 176, 176, 176 | 176, 176, 176 | 0.00 | 176, 176, 176 | 0.00 |
| gray 192 | 192, 192, 192 | 192, 192, 192 | 0.00 | 192, 192, 192 | 0.00 |
| gray 208 | 208, 208, 208 | 208, 208, 208 | 0.00 | 208, 208, 208 | 0.00 |
| gray 224 | 224, 224, 224 | 224, 224, 224 | 0.00 | 224, 224, 224 | 0.00 |
| gray 240 | 240, 240, 240 | 240, 240, 240 | 0.00 | 240, 240, 240 | 0.00 |
| gray 255 | 255, 255, 255 | 255, 255, 255 | 0.00 | 255, 255, 255 | 0.00 |
| R 100% | 255, 0, 0 | 254, 0, 0 | 0.21 | 254, 0, 0 | 0.21 |
| G 100% | 0, 255, 0 | 0, 255, 0 | 0.00 | 0, 255, 0 | 0.00 |
| B 100% | 0, 0, 255 | 0, 1, 255 | 0.05 | 0, 1, 255 | 0.05 |
| C 100% | 0, 255, 255 | 0, 255, 253 | 0.62 | 0, 255, 253 | 0.62 |
| M 100% | 255, 0, 255 | 254, 1, 255 | 0.17 | 254, 1, 255 | 0.17 |
| Y 100% | 255, 255, 0 | 253, 255, 0 | 0.41 | 253, 255, 0 | 0.41 |
| R 75% | 191, 0, 0 | 190, 0, 0 | 0.20 | 190, 0, 0 | 0.20 |
| G 75% | 0, 191, 0 | 0, 192, 0 | 0.27 | 0, 192, 0 | 0.27 |
| B 75% | 0, 0, 191 | 0, 0, 191 | 0.00 | 0, 0, 191 | 0.00 |
| C 75% | 0, 191, 191 | 0, 192, 190 | 0.75 | 0, 192, 190 | 0.75 |
| M 75% | 191, 0, 191 | 190, 0, 190 | 0.24 | 190, 0, 190 | 0.24 |
| Y 75% | 191, 191, 0 | 190, 191, 0 | 0.24 | 190, 191, 0 | 0.24 |
| dark skin | 115, 82, 68 | 115, 82, 68 | 0.00 | 115, 82, 68 | 0.00 |
| light skin | 194, 150, 130 | 193, 150, 130 | 0.31 | 193, 150, 130 | 0.31 |
| blue sky | 98, 122, 157 | 98, 123, 157 | 0.61 | 98, 123, 157 | 0.61 |
| foliage | 87, 108, 67 | 87, 109, 66 | 0.62 | 87, 109, 66 | 0.62 |
| blue flower | 133, 128, 177 | 133, 129, 176 | 0.57 | 133, 129, 176 | 0.57 |
| bluish green | 103, 189, 170 | 101, 189, 169 | 0.40 | 101, 189, 169 | 0.40 |
| orange | 214, 126, 44 | 212, 126, 43 | 0.57 | 212, 126, 43 | 0.57 |
| purplish blue | 80, 91, 166 | 78, 92, 165 | 0.69 | 78, 92, 165 | 0.69 |
| moderate red | 193, 90, 99 | 192, 90, 97 | 0.66 | 192, 90, 97 | 0.66 |
| purple | 94, 60, 108 | 93, 61, 108 | 0.49 | 93, 61, 108 | 0.49 |
| yellow green | 157, 188, 64 | 156, 188, 62 | 0.25 | 156, 188, 62 | 0.25 |
| orange yellow | 224, 163, 46 | 222, 163, 44 | 0.65 | 222, 163, 44 | 0.65 |

## What this measurement does NOT cover

- Windows hardware encoders (NVIDIA NVENC, Intel Quick Sync, AMD AMF via Media Foundation) — a software encoder was used.
- Decoding and display in Safari on iPhone and Chrome on Android — Chromium was used.
- Windows screen capture (Windows Graphics Capture) — a synthetic video stream was used.
- Motion, grain, fine detail and smooth gradients — static flat patches were used.
- Real network latency.

So these numbers describe the accuracy of the encode → transport → decode chain itself, not what a client will see on their phone. How to check your whole chain — see "Check it yourself" in the README.
