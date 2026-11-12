// Measures color (ΔE2000) and latency through the real GradeStream pipeline.
// Run (Linux/macOS, needs Node.js and Playwright with Chromium):
//   cd app && go build -o gs . && GS_NO_TUNNEL=1 GS_LAN_ANY=1 ./gs > ../tests/gs.log 2>&1 &
//   cd tests && npm i playwright && GS_LOG=gs.log BR=8000000 node colortest.js
// GS_LAN_ANY allows test addresses outside private ranges, GS_NO_TUNNEL skips Cloudflare.
const fs = require('fs'); const { chromium } = require('playwright');
const sleep = ms => new Promise(r => setTimeout(r, ms));
// ---- test chart ----
const P = [];
for (const g of [0,16,32,48,64,80,96,112,128,144,160,176,192,208,224,240,255]) P.push({ n: 'gray ' + g, c: [g,g,g], gray: true });
for (const [n, c] of [['R 100%',[255,0,0]],['G 100%',[0,255,0]],['B 100%',[0,0,255]],['C 100%',[0,255,255]],['M 100%',[255,0,255]],['Y 100%',[255,255,0]],
  ['R 75%',[191,0,0]],['G 75%',[0,191,0]],['B 75%',[0,0,191]],['C 75%',[0,191,191]],['M 75%',[191,0,191]],['Y 75%',[191,191,0]],
  ['dark skin',[115,82,68]],['light skin',[194,150,130]],['blue sky',[98,122,157]],['foliage',[87,108,67]],['blue flower',[133,128,177]],['bluish green',[103,189,170]],
  ['orange',[214,126,44]],['purplish blue',[80,91,166]],['moderate red',[193,90,99]],['purple',[94,60,108]],['yellow green',[157,188,64]],['orange yellow',[224,163,46]]]) P.push({ n, c });
const COLS = 8, PW = 240, PH = 150, Y0 = 90;
P.forEach((p, i) => { p.x = (i % COLS) * PW; p.y = Y0 + Math.floor(i / COLS) * PH; });
// ---- ΔE2000 ----
function lab([r,g,b]) { const f = v => { v /= 255; return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; };
  const R = f(r), G = f(g), B = f(b);
  let X = (0.4124*R + 0.3576*G + 0.1805*B) / 0.95047, Y = 0.2126*R + 0.7152*G + 0.0722*B, Z = (0.0193*R + 0.1192*G + 0.9505*B) / 1.08883;
  const h = t => t > 216/24389 ? Math.cbrt(t) : (24389/27 * t + 16) / 116; X = h(X); Y = h(Y); Z = h(Z);
  return [116*Y - 16, 500*(X - Y), 200*(Y - Z)]; }
function de2000(l1, l2) { const [L1,a1,b1] = l1, [L2,a2,b2] = l2, rad = Math.PI/180;
  const C1 = Math.hypot(a1,b1), C2 = Math.hypot(a2,b2), Cm = (C1+C2)/2, G = 0.5*(1-Math.sqrt(Cm**7/(Cm**7+25**7)));
  const a1p = a1*(1+G), a2p = a2*(1+G), C1p = Math.hypot(a1p,b1), C2p = Math.hypot(a2p,b2);
  const h1p = (Math.atan2(b1,a1p)/rad+360)%360, h2p = (Math.atan2(b2,a2p)/rad+360)%360;
  const dLp = L2-L1, dCp = C2p-C1p; let dhp = 0; if (C1p*C2p) { dhp = h2p-h1p; if (dhp > 180) dhp -= 360; else if (dhp < -180) dhp += 360; }
  const dHp = 2*Math.sqrt(C1p*C2p)*Math.sin(dhp/2*rad), Lpm = (L1+L2)/2, Cpm = (C1p+C2p)/2;
  let hpm = h1p+h2p; if (C1p*C2p) { if (Math.abs(h1p-h2p) > 180) hpm += hpm < 360 ? 360 : -360; hpm /= 2; }
  const T = 1-0.17*Math.cos((hpm-30)*rad)+0.24*Math.cos(2*hpm*rad)+0.32*Math.cos((3*hpm+6)*rad)-0.2*Math.cos((4*hpm-63)*rad);
  const dTh = 30*Math.exp(-(((hpm-275)/25)**2)), RC = 2*Math.sqrt(Cpm**7/(Cpm**7+25**7));
  const SL = 1+0.015*(Lpm-50)**2/Math.sqrt(20+(Lpm-50)**2), SC = 1+0.045*Cpm, SH = 1+0.015*Cpm*T, RT = -Math.sin(2*dTh*rad)*RC;
  return Math.sqrt((dLp/SL)**2+(dCp/SC)**2+(dHp/SH)**2+RT*(dCp/SC)*(dHp/SH)); }

async function readPatches(page, srcExpr) {
  return page.evaluate(([P, srcExpr]) => {
    const src = eval(srcExpr); const w = src.videoWidth || src.width, h = src.videoHeight || src.height;
    const c = document.createElement('canvas'); c.width = 1920; c.height = 1080; const g = c.getContext('2d'); g.drawImage(src, 0, 0, 1920, 1080);
    return P.map(p => { const d = g.getImageData(p.x + 90, p.y + 55, 60, 40).data; const s = [0,0,0];
      for (let i = 0; i < d.length; i += 4) { s[0] += d[i]; s[1] += d[i+1]; s[2] += d[i+2]; } const n = d.length / 4; return s.map(v => v / n); });
  }, [P, srcExpr]);
}
async function readCounter(page, srcExpr) {
  return page.evaluate(srcExpr => { const src = eval(srcExpr); const c = document.createElement('canvas'); c.width = 1920; c.height = 1080;
    const g = c.getContext('2d'); g.drawImage(src, 0, 0, 1920, 1080); let n = 0;
    for (let b = 0; b < 16; b++) { const d = g.getImageData(b * 60 + 20, 25, 10, 10).data; if (d[0] > 128) n |= 1 << b; } return { n, t: Date.now() }; }, srcExpr);
}

(async () => {
  const log = fs.readFileSync(process.env.GS_LOG || 'gs.log', 'utf8');
  const key = log.match(/key=([0-9a-f]+)/)[1], room = log.match(/room ([0-9a-f]+)/)[1];
  const b = await chromium.launch({ args: ['--no-proxy-server', '--autoplay-policy=no-user-gesture-required'] });
  const ctx = await b.newContext({ viewport: { width: 1360, height: 900 } });
  await ctx.addInitScript(P => {
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas'); c.width = 1920; c.height = 1080; const g = c.getContext('2d');
      g.fillStyle = '#404040'; g.fillRect(0, 0, 1920, 1080);
      for (const p of P) { g.fillStyle = `rgb(${p.c})`; g.fillRect(p.x, p.y, 240, 150); }
      window.__draw = {}; let n = 0;
      setInterval(() => { n = (n + 1) & 0xffff; for (let b = 0; b < 16; b++) { g.fillStyle = (n >> b) & 1 ? '#fff' : '#000'; g.fillRect(b * 60, 0, 60, 60); } window.__draw[n] = Date.now(); }, 20);
      return c.captureStream(50);
    };
  }, P);
  const host = await ctx.newPage();
  await host.goto(`http://127.0.0.1:8787/host?key=${key}`);
  await sleep(700);
  await host.click('label:has(input[name=mode][value=both])'); await sleep(1500);
  await host.selectOption('#bitrate', process.env.BR || '8000000');
  await host.selectOption('#fps', '50');
  const lanLink = await host.evaluate(() => document.getElementById('lanLink').value);
  await host.click('#btnStart'); await sleep(1500);
  const net = await ctx.newPage(); await net.goto(`http://127.0.0.1:8787/v/${room}`);
  const lan = await ctx.newPage(); await lan.goto(lanLink);
  await sleep(6000);
  const enc = await host.evaluate(() => document.getElementById('encInfo').textContent);
  const out = { encoder: enc, bitrate: process.env.BR || '8000000', paths: {} };
  for (const [name, page, expr] of [['internet (WebCodecs)', net, "document.getElementById('c')"], ['wifi (WebRTC)', lan, "document.getElementById('lv')"]]) {
    await page.bringToFront(); await sleep(1500);
    // wait until the picture is really playing (a slow CI machine can take longer to start WebRTC)
    await page.waitForFunction(expr => { const s = eval(expr); if (s.tagName === 'VIDEO' && (s.readyState < 2 || !s.videoWidth)) return false;
      const c = document.createElement('canvas'); c.width = 64; c.height = 36; const g = c.getContext('2d'); g.drawImage(s, 0, 0, 64, 36);
      const d = g.getImageData(0, 0, 64, 36).data; let m = 0; for (let i = 0; i < d.length; i += 4) m = Math.max(m, d[i], d[i + 1], d[i + 2]); return m > 100; }, expr, { timeout: 30000, polling: 500 }).catch(() => console.log(name + ': picture did not start within 30 s'));
    await sleep(1000);
    const got = await readPatches(page, expr);
    const rows = P.map((p, i) => { const s = got[i]; return { n: p.n, src: p.c, got: s.map(v => +v.toFixed(1)), dE: +de2000(lab(p.c), lab(s)).toFixed(2), gray: !!p.gray, chroma: p.gray ? +Math.hypot(lab(s)[1], lab(s)[2]).toFixed(2) : null }; });
    // latency: 15 samples
    const lat = [];
    for (let k = 0; k < 15; k++) { const r = await readCounter(page, expr); const dt = await host.evaluate(n => window.__draw[n], r.n); if (dt) lat.push(r.t - dt); await sleep(150); }
    lat.sort((a, b) => a - b);
    out.paths[name] = { rows, latency: { median: lat[lat.length >> 1], min: lat[0], max: lat[lat.length - 1] } };
  }
  fs.writeFileSync(`color_${process.env.BR || '8000000'}.json`, JSON.stringify(out, null, 1));
  for (const [k, v] of Object.entries(out.paths)) {
    const d = v.rows.map(r => r.dE), gr = v.rows.filter(r => r.gray);
    console.log(`${k}: ΔE2000 mean ${(d.reduce((a, b) => a + b) / d.length).toFixed(2)}, max ${Math.max(...d).toFixed(2)} (${v.rows.find(r => r.dE === Math.max(...d)).n}); gray: max ΔE ${Math.max(...gr.map(r => r.dE)).toFixed(2)}, max chroma ${Math.max(...gr.map(r => r.chroma)).toFixed(2)}; black→${gr[0].got} white→${gr[gr.length-1].got}; latency median ${v.latency.median} ms (${v.latency.min}–${v.latency.max})`);
  }
  console.log('encoder:', enc);
  await b.close(); process.exit(0);
})();
