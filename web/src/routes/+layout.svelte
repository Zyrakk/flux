<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { clearAuthToken, getAuthToken } from '$lib/api';

	let hasToken = false;

	function setGlyphVariable(name: string, value: string): void {
		document.documentElement.style.setProperty(name, value);
	}

	function setGlyphDefaults(): void {
		setGlyphVariable('--spot-x', '50%');
		setGlyphVariable('--spot-y', '38%');
		setGlyphVariable('--spot-radius', '220px');
		setGlyphVariable('--glyph-scroll', '0px');
	}

	function addMediaQueryListener(query: MediaQueryList, handler: () => void): () => void {
		if (typeof query.addEventListener === 'function') {
			query.addEventListener('change', handler);
			return () => query.removeEventListener('change', handler);
		}
		query.addListener(handler);
		return () => query.removeListener(handler);
	}

	function setupGlyphSpotlight(): () => void {
		const root = document.documentElement;
		const reduceMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		const coarsePointerQuery = window.matchMedia('(pointer: coarse)');
		const disabled = (): boolean => reduceMotionQuery.matches || coarsePointerQuery.matches;
		const clamp = (value: number, min: number, max: number): number => Math.min(Math.max(value, min), max);

		let lastClientX = window.innerWidth * 0.5;
		let lastClientY = window.innerHeight * 0.38;

		function updateMode(): void {
			root.classList.toggle('glyph-static', disabled());
		}

		function updateSpotlight(clientX: number, clientY: number): void {
			const xPercent = clamp((clientX / Math.max(window.innerWidth, 1)) * 100, 0, 100);
			const yPercent = clamp((clientY / Math.max(window.innerHeight, 1)) * 100, 0, 100);
			const radius = 200 + Math.round((1 - Math.abs(xPercent - 50) / 50) * 40);
			setGlyphVariable('--spot-x', `${xPercent.toFixed(2)}%`);
			setGlyphVariable('--spot-y', `${yPercent.toFixed(2)}%`);
			setGlyphVariable('--spot-radius', `${radius}px`);
		}

		function updateScrollShift(): void {
			setGlyphVariable('--glyph-scroll', `${Math.round(window.scrollY)}px`);
		}

		function onPointerMove(event: PointerEvent): void {
			if (disabled()) return;
			lastClientX = event.clientX;
			lastClientY = event.clientY;
			updateSpotlight(lastClientX, lastClientY);
		}

		function onScroll(): void {
			updateScrollShift();
		}

		function onResize(): void {
			updateScrollShift();
			if (disabled()) return;
			lastClientX = clamp(lastClientX, 0, window.innerWidth);
			lastClientY = clamp(lastClientY, 0, window.innerHeight);
			updateSpotlight(lastClientX, lastClientY);
		}

		function onPreferenceChange(): void {
			updateMode();
			updateScrollShift();
			if (disabled()) {
				setGlyphDefaults();
				return;
			}
			updateSpotlight(lastClientX, lastClientY);
		}

		updateMode();
		updateScrollShift();
		if (disabled()) {
			setGlyphDefaults();
		} else {
			updateSpotlight(lastClientX, lastClientY);
		}

		window.addEventListener('pointermove', onPointerMove, { passive: true });
		window.addEventListener('scroll', onScroll, { passive: true });
		window.addEventListener('resize', onResize, { passive: true });
		const removeReduceMotionListener = addMediaQueryListener(reduceMotionQuery, onPreferenceChange);
		const removeCoarsePointerListener = addMediaQueryListener(coarsePointerQuery, onPreferenceChange);

		return () => {
			window.removeEventListener('pointermove', onPointerMove);
			window.removeEventListener('scroll', onScroll);
			window.removeEventListener('resize', onResize);
			removeReduceMotionListener();
			removeCoarsePointerListener();
		};
	}

	onMount(() => {
		hasToken = getAuthToken() !== '';
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker
				.register('/service-worker.js', { updateViaCache: 'none' })
				.then((registration) => registration.update().catch(() => {}))
				.catch(() => {});
		}

		const cleanupGlyphSpotlight = setupGlyphSpotlight();
		return () => cleanupGlyphSpotlight();
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

<div class="site-shell">
	<div class="glyph-stage" aria-hidden="true">
		<div class="glyph-grid glyph-grid--base"></div>
		<div class="glyph-grid glyph-grid--glow"></div>
		<div class="glyph-spot"></div>
		<div class="glyph-vignette"></div>
	</div>

	<header class="site-header">
		<div class="site-header__inner">
			<a href="/" class="site-brand">
				<span class="site-brand__mark">F</span>
				<span class="site-brand__text">
					<span class="site-brand__title">Flux Intelligence</span>
					<span class="site-brand__subtitle">Daily Signal Briefing</span>
				</span>
			</a>

			<div class="flex items-center gap-2">
				<nav class="site-nav">
					{#each navItems as item}
						<a
							href={item.href}
							class="site-nav__link {isActive(item.href, $page.url.pathname) ? 'active' : ''}"
						>
							<span class="site-nav__link-icon">{item.icon}</span>
							{item.label}
						</a>
					{/each}
				</nav>

				{#if hasToken}
					<button class="btn-ghost !rounded-full !px-3 !py-2 !text-[11px]" on:click={logout}>
						Salir
					</button>
				{/if}
			</div>
		</div>
	</header>

	<main class="site-main">
		{#key $page.url.pathname}
			<div class="route-stage" in:fade={{ duration: 260 }} out:fade={{ duration: 140 }}>
				<slot />
			</div>
		{/key}
	</main>

	<footer class="site-footer">Flux · read less, know more</footer>
</div>
