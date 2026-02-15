<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { clearAuthToken, getAuthToken } from '$lib/api';

	let hasToken = false;

	onMount(() => {
		hasToken = getAuthToken() !== '';
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/service-worker.js').catch(() => {
				// Keep silent: offline support is optional at runtime.
			});
		}
	});

	afterNavigate(() => {
		hasToken = getAuthToken() !== '';
	});

	async function logout() {
		clearAuthToken();
		hasToken = false;
		if (browser) {
			await goto('/login');
		}
	}
</script>

<svelte:head>
	<title>Flux</title>
</svelte:head>

<div class="min-h-screen">
	<header class="sticky top-0 z-20 border-b border-slate-800 bg-bg-0/90 backdrop-blur">
		<div class="mx-auto flex max-w-6xl items-center justify-between gap-3 px-4 py-3">
			<div class="flex items-center gap-3">
				<div class="rounded-lg bg-orange-500 px-2 py-1 font-mono text-xs font-semibold text-slate-950">Flux</div>
				<nav class="flex items-center gap-2 text-sm text-text-1">
					<a class="btn-secondary !px-2.5 !py-1.5" href="/">Briefing</a>
					<a class="btn-secondary !px-2.5 !py-1.5" href="/feed">Feed</a>
					<a class="btn-secondary !px-2.5 !py-1.5" href="/admin/sources">Admin</a>
				</nav>
			</div>

			{#if hasToken}
				<button class="btn-secondary !px-2.5 !py-1.5 text-xs" on:click={logout}>Salir</button>
			{/if}
		</div>
	</header>

	<main class="mx-auto max-w-6xl px-4 py-4">
		<slot />
	</main>
</div>
