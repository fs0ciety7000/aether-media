/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true"/>
/// <reference lib="esnext" />
/// <reference lib="webworker" />
// Minimal, safe service worker (T-082): it caches the app shell and the versioned build assets
// so the interface opens fast and shows a sober offline page. It NEVER caches /api (data, images,
// video streams, HLS, subtitles), the internal proxy, or anything it did not precache itself.
import { build, files, version } from '$service-worker';

const sw = self as unknown as ServiceWorkerGlobalScope;
const CACHE = `aether-${version}`;
const SHELL = '/';
const OFFLINE = '/offline.html';

// Build output (hashed, immutable) and static files (icons, fonts, manifest, offline page).
const ASSETS = new Set([...build, ...files.filter((f) => !f.endsWith('.map') && !f.endsWith('.gitkeep'))]);

/** Paths the worker must never answer from a cache. */
function isPrivate(path: string): boolean {
	return path.startsWith('/api/') || path.startsWith('/internal/') || path === '/healthz';
}

sw.addEventListener('install', (event) => {
	event.waitUntil(
		(async () => {
			const cache = await caches.open(CACHE);
			await cache.addAll([...ASSETS]);
			// The SPA shell (index.html, served for every route): no user data in it.
			const shell = await fetch(SHELL, { cache: 'reload', credentials: 'same-origin' });
			if (shell.ok) await cache.put(SHELL, shell);
			await sw.skipWaiting();
		})()
	);
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		(async () => {
			for (const key of await caches.keys()) if (key !== CACHE && key.startsWith('aether-')) await caches.delete(key);
			await sw.clients.claim();
		})()
	);
});

sw.addEventListener('fetch', (event) => {
	const req = event.request;
	if (req.method !== 'GET' || req.headers.has('range')) return;
	const url = new URL(req.url);
	if (url.origin !== sw.location.origin || isPrivate(url.pathname)) return;

	// Pages: always the network (fresh shell, auth handled by the app); offline → cached shell,
	// which shows its own offline state, or the static offline page as a last resort.
	if (req.mode === 'navigate') {
		event.respondWith(
			(async () => {
				try {
					const res = await fetch(req);
					if (res.ok && res.headers.get('content-type')?.includes('text/html')) {
						const cache = await caches.open(CACHE);
						cache.put(SHELL, res.clone()).catch(() => {});
					}
					return res;
				} catch {
					const cache = await caches.open(CACHE);
					return (await cache.match(SHELL)) ?? (await cache.match(OFFLINE)) ?? Response.error();
				}
			})()
		);
		return;
	}

	// Versioned assets: cache first (they never change under the same URL).
	if (ASSETS.has(url.pathname)) {
		event.respondWith(
			(async () => {
				const cache = await caches.open(CACHE);
				const hit = await cache.match(url.pathname);
				if (hit) return hit;
				const res = await fetch(req);
				if (res.ok) cache.put(url.pathname, res.clone()).catch(() => {});
				return res;
			})()
		);
	}
	// Anything else goes straight to the network, untouched.
});
