<svelte:options runes={true} />
<script lang="ts">
	import { onMount } from 'svelte';

	let reticleEl = $state<HTMLDivElement | null>(null);
	let glowEl = $state<HTMLDivElement | null>(null);
	let enabled = $state(false);

	function addMediaQueryListener(query: MediaQueryList, handler: () => void): () => void {
		if (typeof query.addEventListener === 'function') {
			query.addEventListener('change', handler);
			return () => query.removeEventListener('change', handler);
		}
		query.addListener(handler);
		return () => query.removeListener(handler);
	}

	onMount(() => {
		const reduceMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
		const coarsePointerQuery = window.matchMedia('(pointer: coarse)');
		const isDisabled = (): boolean => reduceMotionQuery.matches || coarsePointerQuery.matches;

		const lerp = (a: number, b: number, amount: number): number => a + (b - a) * amount;

		let target = { x: window.innerWidth * 0.5, y: window.innerHeight * 0.5 };
		let reticlePos = { ...target };
		let glowPos = { ...target };
		let raf = 0;
		let running = false;

		const render = (): void => {
			reticlePos.x = lerp(reticlePos.x, target.x, 0.13);
			reticlePos.y = lerp(reticlePos.y, target.y, 0.13);
			glowPos.x = lerp(glowPos.x, target.x, 0.08);
			glowPos.y = lerp(glowPos.y, target.y, 0.08);

			if (reticleEl) {
				reticleEl.style.transform = `translate3d(${reticlePos.x - 22}px, ${reticlePos.y - 22}px, 0)`;
			}
			if (glowEl) {
				glowEl.style.transform = `translate3d(${glowPos.x - 250}px, ${glowPos.y - 250}px, 0)`;
			}
			raf = window.requestAnimationFrame(render);
		};

		const onPointerMove = (event: PointerEvent): void => {
			target = { x: event.clientX, y: event.clientY };
		};

		const start = (): void => {
			if (running || isDisabled()) {
				enabled = false;
				return;
			}
			running = true;
			enabled = true;
			window.addEventListener('pointermove', onPointerMove, { passive: true });
			raf = window.requestAnimationFrame(render);
		};

		const stop = (): void => {
			if (!running) return;
			running = false;
			enabled = false;
			window.removeEventListener('pointermove', onPointerMove);
			window.cancelAnimationFrame(raf);
		};

		const onPreferenceChange = (): void => {
			if (isDisabled()) {
				stop();
				return;
			}
			if (!running) {
				start();
			}
		};

		start();

		const removeReduceMotionListener = addMediaQueryListener(reduceMotionQuery, onPreferenceChange);
		const removeCoarsePointerListener = addMediaQueryListener(coarsePointerQuery, onPreferenceChange);

		return () => {
			stop();
			removeReduceMotionListener();
			removeCoarsePointerListener();
		};
	});
</script>

{#if enabled}
	<div bind:this={glowEl} class="cursor-reticle__glow" aria-hidden="true"></div>
	<div bind:this={reticleEl} class="cursor-reticle" aria-hidden="true">
		<div class="cursor-reticle__ring cursor-reticle__ring--outer"></div>
		<div class="cursor-reticle__ring cursor-reticle__ring--inner"></div>
		<div class="cursor-reticle__line cursor-reticle__line--h"></div>
		<div class="cursor-reticle__line cursor-reticle__line--v"></div>
		<div class="cursor-reticle__dot"></div>
	</div>
{/if}

<style>
	.cursor-reticle__glow {
		position: fixed;
		top: 0;
		left: 0;
		width: 500px;
		height: 500px;
		border-radius: 999px;
		background: radial-gradient(
			circle,
			rgba(6, 182, 212, 0.045) 0%,
			rgba(6, 182, 212, 0.015) 35%,
			transparent 65%
		);
		pointer-events: none;
		z-index: 1;
		will-change: transform;
	}

	.cursor-reticle {
		position: fixed;
		top: 0;
		left: 0;
		width: 44px;
		height: 44px;
		pointer-events: none;
		z-index: 50;
		will-change: transform;
	}

	.cursor-reticle__ring {
		position: absolute;
		border-radius: 999px;
		inset: 0;
	}

	.cursor-reticle__ring--outer {
		border: 1px solid rgba(6, 182, 212, 0.18);
		animation: reticle-spin 10s linear infinite;
	}

	.cursor-reticle__ring--inner {
		inset: 5px;
		border: 1px solid rgba(6, 182, 212, 0.08);
		animation: reticle-spin 15s linear infinite reverse;
	}

	.cursor-reticle__line {
		position: absolute;
		background: rgba(6, 182, 212, 0.2);
	}

	.cursor-reticle__line--h {
		top: 50%;
		left: 6px;
		right: 6px;
		height: 0.5px;
	}

	.cursor-reticle__line--v {
		left: 50%;
		top: 6px;
		bottom: 6px;
		width: 0.5px;
	}

	.cursor-reticle__dot {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 3px;
		height: 3px;
		background: #06b6d4;
		border-radius: 999px;
		transform: translate(-50%, -50%);
		box-shadow: 0 0 10px #06b6d4, 0 0 20px rgba(6, 182, 212, 0.3);
	}

	@keyframes reticle-spin {
		0% {
			transform: rotate(0deg);
		}
		100% {
			transform: rotate(360deg);
		}
	}
</style>
