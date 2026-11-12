// iPhone full screen: Safari on iPhone can't put a page in full screen, so GradeStream uses an in-page
// full-screen mode. This test makes Chromium behave like an iPhone (no Fullscreen API) and checks that the
// tiles hide and come back on a tap, scopes and markers work in that mode, and the page can scroll (a swipe
// up is what makes Safari hide its bars). Run: GS_LOG=gs.log node fullscreentest.js
const { sleep, startHost, scene, viewer } = require('./common');
(async () => {
  const { b, host, url } = await startHost({ draw: scene });
  const vc = await b.newContext({ viewport: { width: 844, height: 390 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, locale: 'en-US' });
  if (process.env.IPHONE !== '0') await vc.addInitScript(() => { for (const k of ['requestFullscreen', 'webkitRequestFullscreen']) { delete Element.prototype[k]; Object.defineProperty(Element.prototype, k, { value: undefined }); } });
  const v = await vc.newPage(); const errs = []; v.on('pageerror', e => errs.push(e.message)); await v.goto(url); await sleep(4000);
  const PAGE = process.env.IPHONE === '0';
  const st = () => v.evaluate(() => ({ imm: document.documentElement.classList.contains('immersive') || !!document.fullscreenElement, barHidden: document.getElementById('bar').classList.contains('hidden'),
    scroll: document.scrollingElement.scrollHeight > innerHeight + 50, scopes: document.getElementById('scopes').classList.contains('show') }));
  const res = [];
  await v.tap('#btnFull'); await sleep(500); let s = await st();
  res.push([PAGE ? 'page full screen on, tiles hidden' : 'full-screen mode on, tiles hidden, page can scroll', s.imm && s.barHidden && (PAGE || s.scroll)]);
  await v.mouse.click(420, 150); await sleep(500); s = await st();
  res.push(['tap brings the tiles back', !s.barHidden]);
  await v.tap('#btnScopes'); await sleep(1500); s = await st();
  res.push(['scopes open in full-screen mode', s.scopes && s.imm]);
  await v.tap('#btnMark'); await sleep(200);
  const c = await v.evaluate(() => { const r = document.getElementById('c').getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; });
  await v.mouse.click(c[0], c[1]); await sleep(300);
  await v.fill('#ctext', 'fullscreen mark'); await v.tap('#csend'); await sleep(800);
  res.push(['the comment box keeps the page in full screen', (await st()).imm]);
  res.push(['marker placed in full-screen mode reaches the colorist', (await host.evaluate(() => document.getElementById('marks').innerText)).includes('fullscreen mark')]);
  await v.tap('#scClose'); await sleep(300);
  await v.tap('#btnFull'); await sleep(400); s = await st();
  res.push(['full-screen mode off again', !s.imm]);
  res.push(['no script errors on the page', errs.length === 0]);
  let fails = 0; for (const [t, ok] of res) { console.log(`${PAGE ? 'Android/PC page full screen' : 'iPhone full screen'}: ${t} → ${ok ? 'PASS' : 'FAIL'}`); if (!ok) fails++; }
  await b.close(); process.exit(fails ? 1 : 0);
})();
