// Measures audio/video sync and frame pacing through the real GradeStream pipeline.
// The control window flashes white at the start of every second; the 1 kHz test tone beeps at the same moment.
// Internet path: the viewer logs when the flash is drawn and when the beep reaches the audio output (Web Audio output timestamp).
// Wi-Fi path: the browser's own estimatedPlayoutTimestamp for audio and video (WebRTC stats).
// Env: PLAY=live|sync, SOUND=1|0, VPATH=net|lan, SECS, J (jitter label), GS_LOG.
const fs = require('fs'); const { chromium } = require('playwright');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const MODE = process.env.PLAY || 'live', SOUND = process.env.SOUND !== '0', PATH_ = process.env.VPATH || 'net', SECS = +(process.env.SECS || 12);
(async () => {
  const log = fs.readFileSync(process.env.GS_LOG || 'gs.log', 'utf8');
  const key = log.match(/key=([0-9a-f]+)/)[1], room = log.match(/room ([0-9a-f]+)/)[1];
  const b = await chromium.launch({ args: ['--no-proxy-server', '--autoplay-policy=no-user-gesture-required'] });
  const hctx = await b.newContext({ locale: 'en-US' });
  await hctx.addInitScript(() => {
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas'); c.width = 1920; c.height = 1080; const g = c.getContext('2d');
      setInterval(() => { const on = Date.now() % 1000 < 100; g.fillStyle = on ? '#fff' : '#202020'; g.fillRect(0, 0, 1920, 1080); }, 10);
      return c.captureStream(25);
    };
  });
  const host = await hctx.newPage();
  await host.goto(`http://127.0.0.1:8787/host?key=${key}`); await sleep(800);
  if (PATH_ === 'lan') { await host.click('text=Wi-Fi'); await sleep(1500); }
  await host.selectOption('#playout', MODE);
  await host.click('#btnStart'); await sleep(1200);
  if (SOUND) { await host.selectOption('#audioSrc', 'test'); await host.dispatchEvent('#audioSrc', 'change'); await sleep(800); }
  const link = PATH_ === 'lan' ? await host.evaluate(() => document.getElementById('lanLink').value) : `http://127.0.0.1:8787/v/${room}`;
  const vctx = await b.newContext({ locale: 'en-US' });
  await vctx.addInitScript(() => {
    window.__av = { flash: [], beep: [], shown: [] }; let prevBright = false, prevLoud = false;
    window.__avProbe = (k, ts, a, buf, actx) => {
      if (k === 'v') {
        const px = document.getElementById('c').getContext('2d').getImageData(960, 540, 1, 1).data[0];
        const br = px > 128; if (br && !prevBright) window.__av.flash.push(a); prevBright = br; window.__av.shown.push(a);
      } else {
        const d = buf.getChannelData(0); let idx = -1, mx = 0;
        for (let i = 0; i < d.length; i++) { const v = Math.abs(d[i]); if (v > mx) mx = v; if (idx < 0 && v > 0.02) idx = i; }
        if (idx >= 0 && !prevLoud) { const t = a + idx / buf.sampleRate; const o = actx.getOutputTimestamp();
          window.__av.beep.push(o.performanceTime + (t - o.contextTime) * 1000); }
        prevLoud = mx > 0.02 && idx >= 0 && d[d.length - 1] !== 0 ? true : mx > 0.02;
      }
    };
  });
  const v = await vctx.newPage(); v.on('pageerror', e => console.log('VIEW EXC', e.message));
  await v.goto(link); await sleep(2500);
  if (SOUND) { await v.click('#btnSound'); }
  await sleep(SECS * 1000);
  let res;
  if (PATH_ === 'net') {
    res = await v.evaluate(() => {
      const { flash, beep, shown } = window.__av; const off = [];
      for (const f of flash) { let best = null; for (const bb of beep) if (Math.abs(bb - f) < 500 && (best === null || Math.abs(bb - f) < Math.abs(best - f))) best = bb; if (best !== null) off.push(best - f); }
      const iv = []; for (let i = 1; i < shown.length; i++) iv.push(shown[i] - shown[i - 1]);
      return { off, iv, flashes: flash.length, beeps: beep.length, delay: Math.round((playDelay || 0) / 1000) };
    });
  } else {
    res = await v.evaluate(async () => {
      const samples = []; const iv = [];
      let last = null; const lv = document.getElementById('lv');
      await new Promise(done => { const cb = (now, md) => { if (last !== null) iv.push(md.expectedDisplayTime - last); last = md.expectedDisplayTime; if (iv.length < 200) lv.requestVideoFrameCallback(cb); else done(); }; lv.requestVideoFrameCallback(cb); setTimeout(done, 9000); });
      for (let k = 0; k < 10; k++) { const st = await pc.getStats(); let a = null, vv = null;
        st.forEach(r => { if (r.type === 'inbound-rtp' && r.estimatedPlayoutTimestamp) { if (r.kind === 'audio') a = r.estimatedPlayoutTimestamp; else vv = r.estimatedPlayoutTimestamp; } });
        if (a && vv) samples.push(vv - a); await new Promise(r => setTimeout(r, 300)); }
      return { off: samples, iv, stats: true };
    });
  }
  const stat = arr => { if (!arr.length) return 'no data'; const s = [...arr].sort((x, y) => x - y); const med = s[s.length >> 1]; return `median ${med.toFixed(0)} ms, range ${s[0].toFixed(0)}…${s[s.length - 1].toFixed(0)} ms (n=${s.length})`; };
  const iv = res.iv.filter(x => x > 0 && x < 1000); const dev = iv.map(x => Math.abs(x - 40));
  const stutter = iv.filter(x => x > 60).length;
  console.log(`[${PATH_ === 'lan' ? 'Wi-Fi/WebRTC' : 'internet/WebCodecs'} · playback=${MODE}${SOUND ? ' · sound on' : ''} · network jitter 0–${process.env.J || 0} ms] sound−picture: ${stat(res.off)}${res.delay && (MODE === 'sync' || SOUND) ? ' · buffer ~' + res.delay + ' ms' : ''}`);
  console.log(`   frame pacing (25 fps, ideal 40 ms): deviation ${stat(dev)}; intervals >60 ms: ${stutter} of ${iv.length}`);
  await b.close(); process.exit(0);
})();
