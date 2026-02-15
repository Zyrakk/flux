import { browser } from '$app/environment';

export const AUTH_TOKEN_KEY = 'flux_auth_token';

export function getAuthToken(): string {
	if (!browser) {
		return '';
	}
	return localStorage.getItem(AUTH_TOKEN_KEY)?.trim() ?? '';
}

export function setAuthToken(token: string): void {
	if (!browser) {
		return;
	}
	localStorage.setItem(AUTH_TOKEN_KEY, token.trim());
}

export function clearAuthToken(): void {
	if (!browser) {
		return;
	}
	localStorage.removeItem(AUTH_TOKEN_KEY);
}

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
	const url = path.startsWith('/api') ? path : `/api${path}`;
	const headers = new Headers(init.headers);
	const token = getAuthToken();
	if (token !== '') {
		headers.set('Authorization', `Bearer ${token}`);
	}
	if (!headers.has('Content-Type') && init.body != null && !(init.body instanceof FormData)) {
		headers.set('Content-Type', 'application/json');
	}

	const response = await fetch(url, {
		...init,
		headers
	});

	if (response.status === 401) {
		throw new Error('UNAUTHORIZED');
	}

	return response;
}

export async function apiJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
	const response = await apiFetch(path, init);
	if (!response.ok) {
		const errorText = (await response.text()) || `HTTP ${response.status}`;
		throw new Error(errorText);
	}
	return (await response.json()) as T;
}
