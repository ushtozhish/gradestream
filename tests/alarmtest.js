// Out-of-range indication: a frame with blown highlights, crushed blacks and over-saturated color must light
// the red alarm on Waveform, Parade and Vectorscope; a normal frame with letterbox bars must not.
// Run: GS_LOG=gs.log node alarmtest.js
const { sleep, startHost, scene, viewer } = require('./common');
const CASE = process.env.CASE || 'bad';
(async () => {
  const draw = CASE === 'bad'
    ? function (g, w, h) { g.fillStyle = '#6a6a6a'; g.fillRect(0, 0, w, h); g.fillStyle = '#fff'; g.fillRect(0, 0, w * 0.3, h * 0.3);
        g.fillStyle = '#000'; g.fillRect(w * 0.4, h * 0.4, w * 0.2, h * 0.2); g.fillStyle = '#ff0000'; g.fillRect(w * 0.7, h * 0.7, w * 0.25, h * 0.25); }
    : function (g, w, h) { const bar = Math.round(h * 0.12); g.save(); g.translate(0, bar); g.scale(1, (h - 2 * bar) / h);
        let gr = g.createLinearGradient(0, 0, 0, h); gr.addColorStop(0, '#3a5d8a'); gr.addColorStop(1, '#8aa0b0'); g.fillStyle = gr; g.fillRect(0, 0, w, h);
        g.fillStyle = '#b06050'; g.fillRect(w * 0.3, h * 0.4, w * 0.2, h * 0.3); g.restore();
        g.fillStyle = '#000'; g.fillRect(0, 0, w, bar); g.fillRect(0, h - bar, w, bar); };   // letterbox bars, nothing clipped inside the picture
  const { b, url } = await startHost({ draw });
  const { v } = await viewer(b, url, { width: 844, height: 390, scopes: { on: { wave: true, parade: true, vector: true }, open: false, big: true, color: true } });
  await v.tap('#btnScopes'); await sleep(2500);
  const r = await v.evaluate(() => ({ flags: window.__scopeFlags, alarm: [...document.querySelectorAll('#scBody canvas')].map(c => c.classList.contains('alarm')) }));
  if (process.env.SHOTS) await v.screenshot({ path: `${process.env.SHOTS}/alarm_${CASE}.png` });
  const lit = r.alarm.filter(Boolean).length;
  const ok = CASE === 'bad' ? lit === 3 && r.flags.wave.length === 2 && r.flags.parade.length === 2 && r.flags.vector.length === 1 : lit === 0;
  console.log(`out-of-range alarm, ${CASE === 'bad' ? 'clipped and over-saturated frame' : 'normal frame with letterbox bars'}: ${lit} of 3 scopes lit (${JSON.stringify(r.flags)}) → ${ok ? 'PASS' : 'FAIL'}`);
  await b.close(); process.exit(ok ? 0 : 1);
})();
