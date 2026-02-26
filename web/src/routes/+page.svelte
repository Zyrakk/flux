<svelte:options runes={true} />
<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { marked } from 'marked';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import BootSequence from '$lib/components/BootSequence.svelte';
	import FeaturedCard from '$lib/components/FeaturedCard.svelte';
	import SectionHeader from '$lib/components/SectionHeader.svelte';
	import SignalCard from '$lib/components/SignalCard.svelte';
	import StatsDashboard from '$lib/components/StatsDashboard.svelte';
	import { apiFetch, apiJSON } from '$lib/api';
	import { isSameCalendarDay, priorityLabel, sectionColor, sectionTint } from '$lib/format';
	import type { Article, Briefing, FeedbackAction } from '$lib/types';

	type SectionConfig = {
		name: string;
		displayName: string;
		hudLabel: string;
	};

	const sections: SectionConfig[] = [
		{ name: 'cybersecurity', displayName: 'Cybersecurity', hudLabel: 'CYBERSEC' },
		{ name: 'tech', displayName: 'Tech', hudLabel: 'TECH' },
		{ name: 'economy', displayName: 'Economy', hudLabel: 'ECONOMY' },
		{ name: 'world', displayName: 'World', hudLabel: 'GEOPOLITICS' }
	];

	let briefing = $state<Briefing | null>(null);
	let markdownHtml = $state('');
	let loading = $state(true);
	let error = $state('');
	let noBriefingToday = $state(false);
	let bootReady = $state(false);
	let markdownOpen = $state(true);
	let activeSection = $state('cybersecurity');

	const grouped = $derived(groupArticlesBySection(briefing?.articles ?? []));
	const totalBriefed = $derived(briefing?.articles.length ?? 0);
	const activeSources = $derived(readMetaNumber(['source_count', 'sources_active', 'active_sources'], 47));
	const ingestedTotal = $derived(readMetaNumber(['ingested_count', 'total_ingested', 'ingested'], briefing?.articles.length ?? 0));
	const processedTotal = $derived(
		readMetaNumber(['processed_count', 'total_processed', 'processed'], briefing?.articles.length ?? 0)
	);
	const threatLevel = $derived(hasCritical(grouped.cybersecurity ?? []) ? 'ELEVATED' : 'GUARDED');

	const stats = $derived([
		{ label: 'SOURCES', value: activeSources, color: '#06b6d4' },
		{ label: 'INGESTED', value: ingestedTotal, color: 'rgba(255,255,255,0.65)' },
		{ label: 'PROCESSED', value: processedTotal, color: 'rgba(255,255,255,0.45)' },
		{ label: 'BRIEFED', value: totalBriefed, color: '#a78bfa' },
		{ label: 'THREAT LVL', value: threatLevel, color: '#fbbf24', mono: true }
	]);

	function groupArticlesBySection(articles: Article[]): Record<string, Article[]> {
		const out: Record<string, Article[]> = {
			cybersecurity: [],
			tech: [],
			economy: [],
			world: []
		};
		for (const article of articles) {
			const key = article.section?.name ?? 'tech';
			if (!out[key]) out[key] = [];
			out[key].push(article);
		}
		return out;
	}

	function readMetaNumber(keys: string[], fallback: number): number {
		const metadata = briefing?.metadata as Record<string, unknown> | undefined;
		if (!metadata) return fallback;
		for (const key of keys) {
			const value = metadata[key];
			if (typeof value === 'number' && Number.isFinite(value)) {
				return value;
			}
		}
		return fallback;
	}

	function articlePriority(article: Article): string {
		const metaPriority = article.metadata && typeof article.metadata.priority === 'string' ? article.metadata.priority : undefined;
		return (metaPriority ?? priorityLabel(article.relevance_score)).toUpperCase();
	}

	function hasCritical(articles: Article[]): boolean {
		return articles.some((article) => articlePriority(article) === 'CRITICAL');
	}

	function priorityClass(article: Article): string {
		switch (articlePriority(article)) {
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

	function sectionCount(sectionName: string): number {
		return grouped[sectionName]?.length ?? 0;
	}

	function jumpToSection(sectionName: string): void {
		activeSection = sectionName;
		if (!browser) return;
		document.getElementById(`section-${sectionName}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	async function loadLatestBriefing() {
		loading = true;
		error = '';
		noBriefingToday = false;
		try {
			const payload = await apiJSON<Briefing>('/briefings/latest');
			if (!isSameCalendarDay(payload.generated_at)) {
				noBriefingToday = true;
				briefing = null;
				markdownHtml = '';
				emitFooterStats(0, activeSources);
				return;
			}
			briefing = payload;
			markdownHtml = String(marked.parse(payload.content || ''));
			emitFooterStats(payload.articles.length, readMetaNumber(['source_count', 'sources_active', 'active_sources'], 47));
		} catch (err) {
			const message = err instanceof Error ? err.message : 'No se pudo cargar el briefing';
			if (message.includes('UNAUTHORIZED')) {
				await goto('/login');
				return;
			}
			if (message.toLowerCase().includes('no briefings generated yet') || message.includes('404')) {
				noBriefingToday = true;
				briefing = null;
				markdownHtml = '';
				emitFooterStats(0, activeSources);
				return;
			}
			error = message;
		} finally {
			loading = false;
		}
	}

	function emitFooterStats(briefed: number, sources: number): void {
		if (!browser) return;
		window.dispatchEvent(new CustomEvent('flux:stats', { detail: { briefed, sources } }));
	}

	function bootComplete(): void {
		bootReady = true;
	}

	async function onFeedback(article: Article, action: FeedbackAction) {
		const active = getActionActive(article, action);
		try {
			const feedbackID = getActionFeedbackID(article, action);
			if (active && feedbackID) {
				await apiFetch(`/feedback/${feedbackID}`, { method: 'DELETE' });
				setActionActive(article, action, false);
				setActionFeedbackID(article, action, undefined);
				adjustActionCount(article, action, -1);
				briefing = briefing;
				return;
			}

			const response = await apiJSON<{ feedback: { id: string } }>('/feedback', {
				method: 'POST',
				body: JSON.stringify({ article_id: article.id, action })
			});
			setActionFeedbackID(article, action, response.feedback.id);

			if (!getActionActive(article, action)) {
				adjustActionCount(article, action, 1);
			}
			setActionActive(article, action, true);

			if (action === 'like' && article.feedback.disliked) {
				article.feedback.disliked = false;
				article.feedback.dislikes = Math.max(0, article.feedback.dislikes - 1);
			}
			if (action === 'dislike' && article.feedback.liked) {
				article.feedback.liked = false;
				article.feedback.likes = Math.max(0, article.feedback.likes - 1);
			}

			briefing = briefing;
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Error enviando feedback';
			if (message.includes('UNAUTHORIZED')) {
				await goto('/login');
				return;
			}
			alert(message);
		}
	}

	function getActionActive(article: Article, action: FeedbackAction): boolean {
		switch (action) {
			case 'like':
				return article.feedback.liked;
			case 'dislike':
				return article.feedback.disliked;
			case 'save':
				return article.feedback.saved;
		}
	}

	function setActionActive(article: Article, action: FeedbackAction, value: boolean): void {
		switch (action) {
			case 'like':
				article.feedback.liked = value;
				break;
			case 'dislike':
				article.feedback.disliked = value;
				break;
			case 'save':
				article.feedback.saved = value;
				break;
		}
	}

	function adjustActionCount(article: Article, action: FeedbackAction, delta: number): void {
		switch (action) {
			case 'like':
				article.feedback.likes = Math.max(0, article.feedback.likes + delta);
				break;
			case 'dislike':
				article.feedback.dislikes = Math.max(0, article.feedback.dislikes + delta);
				break;
			case 'save':
				article.feedback.saves = Math.max(0, article.feedback.saves + delta);
				break;
		}
	}

	function getActionFeedbackID(article: Article, action: FeedbackAction): string | undefined {
		switch (action) {
			case 'like':
				return article.feedback.like_id;
			case 'dislike':
				return article.feedback.dislike_id;
			case 'save':
				return article.feedback.save_id;
		}
	}

	function setActionFeedbackID(article: Article, action: FeedbackAction, id?: string): void {
		switch (action) {
			case 'like':
				article.feedback.like_id = id;
				break;
			case 'dislike':
				article.feedback.dislike_id = id;
				break;
			case 'save':
				article.feedback.save_id = id;
				break;
		}
	}

	onMount(() => {
		void loadLatestBriefing();
	});
</script>

<section class="briefing-page">
	<BootSequence onComplete={bootComplete} />

	{#if loading}
		<div class="panel surface-pad text-center">
			<div class="loading-pulse text-sm text-[rgba(255,255,255,0.45)]">Conectando fuentes y generando briefing...</div>
		</div>
	{:else if error}
		<div class="alert error">{error}</div>
	{:else if noBriefingToday}
		<div class="panel surface-pad text-center">
			<div class="mx-auto mb-3 inline-flex h-14 w-14 items-center justify-center rounded-2xl border border-[rgba(6,182,212,0.25)] bg-[rgba(6,182,212,0.08)] text-xl text-[var(--flux-accent)]">
				◆
			</div>
			<h1 class="text-xl font-extrabold tracking-tight text-[rgba(255,255,255,0.9)]">No hay briefing disponible todavía</h1>
			<p class="mx-auto mt-2 max-w-xl text-sm text-[rgba(255,255,255,0.5)]">
				El próximo briefing diario se genera durante el siguiente ciclo UTC.
			</p>
			<div class="mt-5 flex flex-wrap items-center justify-center gap-2">
				<a class="btn-ghost" href="/feed">Abrir feed</a>
				<a class="btn-ghost" href="/admin/sources">Gestionar fuentes</a>
			</div>
		</div>
	{:else if briefing}
		<div class={`briefing-fade ${bootReady ? 'ready' : ''}`}>
			<div class="mb-5">
				<StatsDashboard stats={stats} />
			</div>

			<div class="hud-separator mb-4"></div>

			<div class="mb-4 flex flex-wrap gap-2">
				{#each sections as sec}
					<button
						type="button"
						class={`section-tab ${activeSection === sec.name ? 'active' : ''}`}
						style={`--section-tint: ${sectionTint(sec.name)}; color: ${activeSection === sec.name ? sectionColor(sec.name) : ''};`}
						onclick={() => jumpToSection(sec.name)}
					>
						{sec.displayName}
						<span class="text-[10px] text-[rgba(255,255,255,0.45)]">{sectionCount(sec.name)}</span>
					</button>
				{/each}
			</div>

			<div class="panel surface-pad mb-4">
				<div class="mb-3 flex items-center justify-between gap-3">
					<h2 class="font-mono text-[11px] font-bold uppercase tracking-[0.18em] text-[rgba(255,255,255,0.32)]">
						ANALYST BRIEFING
					</h2>
					<button class="btn-ghost !px-3 !py-1.5 !text-[10px]" onclick={() => (markdownOpen = !markdownOpen)}>
						{markdownOpen ? 'Ocultar' : 'Mostrar'}
					</button>
				</div>
				{#if markdownOpen}
					<div class="briefing-markdown prose prose-invert prose-flux max-w-none">
						{@html markdownHtml}
					</div>
				{/if}
			</div>

			{#each sections as sec, sectionIndex}
				{@const articles = grouped[sec.name] ?? []}
				{#if articles.length > 0}
					<section id={`section-${sec.name}`} class="section-block" style={`--section-tint: ${sectionTint(sec.name)};`}>
						<SectionHeader label={sec.hudLabel} count={articles.length} tint={sectionTint(sec.name)} />
						<div class={`section-grid ${articles.length > 1 ? 'has-side' : ''}`}>
							<div>
								<FeaturedCard article={articles[0]} sectionTint={sectionTint(sec.name)} delay={sectionIndex * 120 + 200} />
								<div class="feedback-row mt-2">
									<button
										type="button"
										class={`feedback-btn ${articles[0].feedback.liked ? 'liked' : ''}`}
										onclick={() => onFeedback(articles[0], 'like')}
									>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 10v12"/><path d="M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2h0a3.13 3.13 0 0 1 3 3.88Z"/></svg>
										{articles[0].feedback.likes}
									</button>
									<button
										type="button"
										class={`feedback-btn ${articles[0].feedback.disliked ? 'disliked' : ''}`}
										onclick={() => onFeedback(articles[0], 'dislike')}
									>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 14V2"/><path d="M9 18.12 10 14H4.17a2 2 0 0 1-1.92-2.56l2.33-8A2 2 0 0 1 6.5 2H20a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2.76a2 2 0 0 0-1.79 1.11L12 22h0a3.13 3.13 0 0 1-3-3.88Z"/></svg>
										{articles[0].feedback.dislikes}
									</button>
									<button
										type="button"
										class={`feedback-btn ${articles[0].feedback.saved ? 'saved' : ''}`}
										onclick={() => onFeedback(articles[0], 'save')}
									>
										<svg width="14" height="14" viewBox="0 0 24 24" fill={articles[0].feedback.saved ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z"/></svg>
										{articles[0].feedback.saves}
									</button>
								</div>
							</div>

							{#if articles.length > 1}
								<div class="section-grid__side">
									{#each articles.slice(1) as article, idx (article.id)}
										<div>
											<SignalCard article={article} sectionTint={sectionTint(sec.name)} index={idx} />
											<div class="feedback-row mt-2">
												<button
													type="button"
													class={`feedback-btn ${article.feedback.liked ? 'liked' : ''}`}
													onclick={() => onFeedback(article, 'like')}
												>
													<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 10v12"/><path d="M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2h0a3.13 3.13 0 0 1 3 3.88Z"/></svg>
													{article.feedback.likes}
												</button>
												<button
													type="button"
													class={`feedback-btn ${article.feedback.disliked ? 'disliked' : ''}`}
													onclick={() => onFeedback(article, 'dislike')}
												>
													<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 14V2"/><path d="M9 18.12 10 14H4.17a2 2 0 0 1-1.92-2.56l2.33-8A2 2 0 0 1 6.5 2H20a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2.76a2 2 0 0 0-1.79 1.11L12 22h0a3.13 3.13 0 0 1-3-3.88Z"/></svg>
													{article.feedback.dislikes}
												</button>
												<button
													type="button"
													class={`feedback-btn ${article.feedback.saved ? 'saved' : ''}`}
													onclick={() => onFeedback(article, 'save')}
												>
													<svg width="14" height="14" viewBox="0 0 24 24" fill={article.feedback.saved ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z"/></svg>
													{article.feedback.saves}
												</button>
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					</section>
				{/if}
			{/each}
		</div>
	{/if}
</section>
