<svelte:options runes={true} />
<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import CursorReticle from '$lib/components/CursorReticle.svelte';
	import HexGrid from '$lib/components/HexGrid.svelte';
	import { clearAuthToken, getAuthToken } from '$lib/api';

	let { children }: { children: Snippet } = $props();
	let hasToken = $state(false);
	let generatedAt = $state('');
	let totalBriefed = $state(0);
	let totalSources = $state(47);
	let nextGenTime = $state('');

	function toUTCLabel(date: Date): string {
		return `${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, '0')}-${String(date.getUTCDate()).padStart(2, '0')} ${String(date.getUTCHours()).padStart(2, '0')}:${String(date.getUTCMinutes()).padStart(2, '0')} UTC`;
	}

	function toUTCIso(date: Date): string {
		return `${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, '0')}-${String(date.getUTCDate()).padStart(2, '0')}T${String(date.getUTCHours()).padStart(2, '0')}:${String(date.getUTCMinutes()).padStart(2, '0')}Z`;
	}

	function computeNextGen(): string {
		const now = new Date();
		const next = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), 6, 0, 0, 0));
		if (now >= next) {
			next.setUTCDate(next.getUTCDate() + 1);
		}
		return toUTCIso(next);
	}

	function syncStatusClock(): void {
		generatedAt = toUTCLabel(new Date());
		nextGenTime = computeNextGen();
	}

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

	if (browser) {
		afterNavigate(() => {
			hasToken = getAuthToken() !== '';
		});
	}

	onMount(() => {
		hasToken = getAuthToken() !== '';
		syncStatusClock();

		if ('serviceWorker' in navigator) {
			navigator.serviceWorker
				.register('/service-worker.js', { updateViaCache: 'none' })
				.then((registration) => registration.update().catch(() => {}))
				.catch(() => {});
		}

		const timer = window.setInterval(syncStatusClock, 60_000);
		const onFluxStats = (event: Event): void => {
			const detail = (event as CustomEvent<{ briefed?: number; sources?: number }>).detail;
			if (typeof detail?.briefed === 'number') totalBriefed = detail.briefed;
			if (typeof detail?.sources === 'number') totalSources = detail.sources;
		};
		window.addEventListener('flux:stats', onFluxStats as EventListener);

		return () => {
			window.clearInterval(timer);
			window.removeEventListener('flux:stats', onFluxStats as EventListener);
		};
	});
</script>

<svelte:head>
	<title>Flux</title>
</svelte:head>

<div class="site-shell">
	<div class="void-stage" aria-hidden="true">
		<HexGrid />
		<div class="ambient-blob ambient-blob--cyan"></div>
		<div class="ambient-blob ambient-blob--violet"></div>
		<div class="ambient-blob ambient-blob--emerald"></div>
		<div class="noise-grain"></div>
		<div class="scan-line"></div>
	</div>

	<CursorReticle />

	<header class="site-header">
		<div class="site-header__inner">
			<a href="/" class="site-brand">
				<span class="site-brand__mark">F</span>
				<span>
					<span class="site-brand__title">FLUX</span>
					<span class="site-brand__subtitle">INTELLIGENCE BRIEFING SYSTEM</span>
				</span>
			</a>

			<div class="site-header__right">
				<nav class="site-nav">
					{#each navItems as item}
						<a
							href={item.href}
							class="site-nav__link {isActive(item.href, page.url.pathname) ? 'active' : ''}"
						>
							<span class="site-nav__link-icon">{item.icon}</span>
							{item.label}
						</a>
					{/each}
				</nav>

				{#if hasToken}
					<button class="btn-ghost !rounded-full !px-3 !py-2 !text-[11px]" onclick={logout}>Salir</button>
				{/if}

				<div class="site-status">
					<div>
						STATUS: <span class="site-status__ok">■ OPERATIONAL</span>
					</div>
					<div>GEN: {generatedAt}</div>
				</div>
			</div>
		</div>
	</header>

	<main class="site-main">
		{#key page.url.pathname}
			<div class="route-stage" in:fade={{ duration: 260 }} out:fade={{ duration: 140 }}>
				{@render children()}
			</div>
		{/key}
	</main>

	<footer class="site-footer">
		<div class="site-footer__inner">
			<div class="site-footer__line-primary">FLUX INTELLIGENCE · SIGNAL PROCESSING COMPLETE</div>
			<div class="site-footer__line-secondary">
				<strong>{totalBriefed.toLocaleString()}</strong> ITEMS BRIEFED · <strong>{totalSources.toLocaleString()}</strong> SOURCES ACTIVE · NEXT GEN <strong>{nextGenTime}</strong>
			</div>
		</div>
	</footer>
</div>
