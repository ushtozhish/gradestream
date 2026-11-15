// Voice call: the host and two viewers in one call, each with its own fake microphone (a tone of its own pitch).
// Checks: everyone hears the others and not themselves; a viewer on the Wi-Fi link (http) can listen but has no mic;
// mute one / mute all from the host; a viewer turns the mic back on; turning the call off; relay delay; silence is not sent.
const fs = require('fs'); const path = require('path'); const { chromium } = require('playwright');
const { sleep, scene } = require('./common');

// 16-bit mono WAV: tone bursts (0.6 s on / 1.4 s off) so noise suppression keeps them
function toneWav(file, hz) {
  const sr = 48000, n = sr * 10, d = Buffer.alloc(44 + n * 2);
  d.write('RIFF', 0); d.writeUInt32LE(36 + n * 2, 4); d.write('WAVEfmt ', 8); d.writeUInt32LE(16, 16); d.writeUInt16LE(1, 20); d.writeUInt16LE(1, 22);
  d.writeUInt32LE(sr, 24); d.writeUInt32LE(sr * 2, 28); d.writeUInt16LE(2, 32); d.writeUInt16LE(16, 34); d.write('data', 36); d.writeUInt32LE(n * 2, 40);
  for (let i = 0; i < n; i++) { const t = i / sr, on = (t % 2) < 0.6 ? 1 : 0; d.writeInt16LE(Math.round(on * 0.4 * 32767 * Math.sin(2 * Math.PI * hz * t)), 44 + i * 2); }
  fs.writeFileSync(file, d);
}
const browser = wav => chromium.launch({ args: ['--no-proxy-server', '--proxy-server=direct://', '--proxy-bypass-list=*', '--autoplay-policy=no-user-gesture-required',
  '--use-fake-ui-for-media-stream', '--use-fake-device-for-media-stream', `--use-file-for-fake-audio-capture=${wav}`] });
// Every page records what it hears: per speaker id — blocks and the pitch of loud blocks (zero crossings), plus arrival times
const probe = () => {
  window.__rx = {}; window.__tx = 0;
  window.__lv = []; window.__callTx = p => { window.__tx++; window.__lv.push(p[3]); if (window.__lv.length > 600) window.__lv.shift(); };
  window.__callRx = (id, d, seq) => {
    const r = window.__rx[id] || (window.__rx[id] = { n: 0, hz: [], seqs: [] });
    r.n++; r.seqs.push([seq, performance.timeOrigin + performance.now()]); if (r.seqs.length > 400) r.seqs.shift();
    let z = 0, e = 0; for (let i = 1; i < d.length; i++) { if ((d[i - 1] < 0) !== (d[i] < 0)) z++; e += d[i] * d[i]; }
    if (e / d.length > 0.002) r.hz.push(z / 2 / 0.02);
  };
  // send times by sequence number, to measure the relay delay on the same machine
  const orig = WebSocket.prototype.send;
  window.__sent = new Map();
  WebSocket.prototype.send = function (b) {
    if (b instanceof Uint8Array && b[0] === 5) window.__sent.set((b[1] << 8) | b[2], performance.timeOrigin + performance.now());
    return orig.call(this, b);
  };
};
const median = a => { const s = a.slice().sort((x, y) => x - y); return s.length ? s[Math.floor(s.length / 2)] : NaN; };
const pitch = (page, id) => page.evaluate(id => { const r = window.__rx[id]; return r ? { n: r.n, hz: r.hz.length ? r.hz.sort((a, b) => a - b)[Math.floor(r.hz.length / 2)] : 0 } : { n: 0, hz: 0 }; }, id);

(async () => {
  const tmp = fs.mkdtempSync(path.join(require('os').tmpdir(), 'gscall-'));
  const wHost = path.join(tmp, 'host.wav'), wView = path.join(tmp, 'view.wav');
  toneWav(wHost, 440); toneWav(wView, 1000);
  const log = fs.readFileSync(process.env.GS_LOG || 'gs.log', 'utf8');
  const key = log.match(/key=([0-9a-f]+)/)[1], room = log.match(/room ([0-9a-f]+)/)[1];
  let fail = 0; const check = (ok, text) => { console.log(`${ok ? 'ok  ' : 'FAIL'} ${text}`); if (!ok) fail++; };

  // ---- host: fake screen + 440 Hz microphone, both links (internet + Wi-Fi) ----
  const bh = await browser(wHost);
  const hc = await bh.newContext({ viewport: { width: 1360, height: 1000 }, locale: 'en-US' });
  await hc.addInitScript(draw => {
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas'); c.width = 1920; c.height = 1080; const g = c.getContext('2d');
      (0, eval)('(' + draw + ')')(g, 1920, 1080);
      setInterval(() => { g.fillStyle = '#000'; g.fillRect(0, 1079, 1, 1); }, 40);
      return c.captureStream(25);
    };
  }, scene.toString());
  await hc.addInitScript(probe);
  const host = await hc.newPage(); host.on('pageerror', e => console.log('HOST EXC', e.message));
  await host.goto(`http://127.0.0.1:8787/host?key=${key}`); await sleep(800);
  await host.click('label:has(input[name=mode][value=both])'); await sleep(1500);
  await host.click('#btnStart'); await sleep(1200);
  check(await host.isChecked('#callOn'), 'call is on by default on the host');
  await host.click('#btnMic'); await sleep(600);
  check(await host.evaluate(() => voice.micOn), 'host microphone on');
  const lanLink = await host.evaluate(() => document.getElementById('lanLink').value);

  // ---- viewer A: secure page (localhost behaves like https), 1000 Hz microphone ----
  const bv = await browser(wView);
  const ac = await bv.newContext({ viewport: { width: 844, height: 390 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, locale: 'en-US' });
  await ac.addInitScript(probe);
  const a = await ac.newPage(); a.on('pageerror', e => console.log('A EXC', e.message));
  await a.goto(`http://127.0.0.1:8787/v/${room}`); await sleep(2500);
  check(await a.isVisible('#btnCall'), 'viewer sees the «Call» button');
  await a.click('#btnCall'); await sleep(300);
  await a.fill('#cname', 'Director'); await a.click('#cbJoin'); await sleep(800);
  check(await a.evaluate(() => voice.micOn), 'viewer A joined with the microphone on');

  // ---- viewer B: Wi-Fi link (http, not secure) — listens only ----
  const bc = await bv.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, locale: 'en-US' });
  await bc.addInitScript(probe);
  const bb = await bc.newPage(); bb.on('pageerror', e => console.log('B EXC', e.message));
  await bb.goto(lanLink); await sleep(3000);
  await bb.click('#btnCall'); await sleep(300);
  const warnB = await bb.evaluate(() => document.querySelector('#callBox .warnx')?.textContent || '');
  check(/microphone/i.test(warnB), 'Wi-Fi viewer is told the microphone needs the secure link');
  await bb.fill('#cname', 'Producer'); await bb.click('#cbJoin'); await sleep(500);
  check(!(await bb.evaluate(() => voice.micOn)), 'Wi-Fi viewer joined without a microphone (listen only)');

  await sleep(5000);
  const aId = await a.evaluate(() => myId);
  const hostFromA = await pitch(host, aId), aFromHost = await pitch(a, 'host'), aFromSelf = await pitch(a, aId);
  const bFromHost = await pitch(bb, 'host'), bFromA = await pitch(bb, aId);
  console.log(`  host hears A: ${hostFromA.n} blocks, ${Math.round(hostFromA.hz)} Hz · A hears host: ${aFromHost.n}, ${Math.round(aFromHost.hz)} Hz · B hears host: ${bFromHost.n}, ${Math.round(bFromHost.hz)} Hz, A: ${bFromA.n}, ${Math.round(bFromA.hz)} Hz`);
  check(hostFromA.n > 50 && Math.abs(hostFromA.hz - 1000) < 120, 'host hears viewer A (1000 Hz)');
  check(aFromHost.n > 50 && Math.abs(aFromHost.hz - 440) < 60, 'viewer A hears the host (440 Hz)');
  check(aFromSelf.n === 0, 'viewer A does not hear itself');
  check(bFromHost.n > 50 && bFromA.n > 50, 'Wi-Fi viewer hears both the host and viewer A');
  // silence is not sent: tone 0.6 s of every 2 s, plus a 0.4 s tail → about 25 blocks/s of 50
  const rate = await a.evaluate(() => { const s = window.__rx.host.seqs; return (s.length - 1) / ((s[s.length - 1][1] - s[0][1]) / 1000); });
  console.log(`  host → viewer A: ${rate.toFixed(1)} blocks/s (50 = no gating)`);
  if (process.env.DBG) console.log(await host.evaluate(() => window.__lv.join(',')));
  check(rate < 35, 'pauses are not transmitted');
  // relay delay host → viewer (same machine clock): send time by sequence number vs arrival
  const sent = await host.evaluate(() => [...window.__sent.entries()]);
  const sm = new Map(sent), arr = await a.evaluate(() => window.__rx.host.seqs);
  const del = arr.map(([s, t]) => sm.has(s) ? t - sm.get(s) : null).filter(x => x !== null && x >= 0 && x < 2000);
  console.log(`  relay delay host → viewer: median ${median(del).toFixed(1)} ms (+ ~80 ms playout buffer)`);
  check(del.length > 20 && median(del) < 60, 'relay delay is small');
  // names and speaking indicator
  const hostList = await host.evaluate(() => document.getElementById('callPeople').innerText);
  check(/Director/.test(hostList) && /Producer/.test(hostList) && /listening only/.test(hostList), 'host sees both names and the listen-only note');
  const talkB = await bb.evaluate(() => document.getElementById('talk').textContent);
  console.log(`  speaking indicator on B: «${talkB}»`);
  await bb.screenshot({ path: 'shots/call-client-wifi.png' }).catch(() => {});
  await a.click('#btnCall'); await sleep(300); await a.screenshot({ path: 'shots/call-client.png' }).catch(() => {}); await a.click('#cbClose');
  await host.locator('#callPanel').screenshot({ path: 'shots/call-host.png' }).catch(() => {});

  // ---- host mutes viewer A ----
  await host.click(`#callPeople [data-mute="${aId}"]`); await sleep(800);
  check(!(await a.evaluate(() => voice.micOn)), 'host muted viewer A → A microphone off');
  const n1 = (await pitch(host, aId)).n; await sleep(1500); const n2 = (await pitch(host, aId)).n;
  check(n2 - n1 <= 2, 'after mute the host no longer receives A');
  // ---- A turns the mic back on ----
  await a.click('#btnMic'); await sleep(2500);
  check(await a.evaluate(() => voice.micOn) && (await pitch(host, aId)).n > n2 + 20, 'viewer A turned the microphone back on and is heard again');
  // ---- mute all ----
  await host.click('#btnMuteAll'); await sleep(800);
  check(!(await a.evaluate(() => voice.micOn)), '«Turn off everyone’s microphones» works');
  // ---- language of the call texts ----
  await host.click('#langSw [data-lang="ru"]'); await sleep(900);
  check(/Голосовой звонок/i.test(await host.evaluate(() => document.getElementById('callPanel').innerText)), 'call panel switches to Russian');
  await host.click('#langSw [data-lang="en"]'); await sleep(900);
  check(!/[А-Яа-яЁё]/.test(await host.evaluate(() => document.getElementById('callPanel').innerText)), 'call panel back in English, no Cyrillic left');
  // ---- host turns the call off ----
  await host.click('#callOn'); await sleep(900);
  check(!(await a.isVisible('#btnCall')) && !(await a.evaluate(() => callUI.joined)), 'call off → viewers leave and the button disappears');

  console.log(fail ? `CALL: FAIL (${fail})` : 'CALL: PASS');
  await bh.close(); await bv.close(); process.exit(0);
})().catch(e => { console.log('CALL: ERROR', e.message); process.exit(1); });
