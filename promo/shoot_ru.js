const fs = require('fs'); const { chromium } = require('playwright');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const OUT = '/tmp/claude-0/vid/ru/';
const IMG = 'data:image/jpeg;base64,' + fs.readFileSync('/tmp/claude-0/shots/demo_frame.jpg').toString('base64');
const stub = ([img, w, h]) => { navigator.mediaDevices.getDisplayMedia = async () => {
  const c = document.createElement('canvas'); c.width = w; c.height = h; const g = c.getContext('2d');
  const im = new Image(); im.src = img; await im.decode();
  setInterval(() => { g.drawImage(im, 0, 0, w, h); }, 40); return c.captureStream(25); }; };
const R = '3f9a2c71de';
const FAKE = { link: `https://amber-lake-quiet-pine.trycloudflare.com/v/${R}`, tunnel: 'on', tunnelError: '',
  lanLinks: [`http://192.168.1.20:8790/v/${R}`], lanError: '', lanWarn: '',
  viewers: [{ id: 'a', dev: 'iPhone', kbps: 7900, drops: 0, queue: 0, audio: true, pcm: false, lan: false, rtc: false },
            { id: 'b', dev: 'Android', kbps: 8100, drops: 0, queue: 0, audio: false, pcm: false, lan: true, rtc: true }] };
const IPH = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1';
(async () => {
  const log = fs.readFileSync('/tmp/claude-0/sc/gs.log', 'utf8');
  const key = log.match(/key=([0-9a-f]+)/)[1], room = log.match(/room ([0-9a-f]+)/)[1];
  const b = await chromium.launch({ args: ['--no-proxy-server', '--autoplay-policy=no-user-gesture-required'] });
  const hctx = await b.newContext({ viewport: { width: 1440, height: 1010 }, deviceScaleFactor: 2, locale: 'ru-RU' });
  await hctx.addInitScript(stub, [IMG, 1920, 1080]);
  const host = await hctx.newPage(); host.on('pageerror', e => console.log('HOST EXC', e.message));
  await host.goto(`http://127.0.0.1:8787/host?key=${key}`); await sleep(1000);
  await host.click('text=Оба сразу'); await sleep(1500);
  await host.selectOption('#bitrate', '12000000'); await host.click('#btnStart'); await sleep(2500);
  const phone = await b.newContext({ viewport: { width: 926, height: 428 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true, locale: 'ru-RU', userAgent: IPH });
  const ph = await phone.newPage(); ph.on('pageerror', e => console.log('PHONE EXC', e.message));
  await ph.goto(`http://127.0.0.1:8787/v/${room}`); await sleep(3500);
  await ph.evaluate(() => { document.getElementById('btnSound').style.display = ''; });
  await ph.screenshot({ path: OUT + 'phone.png' });
  const mark = async (x, y, txt, shot) => {
    await ph.tap('#btnMark'); await sleep(300); await ph.touchscreen.tap(x, y); await sleep(300);
    await ph.type('#ctext', txt); await sleep(200);
    if (shot) await ph.screenshot({ path: OUT + shot });
    await ph.tap('#csend'); await sleep(1500);
  };
  await mark(250, 300, 'Тени зеленят — сделайте теплее', null); await sleep(3500);
  await mark(560, 205, 'Солнце выбивается, опустите света', 'phone-mark.png');
  await host.evaluate(F => { const o = renderState; renderState = s => o({ ...s, ...F }); renderState(lastState || {});
    codecHW = true; document.getElementById('encInfo').textContent = 'Кодек H.264 avc1.42e028 · аппаратный'; }, FAKE);
  await sleep(1200);
  await host.screenshot({ path: OUT + 'host.png' });
  await host.locator('.stage').screenshot({ path: OUT + 'preview.png' });
  await host.locator('aside .panel').nth(0).screenshot({ path: OUT + 'panel-modes.png' });
  await ph.tap('#btnScopes'); await sleep(1500); await ph.evaluate(()=>{const t=[...document.querySelectorAll('*')].find(e=>e.children.length==0&&e.textContent.trim()=='Parade'); t&&t.click();}); await sleep(2000);
  await ph.screenshot({ path: OUT + 'phone-scopes.png' });
  await b.close(); process.exit(0);
})();
