<svelte:options runes={true} />
<script lang="ts">
	import { onMount } from 'svelte';

	type BootProps = {
		text?: string;
		speed?: number;
		prompt?: string;
		className?: string;
		onComplete?: () => void;
	};

	let {
		text = 'FLUX v2.4.0 // CONNECTING SOURCES... PARSING FEEDS... ANALYZING... BRIEFING READY',
		speed = 20,
		prompt = '$',
		className = '',
		onComplete = () => {}
	}: BootProps = $props();

	let displayText = $state('');
	let done = $state(false);

	onMount(() => {
		const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		if (prefersReducedMotion) {
			displayText = text;
			done = true;
			onComplete();
			return;
		}

		let index = 0;
		const timer = window.setInterval(() => {
			displayText = text.slice(0, index);
			index += 1;
			if (index > text.length) {
				window.clearInterval(timer);
				window.setTimeout(() => {
					done = true;
					onComplete();
				}, 200);
			}
		}, Math.max(12, speed));

		return () => {
			window.clearInterval(timer);
		};
	});
</script>

<div class={`boot-sequence ${className}`.trim()}>
	<span class="boot-sequence__prompt">{prompt}</span>
	<span>{displayText}</span>
	{#if !done}
		<span class="boot-sequence__cursor">▌</span>
	{/if}
</div>
