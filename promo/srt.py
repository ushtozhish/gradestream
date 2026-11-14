def ts(x):
    h=int(x//3600); m=int(x%3600//60); s=x%60
    return f"{h:02}:{m:02}:{int(s):02},{int(round((s-int(s))*1000)):03}"
def write(path, items):
    with open(path,'w',encoding='utf-8') as f:
        for i,(a,b,t) in enumerate(items,1): f.write(f"{i}\n{ts(a)} --> {ts(b)}\n{t}\n\n")
L=[
(0.8,5.6,"GradeStream\nYour DaVinci Resolve grade on the client's phone. Just a link."),
(6.3,10.5,"How it works: Clean Feed monitor → GradeStream on your PC\n→ a link → the client's phone"),
(10.5,15.1,"The client installs nothing —\nno apps, no accounts"),
(15.9,18.0,"Pick the Clean Feed monitor — and you're live"),
(18.0,19.6,"1. Internet, Wi-Fi or both at once"),
(19.6,21.2,"2. A link for the client and a QR code for the room"),
(21.2,23.6,"3. The preview is exactly what the client receives"),
(24.3,27.5,"Internet, Wi-Fi, or both at once"),
(27.5,32.1,"Internet: client in another city, ≈0.2–0.5 s · Wi-Fi: same room, no internet, ≈0.1–0.25 s\nBoth: each frame is encoded once for everyone"),
(32.8,35.5,"Color arrives as a signal —\nthe same RGB values Resolve outputs"),
(35.5,38.0,"Average ΔE2000 0.22 — the visibility threshold is about 1.0"),
(38.0,41.1,"The phone screen is not a reference.\nGradeStream is for discussing changes, not final color approval."),
(41.8,45.3,"Notes right on the frame.\n1. The client taps the frame and writes a comment"),
(45.3,47.5,"2. The marker reaches you: on the preview and over your Clean Feed monitor"),
(47.5,51.6,"3. A snapshot of that exact frame and a summary of all comments are saved"),
(52.3,57.0,"Scopes for the client: Waveform, Parade, Vectorscope.\nComputed on the phone; you decide if they're allowed."),
(57.0,61.6,"Out of range — the scope frame flashes red:\nclipped highlights, crushed blacks, saturation over 100%"),
(62.3,68.6,"Vertical video — 1080×1920.\nRotate the Clean Feed monitor in Windows, and the stream is vertical too."),
(69.3,73.0,"Reviewing an edit: sound in sync.\n\"Smooth & in sync\" mode — for directors and producers"),
(73.0,77.1,"Measured: median −16…+17 ms,\nwell inside what viewers can notice (ITU-R BT.1359)"),
(77.8,81.5,"Full screen with markers on iPhone — in Safari.\nSettings → Apps → Safari → Advanced → Feature Flags → Fullscreen API"),
(81.5,85.1,"Open the link from the messenger in Safari.\nNothing to set up on Android, iPad or a computer."),
(85.8,91.5,"GradeStream — free, open source, Windows 10/11\ngithub.com/ushtozhish/gradestream"),
]
S=[
(0.5,4.2,"GradeStream\nYour Resolve grade on the client's phone. Just a link."),
(4.8,8.5,"How it works: monitor → GradeStream → link → phone"),
(8.5,11.2,"The client installs nothing"),
(11.8,15.0,"Notes right on the frame: the client taps and writes"),
(15.0,19.7,"The marker reaches you, with a snapshot of that frame"),
(20.3,24.2,"Scopes on the client's phone:\nWaveform, Parade, Vectorscope"),
(24.2,27.7,"Out of range — the scope frame flashes red"),
(28.3,32.7,"Vertical video — 1080×1920"),
(33.3,39.2,"Color arrives as a signal: average ΔE2000 0.22\n(visibility threshold ≈ 1.0)"),
(39.8,44.2,"GradeStream — free, open source\ngithub.com/ushtozhish/gradestream"),
]
write('/tmp/claude-0/vid/out/GradeStream_EN.srt',L); write('/tmp/claude-0/vid/out/GradeStream_Shorts_EN.srt',S)
