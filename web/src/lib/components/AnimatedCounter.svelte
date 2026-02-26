<svelte:options runes={true} />
<script lang="ts">
	type AnimatedCounterProps = {
		value: number | string;
		duration?: number;
		color?: string;
		mono?: boolean;
		className?: string;
	};

	let { value, duration = 1000, color = 'inherit', mono = true, className = '' }: AnimatedCounterProps = $props();
	let display = $state(0);

	const numericValue = $derived(
		typeof value === 'number' ? value : Number.parseInt(String(value).replace(/[^\d-]/g, ''), 10)
	);
	const isNumeric = $derived(Number.isFinite(numericValue) && numericValue > 0);
	const rendered = $derived(isNumeric ? Math.round(display).toLocaleString() : String(value));

	$effect(() => {
		if (!isNumeric) {
			display = 0;
			return;
		}

		const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		if (reduceMotion) {
			display = numericValue;
			return;
		}

		const start = performance.now();
		let raf = 0;

		const tick = (now: number): void => {
			const progress = Math.min((now - start) / Math.max(120, duration), 1);
			display = numericValue * progress;
			if (progress < 1) {
				raf = requestAnimationFrame(tick);
			}
		};

		raf = requestAnimationFrame(tick);
		return () => cancelAnimationFrame(raf);
	});
</script>

<span
	class={className}
	style={`color: ${color}; font-family: ${mono ? "'JetBrains Mono', ui-monospace, monospace" : 'inherit'}; font-variant-numeric: tabular-nums;`}
>
	{rendered}
</span>
