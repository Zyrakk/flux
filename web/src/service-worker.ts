/// <reference lib="webworker" />

import { build, files, version } from '$service-worker';

declare const self: ServiceWorkerGlobalScope;

const STATIC_CACHE = `flux-static-${version}`;
const BRIEFING_CACHE = 'flux-briefing-cache';
const BRIEFING_ENDPOINT = '/api/briefings/latest';
const ASSETS = [...build, ...files];

self.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(STATIC_CACHE)
			.then((cache) => cache.addAll(ASSETS))
			.then(() => self.skipWaiting())
	);
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		(async () => {
			const cacheKeys = await caches.keys();
			await Promise.all(cacheKeys.filter((key) => key !== STATIC_CACHE && key !== BRIEFING_CACHE).map((key) => caches.delete(key)));
			await self.clients.claim();
		})()
	);
});

self.addEventListener('fetch', (event) => {
	if (event.request.method !== 'GET') {
		return;
	}

	const url = new URL(event.request.url);

	if (url.pathname === BRIEFING_ENDPOINT) {
		event.respondWith(networkFirstBriefing(event.request));
		return;
	}

	if (url.origin === self.location.origin) {
		event.respondWith(
			caches.match(event.request).then((cached) => {
				if (cached) {
					return cached;
				}
				return fetch(event.request);
			})
		);
	}
});

async function networkFirstBriefing(request: Request): Promise<Response> {
	const cache = await caches.open(BRIEFING_CACHE);
	try {
		const response = await fetch(request);
		if (response.ok) {
			await cache.put(request, response.clone());
		}
		return response;
	} catch {
		const cached = await cache.match(request);
		if (cached) {
			return cached;
		}
		return new Response(JSON.stringify({ error: 'offline' }), {
			status: 503,
			headers: { 'Content-Type': 'application/json' }
		});
	}
}
