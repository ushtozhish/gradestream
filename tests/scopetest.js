// Checks viewer scopes (waveform, parade, vectorscope, histogram) against a known test image
// sent through the real GradeStream pipeline, for both the internet and the Wi-Fi path.
// Run like colortest.js: GS_LOG=gs.log node scopetest.js   (SHOTS=dir saves screenshots)
const fs = require('fs'); const { chromium } = require('playwright');
const sleep = ms => new Promise(r => setTimeout(r, ms));
// 75% color bars (top 60%), gray ramp 0→255 (bottom 40%)
const BARS = [['W75', [191,191,191]], ['Yl', [191,191,0]], ['Cy', [0,191,191]], ['G', [0,191,0]], ['Mg', [191,0,191]], ['R', [191,0,0]], ['B', [0,0,191]]];
const Y709 = ([r, g, b]) => 0.2126 * r + 0.7152 * g + 0.0722 * b;
const CbCr = c => { const y = Y709(c); return [(c[2] - y) / 1.8556 / 255, (c[0] - y) / 1.5748 / 255]; };

(async () => {
  const log = fs.readFileSync(process.env.GS_LOG || 'gs.log', 'utf8');
  const key = log.match(/key=([0-9a-f]+)/)[1], room = log.match(/room ([0-9a-f]+)/)[1];
  const b = await chromium.launch({ args: ['--no-proxy-server', '--autoplay-policy=no-user-gesture-required'] });
  const ctx = await b.newContext({ viewport: { width: 1360, height: 900 } });
  await ctx.addInitScript(BARS => {
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas'); c.width = 1920; c.height = 1080; const g = c.getContext('2d');
      const bw = 1920 / BARS.length;
      BARS.forEach(([, col], i) => { g.fillStyle = `rgb(${col})`; g.fillRect(Math.round(i * bw), 0, Math.ceil(bw), 648); });
      for (let x = 0; x < 1920; x++) { const v = Math.round(x / 1919 * 255); g.fillStyle = `rgb(${v},${v},${v})`; g.fillRect(x, 648, 1, 432); }
      setInterval(() => { g.fillStyle = '#000'; g.fillRect(0, 1079, 1, 1); }, 40);   // keep frames flowing
      return c.captureStream(25);
    };
  }, BARS);
  const host = await ctx.newPage();
  await host.goto(`http://127.0.0.1:8787/host?key=${key}`);
  await sleep(700);
  await host.click('label:has(input[name=mode][value=both])'); await sleep(1500);
  await host.selectOption('#bitrate', '8000000');
  const lanLink = await host.evaluate(() => document.getElementById('lanLink').value);
  await host.click('#btnStart'); await sleep(1500);
  let fails = 0;
  for (const [name, url] of [['internet', `http://127.0.0.1:8787/v/${room}`], ['wifi', lanLink]]) {
    const v = await ctx.newPage(); await v.setViewportSize({ width: 844, height: 390 });
    await v.addInitScript(() => localStorage.setItem('gs.view.scopes', JSON.stringify({ on: { wave: true, parade: true, vector: true, hist: true }, size: 'S', open: false })));
    await v.goto(url); await sleep(5000);
    await v.bringToFront();
    const btnShown = await v.evaluate(() => getComputedStyle(document.getElementById('btnScopes')).display !== 'none');
    await v.click('#btnScopes');
    await v.waitForFunction(() => { const V = window.__scopeViews; return V && ['wave', 'parade', 'vector', 'hist'].every(k => V[k] && V[k].w > 0); }, null, { timeout: 30000 }).catch(() => {});
    await sleep(1500);
    const r = await v.evaluate(([BARS]) => {
      const V = window.__scopeViews, out = {};
      const img = k => { const x = V[k]; return x.dctx.getImageData(0, 0, x.w, x.h).data; };
      // waveform (Y): bar centre columns, brightest row in the top 60% part = 255 - Y
      const w = img('wave'), W = V.wave.w, H = V.wave.h; out.size = [W, H, V.vector.w];
      // rows are 8-bit levels scaled to the scope height; each column also holds one gray-ramp level,
      // so look for the bright trace nearest the expected bar level
      out.wave = BARS.map(([, c], i) => { const col = Math.round((i + 0.5) / BARS.length * W), Y = .2126*c[0]+.7152*c[1]+.0722*c[2], e = (255 - Y) / 255 * (H - 1); let row = -999;
        for (let y = 0; y < H; y++) if (w[(y * W + col) * 4 + 1] > 150 && Math.abs(y - e) < Math.abs(row - e)) row = y; return 255 - row / (H - 1) * 255; });
      out.rowUnit = 255 / (H - 1);
      // vectorscope: brightest pixel near each 75% target
      const vs = img('vector'), N = V.vector.w, k = 2 * 118 / 256 * N;
      out.vec = BARS.map(([, c]) => { const yv = .2126*c[0]+.7152*c[1]+.0722*c[2], cb = (c[2]-yv)/1.8556/255, cr = (c[0]-yv)/1.5748/255;
        const tx = Math.round(N / 2 + cb * k), ty = Math.round(N / 2 - cr * k); let best = 0, bx = 0, by = 0;
        for (let y = ty - 12; y <= ty + 12; y++) for (let x = tx - 12; x <= tx + 12; x++) { if (x < 0 || y < 0 || x >= N || y >= N) continue; const s = vs[(y*N+x)*4] + vs[(y*N+x)*4+1] + vs[(y*N+x)*4+2]; if (s > best) { best = s; bx = x; by = y; } }
        return Math.hypot(bx - tx, by - ty) * 256 / N; });   // in units of the full 256-px scope
      // gray ramp in parade: the ramp should give a diagonal in all three channels — check G channel at 25/50/75% of the panel
      const p = img('parade'), PW = V.parade.pw, PWW = V.parade.w, PH = V.parade.h;
      out.ramp = [0.25, 0.5, 0.75].map(f => { const col = PW + V.parade.gap + Math.round(f * PW); const hits = [];
        for (let y = 0; y < PH; y++) if (p[(y * PWW + col) * 4 + 1] > 40) hits.push(255 - y / (PH - 1) * 255);
        return { expect: Math.round(f * 255), got: hits }; });
      return out;
    }, [BARS]);
    const waveErr = r.wave.map((y, i) => Math.abs(y - Y709(BARS[i][1])));
    const rampOK = r.ramp.every(q => q.got.some(v => Math.abs(v - q.expect) <= 4 + r.rowUnit));
    const ok = btnShown && Math.max(...waveErr) <= 3 + r.rowUnit && Math.max(...r.vec) <= 4 && rampOK;
    if (!ok) fails++;
    console.log(`${name} (scope data ${r.size}): button ${btnShown ? 'shown' : 'MISSING'}; waveform bar levels error max ${Math.max(...waveErr).toFixed(1)} of 255; vectorscope bars off-target max ${Math.max(...r.vec).toFixed(1)} px (target box ±5); parade ramp ${rampOK ? 'ok' : 'WRONG ' + JSON.stringify(r.ramp)} → ${ok ? 'PASS' : 'FAIL'}`);
    if (process.env.SHOTS) await v.screenshot({ path: `${process.env.SHOTS}/scopes_${name}.png` });
    // host turns scopes off → button disappears
    if (name === 'internet') {
      await host.selectOption('#scopesAllow', 'off'); await sleep(800);
      const gone = await v.evaluate(() => getComputedStyle(document.getElementById('btnScopes')).display === 'none' && !document.getElementById('scopes').classList.contains('show'));
      console.log(`host switch «off»: ${gone ? 'scopes hidden on the viewer — PASS' : 'FAIL'}`); if (!gone) fails++;
      await host.selectOption('#scopesAllow', 'on'); await sleep(500);
    }
    await v.close();
  }
  await b.close(); process.exit(fails ? 1 : 0);
})();
