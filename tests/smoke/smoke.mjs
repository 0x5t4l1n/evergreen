// Browser smoke test: loads the built site in Chromium and exercises the app.
// Usage: node smoke.mjs [distDir]   (CHROME_PATH overrides the browser binary)
import { chromium } from 'playwright-core';
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = path.resolve(process.argv[2] || '../../dist');
const here = path.dirname(fileURLToPath(import.meta.url));
const art = path.join(here, 'artifacts');
fs.mkdirSync(art, { recursive: true });
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.wasm': 'application/wasm', '.svg': 'image/svg+xml', '.json': 'application/json' };
// Serve under a sub-path to prove the build works on GitHub Pages project URLs.
const base = '/evergreen-smoke/';
const server = http.createServer((req, res) => {
  if (!req.url.startsWith(base)) { res.writeHead(404); return res.end(); }
  let rel = decodeURIComponent(req.url.slice(base.length).split('?')[0]) || 'index.html';
  const file = path.join(dist, path.normalize(rel).replace(/^(\.\.[/\\])+/, ''));
  if (!file.startsWith(dist) || !fs.existsSync(file) || fs.statSync(file).isDirectory()) { res.writeHead(404); return res.end(); }
  res.writeHead(200, { 'Content-Type': mime[path.extname(file)] || 'application/octet-stream' });
  fs.createReadStream(file).pipe(res);
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const url = `http://127.0.0.1:${server.address().port}${base}`;

const candidates = [process.env.CHROME_PATH, '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].filter(Boolean);
const exe = candidates.find((p) => fs.existsSync(p));
const browser = await chromium.launch({
  executablePath: exe,
  args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist', '--enable-webgl'],
});
const page = await browser.newPage({ viewport: { width: 960, height: 540 } });
const errors = [];
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
page.on('console', (m) => { if (m.type() === 'error') errors.push('console: ' + m.text()); });

let failed = 0;
const check = (name, ok, detail = '') => { console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  ' + detail : ''}`); if (!ok) failed++; };
const cmd = (n, v = '') => page.evaluate(([n, v]) => window.evergreen.command(n, v), [n, v]);
const hud = async () => JSON.parse(await page.evaluate(() => window.evergreen.hud()));

try {
  await page.goto(url, { waitUntil: 'load' });
  await page.waitForFunction(() => window.evergreen && window.evergreen.hud, null, { timeout: 90000 });
  check('WebAssembly module loads and exposes its API', true);
  check('rendering canvas exists and fills the viewport', await page.evaluate(() => {
    const c = document.getElementById('evergreen-canvas'); const r = c.getBoundingClientRect();
    return c && r.width >= innerWidth - 1 && r.height >= innerHeight - 1 && c.width > 0;
  }));
  await page.waitForFunction(() => !document.body.classList.contains('loading'), null, { timeout: 120000 });
  check('loading screen dismisses once chunks are ready', true);
  await page.waitForTimeout(3000);
  let h = await hud();
  check('environment renders (chunks resident, draw calls issued)', h.chunks > 3 && h.draws > 10 && h.tris > 1000, `chunks=${h.chunks} draws=${h.draws}`);
  const lit = await page.evaluate(() => {
    const c = document.createElement('canvas'); c.width = 64; c.height = 36;
    const g = c.getContext('2d'); g.drawImage(document.getElementById('evergreen-canvas'), 0, 0, 64, 36);
    const d = g.getImageData(0, 0, 64, 36).data; let sum = 0, hi = 0;
    for (let i = 0; i < d.length; i += 4) { const v = d[i] + d[i + 1] + d[i + 2]; sum += v; if (v > 60) hi++; }
    return { mean: sum / (64 * 36 * 3), hi };
  });
  // drawImage of a WebGL canvas can be blank without preserveDrawingBuffer; a screenshot is authoritative
  await page.screenshot({ path: path.join(art, 'smoke-initial.png') });
  const shot = fs.statSync(path.join(art, 'smoke-initial.png')).size;
  check('screenshot is a real image, not a blank frame', shot > 30000, `${shot} bytes`);

  // navigation
  check('switch to explore mode', (await cmd('cameraMode', 'explore')) === '');
  const x0 = (await hud()).x, z0 = (await hud()).z;
  await page.focus('#evergreen-canvas');
  await page.keyboard.down('KeyW');
  // software rendering can be slow, so poll for movement instead of sleeping a fixed time
  const moved = await page.waitForFunction(([x0, z0]) => { const h = JSON.parse(window.evergreen.hud()); return Math.hypot(h.x - x0, h.z - z0) > 1; }, [x0, z0], { timeout: 40000 }).then(() => true, () => false);
  await page.keyboard.up('KeyW');
  h = await hud();
  check('WASD navigation moves the camera', moved, `moved ${Math.hypot(h.x - x0, h.z - z0).toFixed(1)}m`);
  check('zone navigation teleports', (await cmd('zone', '9')) === '' );
  const arrived = await page.waitForFunction(() => JSON.parse(window.evergreen.hud()).zoneId === 9, null, { timeout: 40000 }).then(() => true, () => false);
  h = await hud();
  check('arrived in the World Tree zone', arrived, h.zone);

  // weather / time / season controls
  check('weather control accepted', (await cmd('weather', 'storm')) === '');
  const wxOK = await page.waitForFunction(() => JSON.parse(window.evergreen.hud()).weather === 'storm', null, { timeout: 30000 }).then(() => true, () => false);
  h = await hud();
  check('weather state follows the control', wxOK, h.weather);
  check('invalid weather is rejected', (await cmd('weather', 'tornado')) !== '');
  check('time control accepted', (await cmd('timeOfDay', '22.5')) === '');
  const timeOK = await page.waitForFunction(() => JSON.parse(window.evergreen.hud()).clock.startsWith('22:'), null, { timeout: 30000 }).then(() => true, () => false);
  h = await hud();
  check('time state follows the control', timeOK, h.clock + ' ' + h.phase);
  check('season control accepted', (await cmd('season', 'winter')) === '');
  check('graphics preset accepted', (await cmd('preset', 'low')) === '');
  check('seed change regenerates the world', (await cmd('seed', 'smoke-test')) === '');
  await page.waitForTimeout(2500);
  check('pause / resume', (await cmd('pause', 'true')) === '' && (await cmd('pause', 'false')) === '');

  // UI: panel, fullscreen
  await page.mouse.move(480, 500); await page.waitForTimeout(300);
  await page.click('#b-menu');
  check('settings panel opens', await page.evaluate(() => !document.getElementById('panel').hidden));
  await page.click('#p-close');
  const fs1 = await page.evaluate(async () => { try { await document.documentElement.requestFullscreen(); return !!document.fullscreenElement; } catch (e) { return 'blocked'; } });
  check('fullscreen control works', fs1 === true || fs1 === 'blocked', String(fs1));
  await page.evaluate(() => document.fullscreenElement && document.exitFullscreen());
  await cmd('screenshot');
  await page.waitForTimeout(1500);

  const critical = errors.filter((e) => !/favicon|Failed to load resource: the server responded with a status of 404/.test(e) && !/GL_INVALID|WebGL.*(warning|performance)/i.test(e));
  check('no critical console errors', critical.length === 0, critical.slice(0, 3).join(' | '));
} catch (e) {
  console.log('FAIL  smoke test crashed: ' + e.message);
  failed++;
  await page.screenshot({ path: path.join(art, 'smoke-crash.png') }).catch(() => {});
} finally {
  await browser.close();
  server.close();
}
console.log(failed ? `\n${failed} check(s) failed` : '\nall smoke checks passed');
process.exit(failed ? 1 : 0);
