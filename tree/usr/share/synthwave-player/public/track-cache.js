(function (global) {
    'use strict';

    const DB_NAME = 'synthwave-player';
    const DB_VERSION = 1;
    const STORE_NAME = 'libraries';
    const META_KEY = 'trackCacheMeta';
    const CACHE_LIMIT = 10000;

    let dbPromise = null;

    function open() {
        if (dbPromise) return dbPromise;
        dbPromise = new Promise((resolve, reject) => {
            const request = indexedDB.open(DB_NAME, DB_VERSION);
            request.onupgradeneeded = () => {
                const db = request.result;
                if (!db.objectStoreNames.contains(STORE_NAME)) {
                    db.createObjectStore(STORE_NAME);
                }
            };
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
        return dbPromise;
    }

    function stripSongForCache(song) {
        const copy = { ...song };
        delete copy.highlighted_title;
        delete copy.highlighted_artist;
        delete copy.highlighted_album;
        delete copy.title_html;
        delete copy.artist_html;
        delete copy.album_html;
        delete copy.highlight;
        return copy;
    }

    function isDefaultView(player) {
        if (!player) return false;
        if ((player.search || '').length > 0) return false;
        if (player.shuffle && player.shuffle !== 'off') return false;
        if (player.selectedPlaylists && player.selectedPlaylists.length > 0) return false;
        if (player.selectedGenres && player.selectedGenres.length > 0) return false;
        if (player.selectedArtists && player.selectedArtists.length > 0) return false;
        if (player.selectedAlbums && player.selectedAlbums.length > 0) return false;
        if (player.ratingFilter && player.ratingFilter.length > 0) return false;
        const cfg = global.SWP_CONFIG || {};
        const defaultSort = cfg.default_sort_by || 'title';
        const defaultDir = cfg.default_sort_order || 'asc';
        return player.sortCol === defaultSort && player.sortDir === defaultDir;
    }

    async function load(origin) {
        try {
            const db = await open();
            return await new Promise((resolve, reject) => {
                const tx = db.transaction(STORE_NAME, 'readonly');
                const req = tx.objectStore(STORE_NAME).get(origin);
                req.onsuccess = () => resolve(req.result || null);
                req.onerror = () => reject(req.error);
            });
        } catch (e) {
            console.warn('SWPTrackCache.load failed:', e);
            return null;
        }
    }

    async function save(origin, payload) {
        try {
            const songs = (payload.songs || []).slice(0, CACHE_LIMIT).map(stripSongForCache);
            const entry = {
                songs,
                total: payload.total || songs.length,
                sortCol: payload.sortCol,
                sortDir: payload.sortDir,
                cachedAt: payload.cachedAt || Date.now(),
            };
            const db = await open();
            await new Promise((resolve, reject) => {
                const tx = db.transaction(STORE_NAME, 'readwrite');
                tx.objectStore(STORE_NAME).put(entry, origin);
                tx.oncomplete = () => resolve();
                tx.onerror = () => reject(tx.error);
            });
            try {
                localStorage.setItem(META_KEY, JSON.stringify({
                    origin,
                    total: entry.total,
                    cachedAt: entry.cachedAt,
                }));
            } catch (_) { /* quota */ }
            return entry;
        } catch (e) {
            console.warn('SWPTrackCache.save failed:', e);
            return null;
        }
    }

    async function clear(origin) {
        try {
            const db = await open();
            await new Promise((resolve, reject) => {
                const tx = db.transaction(STORE_NAME, 'readwrite');
                tx.objectStore(STORE_NAME).delete(origin);
                tx.oncomplete = () => resolve();
                tx.onerror = () => reject(tx.error);
            });
            try {
                const meta = localStorage.getItem(META_KEY);
                if (meta) {
                    const parsed = JSON.parse(meta);
                    if (parsed.origin === origin) {
                        localStorage.removeItem(META_KEY);
                    }
                }
            } catch (_) { /* ignore */ }
        } catch (e) {
            console.warn('SWPTrackCache.clear failed:', e);
        }
    }

    function readMeta() {
        try {
            const raw = localStorage.getItem(META_KEY);
            return raw ? JSON.parse(raw) : null;
        } catch (_) {
            return null;
        }
    }

    global.SWPTrackCache = {
        CACHE_LIMIT,
        open,
        load,
        save,
        clear,
        isDefaultView,
        readMeta,
        stripSongForCache,
    };
})(typeof window !== 'undefined' ? window : globalThis);
