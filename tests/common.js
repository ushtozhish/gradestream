// Shared helpers for the GradeStream browser tests: a fake screen and a started host page.
const fs = require('fs'); const { chromium } = require('playwright');
const sleep = ms => new Promise(r => setTimeout(r, ms));
// A fake "monitor": w×h canvas drawn by `draw` (a function source string run in the page), streamed as the captured screen
async function startHost({ w = 1920, h = 1080, draw, mode = 'internet' } = {}) {
  const log = fs.readFileSync(process.env.GS_LOG || 'gs.log', 'utf8');
  const key = log.match(/key=([0-9a-f]+)/)[1], room = log.match(/room ([0-9a-f]+)/)[1];
  const b = await chromium.launch({ args: ['--no-proxy-server', '--autoplay-policy=no-user-gesture-required'] });
  const ctx = await b.newContext({ viewport: { width: 1360, height: 900 } });
  await ctx.addInitScript(([w, h, draw]) => {
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas'); c.width = w; c.height = h; const g = c.getContext('2d');
      (0, eval)('(' + draw + ')')(g, w, h);
      setInterval(() => { g.fillStyle = '#000'; g.fillRect(0, h - 1, 1, 1); }, 40);   // keep frames flowing
      return c.captureStream(25);
    };
  }, [w, h, draw.toString()]);
  const host = await ctx.newPage();
  await host.goto(`http://127.0.0.1:8787/host?key=${key}`); await sleep(700);
  if (mode === 'both') { await host.click('label:has(input[name=mode][value=both])'); await sleep(1500); }
  await host.click('#btnStart'); await sleep(1500);
  const lanLink = mode === 'both' ? await host.evaluate(() => document.getElementById('lanLink').value) : null;
  return { b, host, room, lanLink, url: `http://127.0.0.1:8787/v/${room}` };
}
// A natural-looking frame: sky and grass gradients, a face, colored objects and grain
function scene(g, w, h) {
  let gr = g.createLinearGradient(0, 0, 0, h * 0.6); gr.addColorStop(0, '#2a5d9a'); gr.addColorStop(1, '#a9c4dc'); g.fillStyle = gr; g.fillRect(0, 0, w, h * 0.6);
  gr = g.createLinearGradient(0, h * 0.6, 0, h); gr.addColorStop(0, '#5c6b33'); gr.addColorStop(1, '#1c2210'); g.fillStyle = gr; g.fillRect(0, h * 0.6, w, h * 0.4);
  const u = Math.min(w, h);
  const rg = g.createRadialGradient(w / 2, h * 0.45, 5, w / 2, h * 0.45, u * 0.18); rg.addColorStop(0, '#f2c4a4'); rg.addColorStop(1, '#6b3f2c');
  g.fillStyle = rg; g.beginPath(); g.ellipse(w / 2, h * 0.45, u * 0.12, u * 0.16, 0, 0, 7); g.fill();
  g.fillStyle = '#b52a5a'; g.fillRect(w * 0.1, h * 0.7, u * 0.12, u * 0.15); g.fillStyle = '#2f4fb0'; g.fillRect(w * 0.25, h * 0.75, u * 0.1, u * 0.12);
  g.fillStyle = '#e8d24a'; g.fillRect(w * 0.6, h * 0.72, u * 0.1, u * 0.14); g.fillStyle = '#d9692b'; g.fillRect(w * 0.75, h * 0.66, u * 0.14, u * 0.17);
  const im = g.getImageData(0, 0, w, h), d = im.data;
  for (let i = 0; i < d.length; i += 4) { const n = (Math.random() + Math.random() + Math.random() - 1.5) * 9; d[i] += n; d[i + 1] += n; d[i + 2] += n; }
  g.putImageData(im, 0, 0);
}
async function viewer(b, url, { width, height, scopes, dpr = 2, locale = 'en-US' }) {
  const vc = await b.newContext({ viewport: { width, height }, deviceScaleFactor: dpr, isMobile: true, hasTouch: true, locale });
  if (scopes) await vc.addInitScript(st => localStorage.setItem('gs.view.scopes', JSON.stringify(st)), scopes);
  const v = await vc.newPage(); await v.goto(url);
  await v.waitForFunction(() => document.getElementById('status').style.display === 'none', null, { timeout: 30000 }).catch(() => {});
  return { vc, v };
}
module.exports = { sleep, startHost, scene, viewer };
