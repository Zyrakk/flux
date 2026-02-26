<script lang="ts">
	import { goto } from '$app/navigation';
	import { marked } from 'marked';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { apiFetch, apiJSON } from '$lib/api';
	import { formatDateTime, formatRelativeTime, isSameCalendarDay, sectionColor, sourceBadge } from '$lib/format';
	import type { Article, Briefing, FeedbackAction } from '$lib/types';

	const sections = [
		{ name: 'cybersecurity', displayName: 'Cybersecurity' },
		{ name: 'tech', displayName: 'Tech' },
		{ name: 'economy', displayName: 'Economy' },
		{ name: 'world', displayName: 'World' }
	];

	let briefing: Briefing | null = null;
	let markdownHtml = '';
	let loading = true;
	let error = '';
	let noBriefingToday = false;
	let activeSection = 'cybersecurity';

	$: grouped = groupArticlesBySection(briefing?.articles ?? []);

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
				return;
			}
			briefing = payload;
			markdownHtml = String(marked.parse(payload.content || ''));
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
				return;
			}
			error = message;
		} finally {
			loading = false;
		}
	}

	function groupArticlesBySection(articles: Article[]): Record<string, Article[]> {
		const out: Record<string, Article[]> = {
			cybersecurity: [],
			tech: [],
			economy: [],
			world: []
		};
		for (const article of articles) {
			const key = article.section?.name ?? 'tech';
			if (!out[key]) {
				out[key] = [];
			}
			out[key].push(article);
		}
		return out;
	}

	function sectionStat(sectionName: string): string {
		if (!briefing) return '0 -> 0';
		const total = briefing.metadata?.sections?.[sectionName]?.total ?? 0;
		const inBriefing = grouped[sectionName]?.length ?? 0;
		return `${total} -> ${inBriefing}`;
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

	onMount(loadLatestBriefing);

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
</script>

{#if loading}
	<div class="panel surface-pad text-center">
		<div class="loading-pulse text-sm text-slate-500">Generando vista del briefing...</div>
	</div>
{:else if error}
	<div class="alert error">{error}</div>
{:else if noBriefingToday}
	<div class="briefing-hero animate-fade-up text-center">
		<div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-sky-300/40 bg-white/70 text-2xl text-sky-700">
			◆
		</div>
		<h1 class="text-xl font-extrabold tracking-tight text-slate-900">No hay briefing disponible todavía</h1>
		<p class="mx-auto mt-2 max-w-xl text-sm text-slate-600">
			El próximo briefing diario se genera a las 03:00 UTC.
		</p>
		<div class="mt-6 flex flex-wrap items-center justify-center gap-2">
			<a class="btn-ghost" href="/feed">Abrir feed</a>
			<a class="btn-ghost" href="/admin/sources">Gestionar fuentes</a>
		</div>
	</div>
{:else if briefing}
	<section class="briefing-page animate-fade-up">
		<div class="briefing-hero">
			<div class="briefing-hero__header">
				<div>
					<h1 class="briefing-hero__title">Briefing Ejecutivo Diario</h1>
					<div class="briefing-hero__meta">
						<span>{formatDateTime(briefing.generated_at)}</span>
						<span>•</span>
						<span class="inline-flex items-center gap-2">
							<span class="status-dot {briefing.metadata?.partial ? 'warning' : 'ok'}"></span>
							{briefing.metadata?.partial ? 'Parcial' : 'Completo'}
						</span>
						<span>•</span>
						<span>{briefing.articles.length} noticias seleccionadas</span>
					</div>
				</div>

				<div class="briefing-stats">
					{#each sections as sec}
						<div class="stat-chip">
							<div class="stat-chip__label">{sec.displayName}</div>
							<div class="stat-chip__value">{sectionStat(sec.name)}</div>
						</div>
					{/each}
				</div>
			</div>

			<div class="briefing-markdown panel-subtle prose-flux surface-pad">
				{@html markdownHtml}
			</div>
		</div>

		<div class="section-strip no-scrollbar">
			{#each sections as sec}
				<button
					type="button"
					on:click={() => (activeSection = sec.name)}
					class="tab-pill {activeSection === sec.name ? 'active' : ''}"
				>
					{sec.displayName}
					<span class="font-mono text-[11px] text-slate-500">{grouped[sec.name]?.length ?? 0}</span>
				</button>
			{/each}
		</div>

		{#key activeSection}
			{#if (grouped[activeSection] ?? []).length === 0}
				<div class="empty-state" in:fade={{ duration: 220 }} out:fade={{ duration: 160 }}>
					No hay noticias en esta sección para este briefing.
				</div>
			{:else}
				<div class="news-grid" in:fade={{ duration: 220 }} out:fade={{ duration: 160 }}>
					{#each grouped[activeSection] as article, idx (article.id)}
						{@const source = sourceBadge(article.source_type)}
						<article class="news-card" in:fly={{ y: 16, duration: 360, delay: idx * 42 }}>
							<div class="news-card__top">
								<span class="badge {source.className}">{source.icon} {source.label}</span>
								<span class="badge {sectionColor(article.section?.name)}">
									{article.section?.display_name ?? 'Sin sección'}
								</span>
								<span class="text-xs font-semibold text-slate-500">
									{formatRelativeTime(article.published_at ?? article.ingested_at)}
								</span>
							</div>

							<h2 class="news-card__title">
								<a href={article.url} target="_blank" rel="noreferrer">{article.title}</a>
							</h2>

							{#if article.summary}
								<p class="news-card__summary">{article.summary}</p>
							{/if}

							<div class="news-card__footer">
								<div class="news-card__relevance">
									<div class="mb-1 flex items-center justify-between text-[11px] font-semibold text-slate-500">
										<span>Relevancia</span>
										<span class="font-mono">{article.relevance_score?.toFixed(3) ?? '—'}</span>
									</div>
									<div class="relevance-track">
										<div
											class="relevance-fill"
											style="width: {Math.round(Math.max(0, Math.min(1, article.relevance_score ?? 0)) * 100)}%;"
										></div>
									</div>
								</div>

								<div class="feedback-row">
									<button
										type="button"
										class="feedback-btn {article.feedback.liked ? 'liked' : ''}"
										on:click={() => onFeedback(article, 'like')}
									>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 10v12"/><path d="M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2h0a3.13 3.13 0 0 1 3 3.88Z"/></svg>
										{article.feedback.likes}
									</button>
									<button
										type="button"
										class="feedback-btn {article.feedback.disliked ? 'disliked' : ''}"
										on:click={() => onFeedback(article, 'dislike')}
									>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 14V2"/><path d="M9 18.12 10 14H4.17a2 2 0 0 1-1.92-2.56l2.33-8A2 2 0 0 1 6.5 2H20a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2.76a2 2 0 0 0-1.79 1.11L12 22h0a3.13 3.13 0 0 1-3-3.88Z"/></svg>
										{article.feedback.dislikes}
									</button>
									<button
										type="button"
										class="feedback-btn {article.feedback.saved ? 'saved' : ''}"
										on:click={() => onFeedback(article, 'save')}
									>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="{article.feedback.saved ? 'currentColor' : 'none'}" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z"/></svg>
										{article.feedback.saves}
									</button>
								</div>
							</div>
						</article>
					{/each}
				</div>
			{/if}
		{/key}
	</section>
{/if}
