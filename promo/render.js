// node render.js <p:0|1> stills "i:t,i:t"  |  node render.js <p> scenes "0,1,2"
const { chromium } = require('playwright'); const { spawn } = require('child_process'); const fs = require('fs');
const [p, mode, list] = process.argv.slice(2); const P = p === '1'; const W = P ? 1080 : 1920, H = P ? 1920 : 1080;
const D = __dirname + '/'; const FPS = 30;
(async () => {
  const b = await chromium.launch(); const pg = await b.newPage({ viewport: { width: W, height: H } });
  pg.on('pageerror', e => console.log('EXC', e.message));
  await pg.goto('file://' + D + 'film.html?p=' + p); await pg.evaluate(() => window.ready);
  const plan = await pg.evaluate(() => window.PLAN);
  const set = async (i, t) => { await pg.evaluate(([i, t]) => { setT(i, t); return Promise.all([...document.images].map(im => im.complete ? 0 : im.decode().catch(() => 0))); }, [i, t]); };
  if (mode === 'stills') {
    for (const it of list.split(',')) { const [i, t] = it.split(':').map(Number); await set(i, t); await set(i, t);
      await pg.screenshot({ path: `${D}still_${p}_${i}_${t}.jpg`, type: 'jpeg', quality: 90 }); }
  } else {
    for (const i of list.split(',').map(Number)) {
      const n = Math.round(plan[i][1] * FPS); const tmp = `${D}out/s${p}_${String(i).padStart(2, '0')}.tmp.mp4`;
      const ff = spawn('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'image2pipe', '-framerate', String(FPS), '-c:v', 'mjpeg', '-i', '-',
        '-c:v', 'libx264', '-crf', '18', '-preset', 'veryfast', '-pix_fmt', 'yuv420p', '-r', String(FPS), tmp], { stdio: ['pipe', 'inherit', 'inherit'] });
      const done = new Promise(r => ff.on('close', r));
      await set(i, 0);
      for (let f = 0; f < n; f++) { await set(i, f / FPS);
        const buf = await pg.screenshot({ type: 'jpeg', quality: 92 });
        if (!ff.stdin.write(buf)) await new Promise(r => ff.stdin.once('drain', r)); }
      ff.stdin.end(); const code = await done;
      if (code === 0) fs.renameSync(tmp, tmp.replace('.tmp.mp4', '.mp4')); console.log('scene', p, i, plan[i][0], n, 'frames, code', code);
    }
  }
  await b.close();
})();
