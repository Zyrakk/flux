<svelte:options runes={true} />
<script lang="ts">
	import { onMount } from 'svelte';
	import { formatRelativeTime, priorityLabel } from '$lib/format';
	import type { Article } from '$lib/types';

	type SignalCardProps = {
		article: Article;
		sectionTint: string;
		index?: number;
	};

	let { article, sectionTint, index = 0 }: SignalCardProps = $props();
	let root = $state<HTMLElement | null>(null);
	let visible = $state(false);

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

	onMount(() => {
		const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
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
			{ threshold: 0.2 }
		);
		if (root) observer.observe(root);
		return () => observer.disconnect();
	});
</script>

<article
	bind:this={root}
	class={`signal-card reveal reveal--x ${visible ? 'is-visible' : ''}`}
	style={`--section-tint: ${sectionTint}; transition-delay: ${index * 100}ms;`}
>
	<div class="signal-card__meta">
		<span class={`priority-badge ${priorityClass(priorityOf(article))}`}>{priorityOf(article)}</span>
		<span class="signal-card__time">{utcTime(article.published_at ?? article.ingested_at)} UTC</span>
		<span class="flex-1"></span>
		<span class="signal-card__source">SRC:{article.source_type.toUpperCase()}</span>
	</div>
	<h3 class="signal-card__title">
		<a href={article.url} target="_blank" rel="noreferrer">{article.title}</a>
	</h3>
	<p class="signal-card__summary">{article.summary || 'Sin resumen disponible para esta señal.'}</p>
	<p class="mt-2 text-[11px] text-[rgba(255,255,255,0.27)]">{formatRelativeTime(article.published_at ?? article.ingested_at)}</p>
</article>
