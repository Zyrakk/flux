<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { clearAuthToken, getAuthToken } from '$lib/api';

	let hasToken = false;

	onMount(() => {
		hasToken = getAuthToken() !== '';
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/service-worker.js').catch(() => {});
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

	const navItems = [
		{ href: '/', label: 'Briefing', icon: '◆' },
		{ href: '/feed', label: 'Feed', icon: '▤' },
		{ href: '/admin/sources', label: 'Admin', icon: '⚙' }
	];

	function isActive(href: string, pathname: string): boolean {
		if (href === '/') return pathname === '/';
		return pathname.startsWith(href);
	}
</script>

<svelte:head>
	<title>Flux</title>
</svelte:head>

<div class="flex min-h-screen flex-col">
	<!-- Header -->
	<header class="sticky top-0 z-30" style="background: rgba(6, 8, 12, 0.8); backdrop-filter: blur(20px) saturate(1.2); -webkit-backdrop-filter: blur(20px) saturate(1.2); border-bottom: 1px solid rgba(255,255,255,0.05);">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-4 py-3 sm:px-6">
			<!-- Logo -->
			<a href="/" class="group flex items-center gap-2.5 no-underline">
				<div class="flex h-8 w-8 items-center justify-center rounded-lg" style="background: linear-gradient(135deg, #06b6d4, #0891b2); box-shadow: 0 2px 12px -2px rgba(6,182,212,0.4);">
					<span class="font-mono text-xs font-bold text-slate-950">F</span>
				</div>
				<span class="text-sm font-semibold tracking-tight" style="color: var(--flux-text);">Flux</span>
			</a>

			<!-- Nav -->
			<nav class="flex items-center gap-1">
				{#each navItems as item}
					<a
						href={item.href}
						class="nav-link rounded-lg px-3 py-1.5 text-xs font-medium no-underline transition-all duration-200 {isActive(item.href, $page.url.pathname) ? 'active' : ''}"
						style="color: {isActive(item.href, $page.url.pathname) ? '#22d3ee' : 'var(--flux-text-muted)'}; background: {isActive(item.href, $page.url.pathname) ? 'rgba(6,182,212,0.1)' : 'transparent'};"
					>
						<span class="mr-1 opacity-60">{item.icon}</span>
						{item.label}
					</a>
				{/each}

				{#if hasToken}
					<div style="width:1px; height:16px; background: rgba(255,255,255,0.08); margin: 0 4px;"></div>
					<button
						class="rounded-lg px-2.5 py-1.5 text-[11px] font-medium transition-all duration-200"
						style="color: var(--flux-text-muted); background: transparent;"
						on:mouseenter={(e) => { e.currentTarget.style.color = 'var(--flux-danger)'; e.currentTarget.style.background = 'rgba(248,113,113,0.08)'; }}
						on:mouseleave={(e) => { e.currentTarget.style.color = 'var(--flux-text-muted)'; e.currentTarget.style.background = 'transparent'; }}
						on:click={logout}
					>
						Salir
					</button>
				{/if}
			</nav>
		</div>
	</header>

	<!-- Main -->
	<main class="mx-auto w-full max-w-5xl flex-1 px-4 py-6 sm:px-6">
		<slot />
	</main>

	<!-- Footer -->
	<footer class="py-4 text-center text-[11px]" style="color: var(--flux-text-muted);">
		Flux · read less, read better
	</footer>
</div>

<style>
	.nav-link:hover {
		color: var(--flux-text) !important;
		background: rgba(255, 255, 255, 0.04) !important;
	}

	.nav-link.active:hover {
		color: #22d3ee !important;
		background: rgba(6, 182, 212, 0.15) !important;
	}
</style>