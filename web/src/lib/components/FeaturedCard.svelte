<svelte:options runes={true} />
<script lang="ts">
	import { onMount } from 'svelte';
	import { formatRelativeTime, priorityLabel } from '$lib/format';
	import type { Article } from '$lib/types';

	type FeaturedCardProps = {
		article: Article;
		sectionTint: string;
		summary?: string;
		delay?: number;
	};

	let { article, sectionTint, summary, delay = 0 }: FeaturedCardProps = $props();

	let wrapper = $state<HTMLDivElement | null>(null);
	let card = $state<HTMLElement | null>(null);
	let visible = $state(false);
	let tiltEnabled = $state(true);

	function clamp(value: number, min: number, max: number): number {
		return Math.min(Math.max(value, min), max);
	}

	function utcTime(input?: string): string {
		if (!input) return '--:--';
		const date = new Date(input);
		if (Number.isNaN(date.getTime())) return '--:--';
		return date.toISOString().slice(11, 16);
	}

	function priorityOf(item: Article): string {
		const fromMeta = item.metadata && typeof item.metadata.priority === 'string' ? item.metadata.priority : undefined;
		return (fromMeta ?? priorityLabel(item.relevance_score)).toUpperCase();
	}

	function priorityClass(priority: string): string {
		switch (priority.toUpperCase()) {
			case 'CRITICAL':
				return 'priority-badge--critical';
			case 'HIGH':
				return 'priority-badge--high';
			case 'LOW':
				return 'priority-badge--low';
			default:
				return 'priority-badge--medium';
		}
	}

	function isTrending(item: Article): boolean {
		const fromMeta = Boolean(item.metadata && item.metadata.hot === true);
		const fromCategory = Boolean(item.categories?.some((entry) => entry.toLowerCase() === 'trending'));
		return fromMeta || fromCategory || priorityOf(item) === 'CRITICAL';
	}

	function onMouseMove(event: MouseEvent): void {
		if (!tiltEnabled || !card) return;
		const rect = card.getBoundingClientRect();
		const x = clamp(((event.clientX - rect.left) / rect.width - 0.5) * 12, -6, 6);
		const y = clamp(((event.clientY - rect.top) / rect.height - 0.5) * -12, -6, 6);
		card.style.transform = `perspective(900px) rotateX(${y}deg) rotateY(${x}deg) scale(1.008)`;
	}

	function onMouseLeave(): void {
		if (!card) return;
		card.style.transform = 'perspective(900px) rotateX(0deg) rotateY(0deg) scale(1)';
	}

	onMount(() => {
		const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		const coarse = window.matchMedia('(pointer: coarse)').matches;
		tiltEnabled = !(reduced || coarse);
		if (reduced) {
			visible = true;
			return;
		}

		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					visible = true;
					observer.disconnect();
				}
			},
			{ threshold: 0.15 }
		);
		if (wrapper) observer.observe(wrapper);
		return () => observer.disconnect();
	});
</script>

<div bind:this={wrapper} class={`reveal ${visible ? 'is-visible' : ''}`} style={`transition-delay: ${delay}ms;`}>
	<article
		bind:this={card}
		class="news-card-featured"
		style={`--section-tint: ${sectionTint};`}
		onmousemove={onMouseMove}
		onmouseleave={onMouseLeave}
	>
		<div class="featured-card__accent"></div>
		<div class="hud-corner tl"></div>
		<div class="hud-corner tr"></div>
		<div class="hud-corner bl"></div>
		<div class="hud-corner br"></div>

		<div class="featured-card__meta">
			<span class={`priority-badge ${priorityClass(priorityOf(article))}`}>{priorityOf(article)}</span>
			<span class="featured-card__time">{utcTime(article.published_at ?? article.ingested_at)} UTC</span>
			<span class="featured-card__source">SRC:{article.source_type.toUpperCase()}</span>
			<span class="flex-1"></span>
			{#if isTrending(article)}
				<span class="trending-badge">● TRENDING</span>
			{/if}
		</div>

		<h3 class="news-card-featured__title">
			<a href={article.url} target="_blank" rel="noreferrer">{article.title}</a>
		</h3>
		<p class="news-card-featured__summary">{summary || article.summary || 'Sin resumen disponible para esta señal principal.'}</p>
		<p class="mt-3 text-[11px] text-[rgba(255,255,255,0.3)]">
			{formatRelativeTime(article.published_at ?? article.ingested_at)}
		</p>
		<div class="featured-card__glow"></div>
	</article>
</div>
