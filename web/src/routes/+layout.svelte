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
		setAmbientVariable('--ambient-scroll-px', '0px');
		setAmbientVariable('--ambient-scroll-wave', '0');
		setAmbientVariable('--ambient-energy', '0');
		setAmbientVariable('--ambient-depth', '1');
		setAmbientVariable('--ambient-tilt-x', '0deg');
		setAmbientVariable('--ambient-tilt-y', '0deg');
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

		let ambientRAF = 0;
		let scrollRAF = 0;
		let maxScroll = Math.max(document.documentElement.scrollHeight - window.innerHeight, 1);
		let currentX = window.innerWidth * 0.5;
		let currentY = window.innerHeight * 0.35;
		let targetX = currentX;
		let targetY = currentY;
		let currentScrollY = window.scrollY;
		let targetScrollY = currentScrollY;
		let ambientEnergy = 0;
		let targetEnergy = 0;
		let lastPointerX = currentX;
		let lastPointerY = currentY;
		let lastPointerTime = performance.now();

		function updateMode(): void {
			root.classList.toggle('ambient-static', disabled());
		}

		function updateScrollBounds(): void {
			maxScroll = Math.max(document.documentElement.scrollHeight - window.innerHeight, 1);
		}

		function applyAmbientFrame(timeMs: number): void {
			const xPercent = clamp((currentX / Math.max(window.innerWidth, 1)) * 100, 0, 100);
			const yPercent = clamp((currentY / Math.max(window.innerHeight, 1)) * 100, 0, 100);
			const shiftX = ((xPercent - 50) / 50) * 52;
			const shiftY = ((yPercent - 50) / 50) * 38;
			const scrollProgress = clamp(currentScrollY / maxScroll, 0, 1);
			const scrollWave = (Math.sin(scrollProgress * Math.PI * 7 + timeMs * 0.0016) + 1) * 0.5;
			const tiltX = ((yPercent - 50) / 50) * -5.5;
			const tiltY = ((xPercent - 50) / 50) * 7;
			const depth = 1 + ambientEnergy * 0.28 + scrollProgress * 0.06;
			setAmbientVariable('--ambient-x', `${xPercent.toFixed(2)}%`);
			setAmbientVariable('--ambient-y', `${yPercent.toFixed(2)}%`);
			setAmbientVariable('--ambient-shift-x', `${shiftX.toFixed(2)}px`);
			setAmbientVariable('--ambient-shift-y', `${shiftY.toFixed(2)}px`);
			setAmbientVariable('--ambient-scroll', scrollProgress.toFixed(4));
			setAmbientVariable('--ambient-scroll-px', `${(scrollProgress * 140).toFixed(2)}px`);
			setAmbientVariable('--ambient-scroll-wave', scrollWave.toFixed(4));
			setAmbientVariable('--ambient-energy', ambientEnergy.toFixed(4));
			setAmbientVariable('--ambient-depth', depth.toFixed(4));
			setAmbientVariable('--ambient-tilt-x', `${tiltX.toFixed(2)}deg`);
			setAmbientVariable('--ambient-tilt-y', `${tiltY.toFixed(2)}deg`);
		}

		function animateAmbient(timeMs: number): void {
			const deltaX = targetX - currentX;
			const deltaY = targetY - currentY;
			const deltaScroll = targetScrollY - currentScrollY;
			currentX += deltaX * 0.11;
			currentY += deltaY * 0.11;
			currentScrollY += deltaScroll * 0.09;
			ambientEnergy += (targetEnergy - ambientEnergy) * 0.12;
			targetEnergy *= 0.93;
			applyAmbientFrame(timeMs);

			const keepAnimating =
				Math.abs(deltaX) + Math.abs(deltaY) > 0.36 ||
				Math.abs(deltaScroll) > 0.45 ||
				ambientEnergy > 0.01 ||
				targetEnergy > 0.01;

			if (keepAnimating) {
				ambientRAF = window.requestAnimationFrame(animateAmbient);
				return;
			}
			ambientRAF = 0;
		}

		function onPointerMove(event: PointerEvent): void {
			if (disabled()) return;
			const now = performance.now();
			const pointerDx = event.clientX - lastPointerX;
			const pointerDy = event.clientY - lastPointerY;
			const pointerDist = Math.hypot(pointerDx, pointerDy);
			const pointerDt = Math.max(now - lastPointerTime, 16);
			const pointerSpeed = clamp((pointerDist / pointerDt) * 0.52, 0, 1.2);
			targetEnergy = clamp(Math.max(targetEnergy, pointerSpeed), 0, 1.25);
			targetX = event.clientX;
			targetY = event.clientY;
			lastPointerX = event.clientX;
			lastPointerY = event.clientY;
			lastPointerTime = now;
			if (!ambientRAF) {
				ambientRAF = window.requestAnimationFrame(animateAmbient);
			}
		}

		function onScroll(): void {
			if (scrollRAF) return;
			scrollRAF = window.requestAnimationFrame(() => {
				scrollRAF = 0;
				updateScrollBounds();
				targetScrollY = clamp(window.scrollY, 0, maxScroll);
				targetEnergy = clamp(Math.max(targetEnergy, 0.46), 0, 1.25);
				if (!ambientRAF) {
					ambientRAF = window.requestAnimationFrame(animateAmbient);
				}
			});
		}

		function onResize(): void {
			updateScrollBounds();
			if (disabled()) return;
			targetX = clamp(targetX, 0, window.innerWidth);
			targetY = clamp(targetY, 0, window.innerHeight);
			targetScrollY = clamp(targetScrollY, 0, maxScroll);
			if (!ambientRAF) {
				ambientRAF = window.requestAnimationFrame(animateAmbient);
			}
		}

		function onPreferenceChange(): void {
			updateMode();
			updateScrollBounds();
			targetScrollY = window.scrollY;
			if (disabled()) {
				if (ambientRAF) {
					window.cancelAnimationFrame(ambientRAF);
					ambientRAF = 0;
				}
				ambientEnergy = 0;
				targetEnergy = 0;
				setAmbientDefaults();
				return;
			}
			if (!ambientRAF) {
				ambientRAF = window.requestAnimationFrame(animateAmbient);
			}
		}

		updateMode();
		updateScrollBounds();
		targetScrollY = window.scrollY;
		currentScrollY = targetScrollY;
		if (disabled()) {
			setAmbientDefaults();
		} else {
			applyAmbientFrame(performance.now());
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
			if (ambientRAF) {
				window.cancelAnimationFrame(ambientRAF);
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
		<div class="ambient-layer ambient-layer--halo"></div>
		<div class="ambient-layer ambient-layer--drift"></div>
		<div class="ambient-layer ambient-layer--ribbons"></div>
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
