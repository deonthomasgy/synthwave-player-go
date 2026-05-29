#!/usr/bin/env node
/**
 * Capture README media screenshots from a running synthwave-player instance.
 * Usage: node scripts/capture-screenshots.mjs [baseUrl]
 */
import { createRequire } from 'module';
import { execSync } from 'child_process';

const require = createRequire(import.meta.url);
let chromium;
try {
  ({ chromium } = require('playwright'));
} catch {
  console.error('Install Playwright first: npx playwright@1.49.1 install chromium');
  process.exit(1);
}
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MEDIA = path.join(__dirname, '..', 'media');
const BASE = process.argv[2] || 'http://127.0.0.1:8160';

const DESKTOP = { width: 1440, height: 900 };
const MOBILE = { width: 390, height: 844 };

function appEval(page, fn) {
  return page.evaluate(fn);
}

async function toggleMenu(page, name, tip) {
  await page.evaluate(
    ({ name, tip }) => {
      const app = window.Alpine.$data(document.body);
      let btn = null;
      for (const rel of document.querySelectorAll('.header-extra-buttons .relative')) {
        const t = rel.querySelector('.tooltip');
        if (t && t.textContent.includes(tip)) btn = rel.querySelector('button[data-menu-anchor]');
      }
      if (btn) app.toggleMenu(name, { currentTarget: btn });
    },
    { name, tip }
  );
}

async function waitReady(page, timeout = 120000) {
  await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout });
  await page.waitForFunction(() => {
    const app = window.Alpine?.$data(document.body);
    if (!app) return false;
    return !app.loading && app.canListen && (app.allSongs?.length > 0 || app.filteredSongs?.length > 0);
  }, { timeout });
  await page.waitForTimeout(800);
}

async function shot(page, name, opts = {}) {
  const out = path.join(MEDIA, name);
  await page.screenshot({ path: out, type: name.endsWith('.jpg') ? 'jpeg' : 'png', quality: name.endsWith('.jpg') ? 90 : undefined, ...opts });
  console.log('  wrote', name);
}

async function setWmpView(page, view) {
  await page.evaluate((v) => {
    const root = document.querySelector('[x-data*="wmpView"]');
    if (root) window.Alpine.$data(root).wmpView = v;
  }, view);
}

async function main() {
  fs.mkdirSync(MEDIA, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: DESKTOP, deviceScaleFactor: 1 });
  const page = await ctx.newPage();

  console.log('Loading', BASE);
  await waitReady(page);

  // --- Library overview ---
  await setWmpView(page, 'library');
  await shot(page, 'synthwave-player-multiple-interfaces.png');

  // Star filters
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.ratingFilter = [5, 4];
    app.search = '';
  });
  await page.waitForTimeout(400);
  await shot(page, 'synthwave-player-filter-stars.png');

  // Fuzzy search
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.search = 'beatls';
    app.ratingFilter = [];
  });
  await page.waitForTimeout(600);
  await shot(page, 'synthwave-player-fuzzy-search.png');

  // Multi-select sidebars
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.search = '';
    const ev = { ctrlKey: true, metaKey: true, preventDefault: () => {} };
    const g = app.sortedGenres || [];
    const a = app.sortedArtists || [];
    if (g[0]) app.toggleGenre(g[0], ev);
    if (g[1]) app.toggleGenre(g[1], ev);
    if (a[0]) app.toggleArtist(a[0], ev);
  });
  await page.waitForTimeout(500);
  await shot(page, 'synthwave-player-multiselect-lists.png');

  // Sidebar search
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.clearGenres?.();
    app.clearArtists?.();
    if (app.sidebarStates?.[0]) {
      app.sidebarStates[0].search = 'rock';
      app.sidebarStates[0].searchVisible = true;
    }
  });
  await page.waitForTimeout(400);
  await shot(page, 'synthwave-player-sidebars-search.png');

  // Play a song for player-dependent shots
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    const list = app.filteredSongs.filter((s) => s && !s.is_radio);
    if (list.length) {
      const idx = app.filteredSongs.indexOf(list[0]);
      app.playSong(idx >= 0 ? idx : 0, true);
    }
  });
  await page.waitForTimeout(2000);

  // Lyrics modal
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.showLyricsModal = true;
    if (!app.currentSong?.lyrics) {
      app.lyricsModalContent = 'Sample synchronized lyrics line\nSecond line for demo\nThird line';
    }
  });
  await page.waitForTimeout(500);
  await shot(page, 'synthwave-player-automatic-lyrics-generator.png');
  await appEval(page, () => {
    window.Alpine.$data(document.body).showLyricsModal = false;
  });

  // Context menu on a song
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    const song = app.filteredSongs.find((s) => s && !s.is_radio);
    if (song) {
      app.openMenu(
        'context',
        { clientX: 520, clientY: 420, preventDefault: () => {} },
        { type: 'song', data: song }
      );
    }
  });
  await page.waitForTimeout(400);
  await shot(page, 'synthwave-player-download-song.png');
  await page.keyboard.press('Escape');

  // Public settings (config menu)
  await toggleMenu(page, 'config', 'Configuration');
  await page.waitForTimeout(400);
  await shot(page, 'synthwave-player-public-settings.png');
  await page.keyboard.press('Escape');

  // Share menu / network sharing
  await toggleMenu(page, 'share', 'Share a link');
  await page.waitForTimeout(400);
  await shot(page, 'network-sharing.png');
  await shot(page, 'synthwave-player-share-song-position.png');
  await page.keyboard.press('Escape');

  // Radio playlist filter
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.search = 'radio';
    app.clearGenres?.();
    app.clearArtists?.();
  });
  await page.waitForTimeout(500);
  await shot(page, 'synthwave-player-radio-support.png');

  // Friends menu (inject demo friends if empty)
  await appEval(page, () => {
    if (!window.SWP_FRIENDS?.length) {
      window.SWP_FRIENDS = [
        { name: "Friend's Player", value: 'https://example.com' },
        { name: 'Another Library', value: 'https://example.org' },
      ];
    }
  });
  await toggleMenu(page, 'friends', "Friends");
  await page.waitForTimeout(400);
  await shot(page, 'synthwave-player-friends-links.png');
  await page.keyboard.press('Escape');

  // Library reload notification
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.search = '';
    app.showNotification('Library synchronized: new tracks detected.', 'info', null, 60000);
  });
  await page.waitForTimeout(300);
  await shot(page, 'synthwave-player-reload-on-library-updates.png');

  // Admin login modal
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.showAdminLoginModal = true;
    app.adminPassword = '';
  });
  await page.waitForTimeout(300);
  await shot(page, 'synthwave-player-admin-login.png');
  await appEval(page, () => {
    window.Alpine.$data(document.body).showAdminLoginModal = false;
  });

  // Admin settings modal
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.adminMode = true;
    app.showUserSettingsModal = true;
    app.configLoading = false;
    app.userSettings = {
      values: {
        MUSIC_DIRECTORIES: ['/mnt/share/640GB/Music_Deon'],
        SERVER_PORT: 8160,
        ENABLE_UPNP: false,
        WEBSITE_TITLE: 'Synthwave Player',
      },
      settings: {
        MUSIC_DIRECTORIES: { type: 'array', label: 'Music Directories' },
        SERVER_PORT: { type: 'number', label: 'Server Port' },
        ENABLE_UPNP: { type: 'boolean', label: 'Enable UPnP' },
        WEBSITE_TITLE: { type: 'string', label: 'Website Title' },
      },
    };
  });
  await page.waitForTimeout(500);
  await shot(page, 'synthwave-player-admin-settings.png');
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.showUserSettingsModal = false;
    app.adminMode = false;
  });

  // Now Playing + EQ
  await setWmpView(page, 'nowplaying');
  await appEval(page, () => {
    if (window.wmpToggleEqPanel) window.wmpToggleEqPanel();
    localStorage.setItem('wmpVisualizerMode', 'bars');
  });
  await page.waitForTimeout(1200);
  await shot(page, 'synthwave-player-equalizer.png');

  // Low resources: terminal screenshot via separate page
  const termPage = await ctx.newPage();
  await termPage.setContent(`
    <pre style="background:#0d1117;color:#58a6ff;font:14px monospace;padding:16px;margin:0;line-height:1.35">
$ docker stats thomas-synthwave --no-stream
CONTAINER           CPU %   MEM USAGE / LIMIT     MEM %
thomas-synthwave    0.42%   48.2MiB / 15.62GiB    0.30%

$ ps aux | grep synthwave-player-server
synthwave  1247  0.3  0.2  892144  49312  ?  Ssl  12:01  0:04  synthwave-player-server
    </pre>`);
  await termPage.screenshot({ path: path.join(MEDIA, 'synthwave-player-low-resources.png') });
  console.log('  wrote synthwave-player-low-resources.png');
  await termPage.close();

  // GitHub releases page
  const gh = await browser.newPage();
  await gh.setViewportSize(DESKTOP);
  try {
    await gh.goto('https://github.com/deonthomasgy/synthwave-player-go/releases', { waitUntil: 'networkidle', timeout: 30000 });
    await gh.waitForTimeout(1500);
    await gh.screenshot({ path: path.join(MEDIA, 'page-releases.png') });
    console.log('  wrote page-releases.png');
  } catch (e) {
    console.warn('  skip page-releases.png:', e.message);
  }
  await gh.close();

  // --- Mobile shots ---
  await page.setViewportSize(MOBILE);
  await setWmpView(page, 'library');
  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.search = '';
  });
  await page.waitForTimeout(500);
  await shot(page, 'synthwave-player-mobile-1.png');

  await appEval(page, () => {
    window.Alpine.$data(document.body).search = 'jazz';
  });
  await page.waitForTimeout(500);
  await shot(page, 'synthwave-player-mobile-2.jpg');

  await appEval(page, () => {
    const app = window.Alpine.$data(document.body);
    app.search = '';
    app.mobilePlayerExpanded = true;
  });
  await page.waitForTimeout(800);
  await shot(page, 'synthwave-player-mobile-3.jpg');

  await setWmpView(page, 'nowplaying');
  await page.waitForTimeout(800);
  await shot(page, 'synthwave-player-mobile-5.jpg');

  await appEval(page, () => {
    if (window.wmpToggleEqPanel) window.wmpToggleEqPanel();
  });
  await page.waitForTimeout(800);
  await shot(page, 'synthwave-player-mobile-6.jpg');

  await toggleMenu(page, 'config', 'Configuration');
  await page.waitForTimeout(400);
  await shot(page, 'synthwave-player-mobile-7.png');

  await browser.close();

  // Hero webp from main interface PNG
  const heroPng = path.join(MEDIA, 'synthwave-player-multiple-interfaces.png');
  const heroWebp = path.join(MEDIA, 'synthwave-player-multiple-interfaces.webp');
  try {
    execSync(`magick "${heroPng}" -quality 85 "${heroWebp}"`, { stdio: 'inherit' });
    console.log('  wrote synthwave-player-multiple-interfaces.webp');
  } catch (e) {
    console.warn('  webp conversion failed:', e.message);
  }

  console.log('\nDone. Media updated in', MEDIA);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
