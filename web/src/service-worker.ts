/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true"/>
/// <reference lib="esnext" />
/// <reference lib="webworker" />
// Installable PWA: the app shell is cached, API calls go to the network and
// fall back to the last copy, so saved data shows offline.
import { build, files, version } from '$service-worker';

const sw = self as unknown as ServiceWorkerGlobalScope;
const SHELL = `shell-${version}`;
const DATA = 'data-v1';
const ASSETS = [...build, ...files];

sw.addEventListener('install', (event) => {
	event.waitUntil(caches.open(SHELL).then((c) => c.addAll(ASSETS)).then(() => sw.skipWaiting()));
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== SHELL && k !== DATA).map((k) => caches.delete(k))))
	);
});

sw.addEventListener('fetch', (event) => {
	const req = event.request;
	if (req.method !== 'GET') return;
	const url = new URL(req.url);
	if (url.origin === sw.location.origin) {
		// App shell: cache first; any route falls back to index.html (single-page app).
		event.respondWith(
			caches.match(req).then((hit) => hit ?? fetch(req).catch(() => caches.match('/index.html') as Promise<Response>))
		);
		return;
	}
	if (url.pathname.startsWith('/v1/') && !url.pathname.startsWith('/v1/admin')) {
		// API reads: network first, last good answer when offline.
		event.respondWith(
			fetch(req)
				.then((res) => {
					if (res.ok) {
						const copy = res.clone();
						caches.open(DATA).then((c) => c.put(req, copy));
					}
					return res;
				})
				.catch(async () => (await caches.match(req)) ?? Response.error())
		);
	}
});

// Firebase Cloud Messaging web push ("New matches this morning").
sw.addEventListener('push', (event) => {
	const data = event.data?.json?.() ?? {};
	const n = data.notification ?? {};
	event.waitUntil(
		sw.registration.showNotification(n.title ?? 'CareerGlobe', {
			body: n.body ?? '',
			icon: '/icon-192.png',
			data: { link: data.data?.link ?? '/matches' }
		})
	);
});

sw.addEventListener('notificationclick', (event) => {
	event.notification.close();
	event.waitUntil(sw.clients.openWindow(event.notification.data?.link ?? '/matches'));
});
