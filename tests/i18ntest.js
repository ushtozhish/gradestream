// Interface languages EN / RU / KZ: the host switcher, the server round trip, text fitting, and the client page by phone language.
// Saves screenshots to tests/shots/ (uploaded with the measurements).
const fs = require('fs'); const { sleep, startHost, scene, viewer } = require('./common');
const CYR = /[А-Яа-яЁё]/, KK = /[ӘәҒғҚқҢңӨөҰұҮүІі]/;
const overflow = page => page.evaluate(() => [...document.querySelectorAll('button, label, select, .tile, h1, h2, h3, .tab, option')]
  .filter(e => e.offsetParent !== null && e.scrollWidth > e.clientWidth + 1 && getComputedStyle(e).overflow !== 'visible')
  .map(e => (e.id ? '#' + e.id + ' ' : '') + e.textContent.trim().slice(0, 40)));
(async () => {
  fs.mkdirSync('shots', { recursive: true });
  const { b, host, url } = await startHost({ draw: scene });
  let fail = 0;
  // Server-side hints (e.g. «the Wi-Fi network is Public») are created in one language and must follow a later switch:
  // turn on the Wi-Fi link while the interface is Russian, then switch languages
  await host.click('#langSw [data-lang="ru"]'); await sleep(700);
  await host.click('label:has(input[name=mode][value=both])'); await sleep(2000);
  const warnRu = await host.evaluate(() => document.getElementById('lanWarn').textContent);
  console.log(`Wi-Fi warning (test network «Office Wi-Fi»): ${warnRu ? 'shown' : 'MISSING'}`); if (!warnRu) fail++;
  for (const lg of ['en', 'ru', 'kk']) {
    await host.click(`#langSw [data-lang="${lg}"]`); await sleep(900);
    const r = await host.evaluate(() => ({ lang: document.documentElement.lang, on: document.querySelector('#langSw .on')?.dataset.lang,
      text: document.querySelector('main, body').innerText }));
    const ov = await overflow(host);
    const okLang = r.lang === lg && r.on === lg;
    const okText = lg === 'en' ? !CYR.test(r.text.replace(/Қазақша|Русский/g, '')) : lg === 'kk' ? KK.test(r.text) : CYR.test(r.text) && !KK.test(r.text);
    await host.screenshot({ path: `shots/host-${lg}.png`, fullPage: true });
    const stray = lg === 'en' ? (r.text.replace(/Қазақша|Русский/g, '').match(/[^\n]*[А-Яа-яЁё][^\n]*/g) || []).slice(0, 3) : [];
    console.log(`host ${lg}: lang=${r.lang} switch=${r.on} text=${okText ? 'ok' : 'WRONG LANGUAGE: ' + stray.join(' | ')} overflow=${ov.length ? ov.join(' | ') : 'none'}`);
    if (!okLang || !okText || ov.length) fail++;
  }
  // The program keeps the choice (server round trip): reload the host page, KZ must stay
  await host.reload(); await sleep(1500);
  const kept = await host.evaluate(() => document.documentElement.lang);
  console.log(`host after reload: ${kept} (expected kk)`); if (kept !== 'kk') fail++;
  for (const [lg, locale] of [['en', 'en-US'], ['ru', 'ru-RU'], ['kk', 'kk-KZ'], ['en', 'de-DE']]) {
    const { vc, v } = await viewer(b, url, { width: 844, height: 390, dpr: 2, locale });
    await sleep(1500);
    const r = await v.evaluate(() => ({ lang: document.documentElement.lang, tiles: [...document.querySelectorAll('.tile .t')].filter(e => e.offsetParent).map(e => e.textContent) }));
    const ov = await overflow(v);
    await v.screenshot({ path: `shots/client-${locale}.png` });
    console.log(`client ${locale}: lang=${r.lang} tiles=[${r.tiles.join(', ')}] overflow=${ov.length ? ov.join(' | ') : 'none'}`);
    if (r.lang !== lg || ov.length) fail++;
    await vc.close();
  }
  console.log(fail ? `I18N: FAIL (${fail})` : 'I18N: PASS');
  await b.close(); process.exit(0);
})();
