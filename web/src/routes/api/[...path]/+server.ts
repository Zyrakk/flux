import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

const API_INTERNAL_URL = env.API_INTERNAL_URL ?? 'http://localhost:8080';

const proxy: RequestHandler = async ({ params, request, url, fetch }) => {
	const path = params.path ?? '';
	const upstreamURL = `${API_INTERNAL_URL.replace(/\/$/, '')}/api/${path}${url.search}`;

	const headers = new Headers(request.headers);
	headers.delete('host');
	headers.delete('connection');

	const init: RequestInit = {
		method: request.method,
		headers,
		redirect: 'manual'
	};

	if (request.method !== 'GET' && request.method !== 'HEAD') {
		init.body = await request.arrayBuffer();
	}

	const upstream = await fetch(upstreamURL, init);
	return new Response(upstream.body, {
		status: upstream.status,
		statusText: upstream.statusText,
		headers: upstream.headers
	});
};

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const OPTIONS = proxy;
export const HEAD = proxy;
