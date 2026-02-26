<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { clearAuthToken, getAuthToken } from '$lib/api';

	let hasToken = false;

	function setAmbientVariable(name: string, value: string): void {
		document.documentElement.style.setProperty(name, value);
	}

	function setAmbientDefaults(): void {
		setAmbientVariable('--ambient-x', '50%');
		setAmbientVariable('--ambient-y', '35%');
		setAmbientVariable('--ambient-shift-x', '0px');
		setAmbientVariable('--ambient-shift-y', '0px');
		setAmbientVariable('--ambient-scroll', '0');
	}

	function addMediaQueryListener(query: MediaQueryList, handler: () => void): () => void {
		if (typeof query.addEventListener === 'function') {
			query.addEventListener('change', handler);
			return () => query.removeEventListener('change', handler);
		}
		query.addListener(handler);
		return () => query.removeListener(handler);
	}

	function setupAmbientMotion(): () => void {
		const root = document.documentElement;
		const reduceMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		const coarsePointerQuery = window.matchMedia('(pointer: coarse)');
		const clamp = (value: number, min: number, max: number): number => Math.min(Math.max(value, min), max);
		const disabled = (): boolean => reduceMotionQuery.matches || coarsePointerQuery.matches;

		let pointerRAF = 0;
		let scrollRAF = 0;
		let currentX = window.innerWidth * 0.5;
		let currentY = window.innerHeight * 0.35;
		let targetX = currentX;
		let targetY = currentY;

		function updateMode(): void {
			root.classList.toggle('ambient-static', disabled());
		}

		function updateScrollProgress(): void {
			const maxScroll = Math.max(document.documentElement.scrollHeight - window.innerHeight, 1);
			const progress = clamp(window.scrollY / maxScroll, 0, 1);
			setAmbientVariable('--ambient-scroll', progress.toFixed(4));
		}

		function applyPointerPosition(): void {
			const xPercent = clamp((currentX / Math.max(window.innerWidth, 1)) * 100, 0, 100);
			const yPercent = clamp((currentY / Math.max(window.innerHeight, 1)) * 100, 0, 100);
			const shiftX = ((xPercent - 50) / 50) * 24;
			const shiftY = ((yPercent - 50) / 50) * 18;
			setAmbientVariable('--ambient-x', `${xPercent.toFixed(2)}%`);
			setAmbientVariable('--ambient-y', `${yPercent.toFixed(2)}%`);
			setAmbientVariable('--ambient-shift-x', `${shiftX.toFixed(2)}px`);
			setAmbientVariable('--ambient-shift-y', `${shiftY.toFixed(2)}px`);
		}

		function animatePointer(): void {
			const deltaX = targetX - currentX;
			const deltaY = targetY - currentY;
			currentX += deltaX * 0.08;
			currentY += deltaY * 0.08;
			applyPointerPosition();

			if (Math.abs(deltaX) + Math.abs(deltaY) > 0.4) {
				pointerRAF = window.requestAnimationFrame(animatePointer);
				return;
			}
			pointerRAF = 0;
		}

		function onPointerMove(event: PointerEvent): void {
			if (disabled()) return;
			targetX = event.clientX;
			targetY = event.clientY;
			if (!pointerRAF) {
				pointerRAF = window.requestAnimationFrame(animatePointer);
			}
		}

		function onScroll(): void {
			if (scrollRAF) return;
			scrollRAF = window.requestAnimationFrame(() => {
				scrollRAF = 0;
				updateScrollProgress();
			});
		}

		function onResize(): void {
			updateScrollProgress();
			if (disabled()) return;
			targetX = clamp(targetX, 0, window.innerWidth);
			targetY = clamp(targetY, 0, window.innerHeight);
			if (!pointerRAF) {
				pointerRAF = window.requestAnimationFrame(animatePointer);
			}
		}

		function onPreferenceChange(): void {
			updateMode();
			updateScrollProgress();
			if (disabled()) {
				if (pointerRAF) {
					window.cancelAnimationFrame(pointerRAF);
					pointerRAF = 0;
				}
				setAmbientDefaults();
				return;
			}
			if (!pointerRAF) {
				pointerRAF = window.requestAnimationFrame(animatePointer);
			}
		}

		updateMode();
		updateScrollProgress();
		if (disabled()) {
			setAmbientDefaults();
		} else {
			applyPointerPosition();
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
			if (pointerRAF) {
				window.cancelAnimationFrame(pointerRAF);
			}
			if (scrollRAF) {
				window.cancelAnimationFrame(scrollRAF);
			}
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

		const cleanupAmbient = setupAmbientMotion();
		return () => cleanupAmbient();
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
	<div class="ambient-stage" aria-hidden="true">
		<div class="ambient-layer ambient-layer--base"></div>
		<div class="ambient-layer ambient-layer--pointer"></div>
		<div class="ambient-layer ambient-layer--drift"></div>
		<div class="ambient-layer ambient-layer--noise"></div>
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
