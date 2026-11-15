// GradeStream — голосовой звонок (общий код для окна ведущего и страницы зрителя).
// Микрофон → 16 кГц моно → µ-law (G.711), блоки по 20 мс. Тишина не передаётся.
// Входящие голоса: у каждого собеседника свой небольшой буфер (~80 мс), всё смешивает Web Audio.
(function () {
  'use strict';
  const RATE = 16000, BLOCK = 320, OP = 5;          // 320 отсчётов = 20 мс
  const JITTER = 0.08, MAX_AHEAD = 0.45;            // буфер воспроизведения и предел накопления, с
  const GATE_DB = -50, HANG_MS = 400;               // ниже порога — тишина; после речи ещё 0,4 с

  // ---- µ-law ----
  const DEC = new Float32Array(256);
  for (let i = 0; i < 256; i++) {
    const u = ~i & 0xff, sign = u & 0x80, exp = (u >> 4) & 7, man = u & 0x0f;
    let s = ((man << 3) + 0x84) << exp; s -= 0x84;
    DEC[i] = (sign ? -s : s) / 32768;
  }
  function enc(x) {                                  // x: −1…1
    let s = Math.max(-32635, Math.min(32635, Math.round(x * 32767)));
    const sign = s < 0 ? 0x80 : 0; if (sign) s = -s;
    s += 0x84;
    let exp = 7; for (let m = 0x4000; (s & m) === 0 && exp > 0; m >>= 1) exp--;
    return ~(sign | (exp << 4) | ((s >> (exp + 3)) & 0x0f)) & 0xff;
  }

  // Захват: приводим поток микрофона к 16 кГц (усреднение по окну — заодно фильтр от наложения)
  const WORKLET = `
class GSCap extends AudioWorkletProcessor {
  constructor() { super(); this.r = sampleRate / ${RATE}; this.ph = 0; this.acc = 0; this.n = 0; this.buf = new Float32Array(${BLOCK}); this.i = 0; }
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (!ch) return true;
    for (let k = 0; k < ch.length; k++) {
      this.acc += ch[k]; this.n++; this.ph += 1;
      if (this.ph >= this.r) {
        this.ph -= this.r; this.buf[this.i++] = this.acc / this.n; this.acc = 0; this.n = 0;
        if (this.i === ${BLOCK}) { this.port.postMessage(this.buf); this.buf = new Float32Array(${BLOCK}); this.i = 0; }
      }
    }
    return true;
  }
}
registerProcessor('gs-cap', GSCap);`;

  function create(opt) {
    // opt.send(Uint8Array) — отправить блок; opt.ctx() — общий AudioContext страницы
    let ctx = null, out = null, mic = null, node = null, src = null, seq = 0, lastLoud = 0;
    let micLevel = 0, deviceId = '';
    const people = new Map();                        // id → { next, talkUntil, level }

    function audio() {
      if (!ctx) {
        ctx = opt.ctx();
        out = ctx.createGain(); out.gain.value = opt.volume ?? 1; out.connect(ctx.destination);
      }
      return ctx;
    }

    function onBlock(f) {
      let sum = 0; for (let i = 0; i < f.length; i++) sum += f[i] * f[i];
      const rms = Math.sqrt(sum / f.length), db = rms > 0 ? 20 * Math.log10(rms) : -100;
      micLevel = Math.max(micLevel * 0.7, Math.min(1, (db + 60) / 60));
      const now = performance.now();
      if (db > GATE_DB) lastLoud = now;
      if (now - lastLoud > HANG_MS) return;          // тишина — не передаём
      const p = new Uint8Array(4 + f.length);
      p[0] = OP; p[1] = (seq >> 8) & 0xff; p[2] = seq & 0xff; seq = (seq + 1) & 0xffff;
      p[3] = Math.max(0, Math.min(255, Math.round((db + 60) / 60 * 255)));
      for (let i = 0; i < f.length; i++) p[4 + i] = enc(f[i]);
      opt.send(p);
      if (window.__callTx) window.__callTx(p);           // для автоматических тестов
    }

    async function startMic(id) {
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) throw Object.assign(new Error('insecure'), { code: 'insecure' });
      if (id !== undefined) deviceId = id;
      stopMic();
      const c = audio();
      try { await c.resume(); } catch {}
      const a = { echoCancellation: true, noiseSuppression: true, autoGainControl: true, channelCount: 1 };
      if (deviceId) a.deviceId = { exact: deviceId };
      const s = await navigator.mediaDevices.getUserMedia({ audio: a });
      mic = s;
      src = c.createMediaStreamSource(s);
      if (c.audioWorklet) {
        if (!c.__gsCap) c.__gsCap = c.audioWorklet.addModule(URL.createObjectURL(new Blob([WORKLET], { type: 'text/javascript' })));
        await c.__gsCap;
        if (mic !== s) return;                       // за это время микрофон выключили
        node = new AudioWorkletNode(c, 'gs-cap', { numberOfInputs: 1, numberOfOutputs: 0 });
        node.port.onmessage = e => onBlock(e.data);
        src.connect(node);
      } else {                                       // старые браузеры
        const r = c.sampleRate / RATE; let ph = 0, acc = 0, n = 0, buf = new Float32Array(BLOCK), i = 0;
        node = c.createScriptProcessor(1024, 1, 1);
        node.onaudioprocess = e => {
          const ch = e.inputBuffer.getChannelData(0);
          for (let k = 0; k < ch.length; k++) { acc += ch[k]; n++; ph += 1;
            if (ph >= r) { ph -= r; buf[i++] = acc / n; acc = 0; n = 0; if (i === BLOCK) { onBlock(buf); buf = new Float32Array(BLOCK); i = 0; } } }
        };
        const mute = c.createGain(); mute.gain.value = 0;
        src.connect(node); node.connect(mute); mute.connect(c.destination);
      }
    }

    function stopMic() {
      if (mic) mic.getTracks().forEach(t => t.stop());
      try { src && src.disconnect(); } catch {}
      try { node && node.disconnect(); } catch {}
      if (node && node.port) node.port.onmessage = null;
      mic = node = src = null; micLevel = 0;
    }

    // Блок от собеседника: [5][длина id][id][номер, 2 байта][уровень][µ-law]
    function onPacket(buf) {
      if (!ctx) return;
      const u = new Uint8Array(buf);
      if (u[0] !== OP || u.length < 2) return;
      const L = u[1], o = 2 + L;
      if (u.length < o + 3) return;
      let id = ''; for (let i = 2; i < o; i++) id += String.fromCharCode(u[i]);
      const level = u[o + 2], n = u.length - o - 3;
      if (n <= 0) return;
      let p = people.get(id);
      if (!p) { p = { next: 0, talkUntil: 0, level: 0 }; people.set(id, p); }
      p.level = level / 255;
      if (level > 60) p.talkUntil = performance.now() + 350;
      const b = ctx.createBuffer(1, n, RATE), d = b.getChannelData(0);
      for (let i = 0; i < n; i++) d[i] = DEC[u[o + 3 + i]];
      if (window.__callRx) window.__callRx(id, d, (u[o] << 8) | u[o + 1]);   // для автоматических тестов
      const now = ctx.currentTime;
      if (p.next < now + 0.01) p.next = now + JITTER;       // начало фразы или пропуск — заново с запасом
      else if (p.next > now + MAX_AHEAD) return;            // накопилось — пропускаем, чтобы догнать
      const s = ctx.createBufferSource(); s.buffer = b; s.connect(out); s.start(p.next);
      p.next += b.duration;
    }

    return {
      startMic, stopMic,
      get micOn() { return !!mic; },
      get micLevel() { micLevel *= 0.9; return micLevel; },
      onPacket,
      ensureAudio: () => audio(),
      setVolume(v) { opt.volume = v; if (out) out.gain.value = v; },
      setSink(id) { audio(); if (ctx.setSinkId) return ctx.setSinkId(id || ''); return Promise.reject(new Error('unsupported')); },
      talking() { const t = performance.now(), r = []; for (const [id, p] of people) if (p.talkUntil > t) r.push(id); return r; },
      forget(id) { people.delete(id); },
    };
  }

  window.GSCall = { create, isOp: buf => new Uint8Array(buf, 0, 1)[0] === OP, secure: () => !!window.isSecureContext && !!navigator.mediaDevices?.getUserMedia };
})();
