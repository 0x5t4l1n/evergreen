/* Loads the Go WebAssembly module with progress and feature detection. */
(async function () {
  'use strict';
  const ui = window.evergreenUI;
  const fail = (m) => ui.fatal(m);
  if (typeof WebAssembly !== 'object') return fail('This browser does not support WebAssembly.');
  try {
    const probe = document.createElement('canvas');
    if (!probe.getContext('webgl2')) return fail('WebGL2 is unavailable (disabled, blocked or unsupported by the GPU).');
  } catch (e) { return fail('WebGL2 could not be initialised.'); }

  window.addEventListener('error', (e) => ui.log('error: ' + e.message));
  const go = new Go();
  try {
    ui.progress(0.05, 'Fetching the world engine…');
    const resp = await fetch('evergreen.wasm', { cache: 'no-cache' });
    if (!resp.ok) throw new Error('evergreen.wasm: HTTP ' + resp.status);
    const total = +resp.headers.get('content-length') || 0;
    let bytes;
    if (resp.body && total) {
      const reader = resp.body.getReader(); const chunks = []; let got = 0;
      for (;;) {
        const { done, value } = await reader.read(); if (done) break;
        chunks.push(value); got += value.length; ui.progress(0.05 + 0.7 * (got / total));
      }
      bytes = new Uint8Array(got); let o = 0; for (const c of chunks) { bytes.set(c, o); o += c.length; }
    } else { bytes = new Uint8Array(await resp.arrayBuffer()); }
    ui.progress(0.8, 'Compiling…');
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    ui.progress(0.92, 'Growing the forest…');
    go.run(instance).then(() => ui.log('wasm exited'));
  } catch (e) { fail('Could not start Evergreen: ' + (e && e.message ? e.message : e)); }
})();
