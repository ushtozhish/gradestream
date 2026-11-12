// Client layout and scope switching: for a landscape (1920×1080) or portrait (1080×1920, monitor rotated
// in Windows) screen, phone held both ways, both layouts, every combination of scopes switched by tapping
// the tabs. Checks: picture and scopes never overlap, stay above the bottom tiles and inside the screen,
// fill the area without big gaps, every visible scope draws a signal; a marker on the picture maps correctly.
// Run: SRC=land|port GS_LOG=gs.log node layouttest.js   (SHOTS=dir saves screenshots)
const { sleep, startHost, scene, viewer } = require('./common');
const SRC = process.env.SRC || 'land', [SW, SH] = SRC === 'port' ? [1080, 1920] : [1920, 1080];
(async () => {
  const { b, host, url } = await startHost({ w: SW, h: SH, draw: scene });
  let fails = 0, checks = 0;
  const fail = m => { fails++; console.log('  FAIL ' + m); };
  for (const [orient, vp] of [['phone sideways', { width: 844, height: 390 }], ['phone upright', { width: 390, height: 844 }]]) {
    const { vc, v } = await viewer(b, url, { ...vp, scopes: { on: { wave: true, parade: true, vector: true }, open: false, big: false, color: true } });
    const size = await v.evaluate(() => [document.getElementById('c').width, document.getElementById('c').height]);
    if (size[0] !== SW || size[1] !== SH) fail(`${orient}: picture is ${size} instead of ${SW}×${SH}`);
    await v.tap('#btnScopes'); await sleep(800);
    for (const big of [false, true]) {
      const isBig = await v.evaluate(() => document.getElementById('scBig').classList.contains('on'));
      if (isBig !== big) { await v.tap('#scBig'); await sleep(500); }
      // all 7 non-empty combinations, reached by tapping the tabs
      for (let mask = 7; mask >= 1; mask--) {
        const want = { wave: !!(mask & 1), parade: !!(mask & 2), vector: !!(mask & 4) };
        for (const k of ['wave', 'parade', 'vector']) {
          const on = await v.evaluate(k => document.querySelector(`[data-sc=${k}]`).classList.contains('on'), k);
          if (on !== want[k] && (want[k] || Object.values(want).some(Boolean))) {
            // turning a scope off when it is the last one is refused by design, so switch the others on first
            await v.tap(`[data-sc=${k}]`); await sleep(150);
          }
        }
        for (const k of ['wave', 'parade', 'vector']) {   // second pass for the order-dependent case
          const on = await v.evaluate(k => document.querySelector(`[data-sc=${k}]`).classList.contains('on'), k);
          if (on !== want[k]) { await v.tap(`[data-sc=${k}]`); await sleep(150); }
        }
        await sleep(900);
        const r = await v.evaluate(() => {
          const R = el => { const b = el.getBoundingClientRect(); return { x: b.left, y: b.top, w: b.width, h: b.height }; };
          const scopes = [...document.querySelectorAll('#scBody canvas')].map(c => ({ ...R(c), drawn: (() => { const g = c.getContext('2d'); const d = g.getImageData(0, 0, c.width, c.height).data; let lit = 0; for (let i = 0; i < d.length; i += 64) if (d[i] + d[i + 1] + d[i + 2] > 120) lit++; return lit; })() }));
          return { pic: R(document.getElementById('c')), scopes, body: R(document.getElementById('scBody')), bar: R(document.getElementById('bar')),
                   head: R(document.getElementById('scHead')), vw: innerWidth, vh: innerHeight, n: document.querySelectorAll('#scBody canvas').length };
        });
        checks++;
        const tag = `${orient} · ${big ? 'big scopes' : 'big picture'} · ${Object.keys(want).filter(k => want[k]).join('+')}`;
        const nOn = Object.values(want).filter(Boolean).length;
        if (r.n !== nOn) fail(`${tag}: ${r.n} scopes shown, expected ${nOn}`);
        const all = [{ ...r.pic, name: 'picture' }, ...r.scopes.map((s, i) => ({ ...s, name: 'scope ' + i }))];
        const ov = (a, c) => Math.max(0, Math.min(a.x + a.w, c.x + c.w) - Math.max(a.x, c.x)) * Math.max(0, Math.min(a.y + a.h, c.y + c.h) - Math.max(a.y, c.y));
        for (let i = 0; i < all.length; i++) for (let j = i + 1; j < all.length; j++) if (ov(all[i], all[j]) > 4) fail(`${tag}: ${all[i].name} overlaps ${all[j].name}`);
        for (const a of all) {
          if (a.w < 40 || a.h < 30) fail(`${tag}: ${a.name} too small ${Math.round(a.w)}×${Math.round(a.h)}`);
          if (a.x < -1 || a.x + a.w > r.vw + 1 || a.y < r.head.y + r.head.h - 1) fail(`${tag}: ${a.name} outside the screen`);
          if (a.y + a.h > r.bar.y + 1) fail(`${tag}: ${a.name} goes under the bottom tiles`);
        }
        const used = all.reduce((s, a) => s + a.w * a.h, 0), avail = r.vw * (r.bar.y - r.head.y - r.head.h);
        if (used / avail < 0.72) fail(`${tag}: only ${Math.round(used / avail * 100)}% of the screen used`);
        for (const [i, s] of r.scopes.entries()) if (s.drawn < 20) fail(`${tag}: scope ${i} draws no signal`);
        if (process.env.SHOTS && (mask === 7 || mask === 1)) await v.screenshot({ path: `${process.env.SHOTS}/layout_${SRC}_${orient.replace(' ', '-')}_${big ? 'scopes' : 'picture'}_${mask === 7 ? 'all' : 'one'}.png` });
      }
    }
    // the last scope cannot be switched off
    for (const k of ['parade', 'vector']) if (await v.evaluate(k => document.querySelector(`[data-sc=${k}]`).classList.contains('on'), k)) { await v.tap(`[data-sc=${k}]`); await sleep(150); }
    await v.tap('[data-sc=wave]'); await sleep(300);
    if (!await v.evaluate(() => document.querySelectorAll('#scBody canvas').length === 1)) fail(`${orient}: the last scope could be switched off`);
    // marker in the middle of the picture → the host gets the centre of the frame
    await v.tap('#btnMark'); await sleep(200);
    const c = await v.evaluate(() => { const b = document.getElementById('c').getBoundingClientRect(); return [b.left + b.width / 2, b.top + b.height / 2]; });
    await v.mouse.click(c[0], c[1]); await sleep(300);
    await v.fill('#ctext', SRC + ' ' + orient); await v.press('#ctext', 'Enter'); await sleep(800);
    const txt = await host.evaluate(() => document.getElementById('marks').innerText);
    const m = txt.match(/(\d+), (\d+) — [^\n]*$/m) || txt.match(/(\d+), (\d+)/);
    if (!m || Math.abs(+m[1] - SW / 2) > 6 || Math.abs(+m[2] - SH / 2) > 6) fail(`${orient}: marker in the picture centre arrived as ${m ? m[1] + ', ' + m[2] : 'nothing'}, expected ${SW / 2}, ${SH / 2}`);
    await vc.close();
  }
  console.log(`layout, ${SRC === 'port' ? 'portrait 1080×1920' : 'landscape 1920×1080'} screen: ${checks} layouts checked, ${fails ? fails + ' problems → FAIL' : 'no overlaps, nothing under the tiles, screen filled, every scope drawing, marker on target → PASS'}`);
  await b.close(); process.exit(fails ? 1 : 0);
})();
