/* Evergreen UI: a thin DOM layer. All simulation lives in Go/WASM; this file
   only forwards commands to window.evergreen.command and renders HUD state. */
(function () {
  'use strict';
  const $ = (id) => document.getElementById(id);
  const body = document.body;
  const modes = ['ambient', 'explore', 'scroll', 'cinematic'];
  let cfg = null, hud = {}, zoneList = [], idleTimer = 0, hidden = false, panelOpen = false;
  let toastT = 0, zoneT = 0, loreT = 0;

  const cmd = (name, value) => {
    if (!window.evergreen) return;
    const err = window.evergreen.command(name, value === undefined ? '' : value);
    if (err) toast(String(err));
    return err;
  };
  function toast(msg) {
    const t = $('toast'); t.textContent = msg; t.classList.add('show');
    clearTimeout(toastT); toastT = setTimeout(() => t.classList.remove('show'), 3800);
  }
  function setSeg(id, value) {
    document.querySelectorAll('#' + id + ' button').forEach((b) => b.classList.toggle('on', b.dataset.v === value));
  }
  function bindSeg(id, name) {
    $(id).addEventListener('click', (e) => {
      const b = e.target.closest('button'); if (!b) return;
      if (cmd(name, b.dataset.v) === '') setSeg(id, b.dataset.v);
    });
  }
  function syncControls() {
    if (!window.evergreen) return;
    try { cfg = JSON.parse(window.evergreen.config()); } catch (e) { return; }
    setSeg('s-mode', cfg.cameraMode); setSeg('s-preset', cfg.preset); setSeg('s-season', cfg.season);
    setSeg('s-timemode', cfg.timeMode);
    $('sel-weather').value = cfg.weather;
    $('r-time').value = cfg.timeOfDay; $('o-time').textContent = clock(cfg.timeOfDay);
    $('r-speed').value = cfg.timeSpeed; $('o-speed').textContent = cfg.timeSpeed;
    $('r-vol').value = cfg.volume; $('o-vol').textContent = Math.round(cfg.volume * 100);
    $('c-mute').checked = cfg.muted; $('c-auto').checked = cfg.autoQuality; $('c-fps').checked = cfg.showFps;
    $('c-motion').checked = cfg.reducedMotion;
    $('r-dist').value = cfg.drawDistance; $('o-dist').textContent = cfg.drawDistance ? cfg.drawDistance + ' chunks' : 'preset';
    $('t-seed').value = cfg.seed; $('t-gh').value = cfg.github || '';
    $('fps').hidden = !cfg.showFps;
    $('b-mode').title = 'Mode: ' + cfg.cameraMode;
    $('b-sound').classList.toggle('on', !cfg.muted && hud.audio);
  }
  const clock = (h) => { const hh = Math.floor(h), mm = Math.floor((h - hh) * 60); return String(hh).padStart(2, '0') + ':' + String(mm).padStart(2, '0'); };

  function buildZones() {
    const box = $('zones'); box.textContent = '';
    zoneList.forEach((z) => {
      const b = document.createElement('button');
      b.dataset.id = z.id; b.textContent = z.name; b.title = z.tagline;
      b.addEventListener('click', () => { cmd('zone', String(z.id)); closePanel(); });
      box.appendChild(b);
    });
  }
  function paintZones() {
    document.querySelectorAll('#zones button').forEach((b) => {
      const id = +b.dataset.id;
      b.classList.toggle('seen', (hud.discovered || []).includes(id));
      b.classList.toggle('here', hud.zoneId === id);
    });
  }

  function openPanel() { panelOpen = true; $('panel').hidden = false; $('b-menu').setAttribute('aria-expanded', 'true'); syncControls(); }
  function closePanel() { panelOpen = false; $('panel').hidden = true; $('b-menu').setAttribute('aria-expanded', 'false'); $('evergreen-canvas').focus(); }
  function toggleFullscreen() {
    if (document.fullscreenElement) document.exitFullscreen(); else if (document.documentElement.requestFullscreen) document.documentElement.requestFullscreen().catch(() => toast('Fullscreen was blocked'));
  }
  function setHidden(h) { hidden = h; body.classList.toggle('hidden', h); if (h) closePanel(); }

  // ---- idle fade ----------------------------------------------------------
  function wake() {
    body.classList.remove('idle'); clearTimeout(idleTimer);
    const delay = cfg && cfg.cameraMode === 'ambient' ? 3500 : 6500;
    idleTimer = setTimeout(() => { if (!panelOpen) body.classList.add('idle'); }, delay);
  }
  ['mousemove', 'pointerdown', 'touchstart', 'keydown'].forEach((e) => window.addEventListener(e, wake, { passive: true }));

  // ---- wiring ------------------------------------------------------------
  $('b-menu').addEventListener('click', () => (panelOpen ? closePanel() : openPanel()));
  $('p-close').addEventListener('click', closePanel);
  $('b-mode').addEventListener('click', () => { const i = modes.indexOf(cfg ? cfg.cameraMode : 'ambient'); cmd('cameraMode', modes[(i + 1) % 4]); syncControls(); toast('Mode: ' + cfg.cameraMode); });
  const pause = () => { cmd('pause', 'toggle'); };
  $('b-pause').addEventListener('click', pause); $('b-pause2').addEventListener('click', pause);
  $('b-sound').addEventListener('click', () => {
    if (!hud.audio && cfg && !cfg.muted) { cmd('audioStart'); } else { cmd('muted', String(!(cfg && cfg.muted))); }
    syncControls();
  });
  $('b-shot').addEventListener('click', () => cmd('screenshot'));
  $('b-full').addEventListener('click', toggleFullscreen);
  $('b-hide').addEventListener('click', () => { setHidden(true); toast('Interface hidden — press H or tap the top-left corner'); });
  $('b-reset').addEventListener('click', () => cmd('resetCamera'));
  bindSeg('s-mode', 'cameraMode'); bindSeg('s-preset', 'preset'); bindSeg('s-season', 'season'); bindSeg('s-timemode', 'timeMode');
  $('sel-weather').addEventListener('change', (e) => cmd('weather', e.target.value));
  $('r-time').addEventListener('input', (e) => { $('o-time').textContent = clock(+e.target.value); cmd('timeOfDay', e.target.value); setSeg('s-timemode', 'manual'); });
  $('r-speed').addEventListener('input', (e) => { $('o-speed').textContent = e.target.value; cmd('timeSpeed', e.target.value); });
  $('r-vol').addEventListener('input', (e) => { $('o-vol').textContent = Math.round(e.target.value * 100); cmd('volume', e.target.value); });
  $('r-dist').addEventListener('change', (e) => { const v = +e.target.value; $('o-dist').textContent = v ? v + ' chunks' : 'preset'; cmd('drawDistance', v && v < 2 ? '2' : String(v)); });
  $('c-mute').addEventListener('change', (e) => cmd('muted', String(e.target.checked)));
  $('c-auto').addEventListener('change', (e) => cmd('autoQuality', String(e.target.checked)));
  $('c-fps').addEventListener('change', (e) => { cmd('showFps', String(e.target.checked)); $('fps').hidden = !e.target.checked; });
  $('c-motion').addEventListener('change', (e) => cmd('reducedMotion', String(e.target.checked)));
  $('f-seed').addEventListener('submit', (e) => { e.preventDefault(); cmd('seed', $('t-seed').value); });
  $('b-dice').addEventListener('click', () => { const s = Math.floor(Math.random() * 1e9).toString(); $('t-seed').value = s; cmd('seed', s); });
  $('f-gh').addEventListener('submit', (e) => { e.preventDefault(); cmd('github', $('t-gh').value.trim()); });
  window.addEventListener('keydown', (e) => {
    if (/^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)) { if (e.key === 'Escape') e.target.blur(); return; }
    if (e.code === 'KeyH') setHidden(!hidden);
    else if (e.code === 'KeyF') toggleFullscreen();
    else if (e.code === 'KeyP') pause();
    else if (e.code === 'Escape' && panelOpen) closePanel();
    else if (e.code === 'KeyM') { if (window.evergreen) { cmd('muted', String(!(cfg && cfg.muted))); syncControls(); } }
    else if (e.code === 'Tab' && !panelOpen) { e.preventDefault(); openPanel(); }
  });
  document.addEventListener('pointerdown', (e) => { if (hidden && e.clientX < 70 && e.clientY < 70) setHidden(false); });

  // ---- API for Go ---------------------------------------------------------
  window.evergreenUI = {
    api() { try { zoneList = JSON.parse(window.evergreen.zones()); } catch (e) { zoneList = []; } buildZones(); syncControls(); wake(); },
    ready() { body.classList.remove('loading'); wake(); },
    state(json) {
      try { hud = JSON.parse(json); } catch (e) { return; }
      const f = $('fps');
      if (!f.hidden) f.textContent = hud.fps + ' fps  ' + hud.draws + ' draws  ' + Math.round(hud.tris / 1000) + 'k tris\nchunks ' + hud.chunks + (hud.pending ? ' (+' + hud.pending + ')' : '') + '  animals ' + hud.animals + '\nscale ' + hud.scale + '  ' + hud.preset + '  (' + hud.x + ', ' + hud.z + ')';
      const p = $('prompt'); p.textContent = hud.prompt || ''; p.classList.toggle('show', !!hud.prompt);
      $('b-pause').classList.toggle('on', hud.paused);
      $('b-pause2').textContent = hud.paused ? 'Resume' : 'Pause';
      if (panelOpen) {
        paintZones();
        $('stats').textContent = hud.clock + ' · ' + hud.phase + ' · ' + hud.weather + ' · ' + hud.season + (hud.githubOk ? ' · GitHub linked' : '');
      }
      if (cfg && cfg.cameraMode !== hud.mode) { cfg.cameraMode = hud.mode; setSeg('s-mode', hud.mode); }
      $('b-sound').classList.toggle('on', !!hud.audio);
    },
    zone(name, tagline, first) {
      const z = $('zone-title'); z.querySelector('b').textContent = name; z.querySelector('span').textContent = first ? 'Discovered · ' + tagline : tagline;
      z.classList.add('show'); clearTimeout(zoneT); zoneT = setTimeout(() => z.classList.remove('show'), 5200);
    },
    lore(text) { const l = $('lore'); l.textContent = text; l.classList.add('show'); clearTimeout(loreT); loreT = setTimeout(() => l.classList.remove('show'), 7000); },
    toast, mode(m) { if (cfg) cfg.cameraMode = m; setSeg('s-mode', m); },
    audio(on) { $('b-sound').classList.toggle('on', on); },
    log(msg) { console.warn('[evergreen]', msg); },
    progress(frac, msg) { $('loader-bar').style.width = Math.round(frac * 100) + '%'; if (msg) $('loader-msg').textContent = msg; },
    fatal(msg) {
      body.classList.remove('loading'); $('loader').style.display = 'none';
      $('fallback-msg').textContent = msg; $('fallback').hidden = false;
      console.error('[evergreen] fatal:', msg);
    },
  };
})();
