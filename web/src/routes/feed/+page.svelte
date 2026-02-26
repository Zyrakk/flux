<svelte:options runes={true} />
<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import { apiFetch, apiJSON } from '$lib/api';
	import { formatDateTime, formatRelativeTime, priorityLabel, sectionColor, sectionTint, sourceBadge } from '$lib/format';
	import type { Article, FeedbackAction, Section, Source } from '$lib/types';

	let articles = $state<Article[]>([]);
	let sections = $state<Section[]>([]);
	let sources = $state<Source[]>([]);

	let loading = $state(true);
	let loadingMore = $state(false);
	let hasMore = $state(true);
	let error = $state('');
	let page = $state(1);
	const perPage = 30;

	let selectedSections = $state<string[]>([]);
	let sourceRef = $state('');
	let status = $state('');
	let fromDate = $state('');
	let toDate = $state('');
	let likedOnly = $state(false);

	let filtersOpen = $state(false);
	let sentinel: HTMLDivElement | null = null;
	let observer: IntersectionObserver | null = null;

	const activeFilterCount = $derived(
		selectedSections.length + (sourceRef ? 1 : 0) + (status ? 1 : 0) + (fromDate ? 1 : 0) + (toDate ? 1 : 0) + (likedOnly ? 1 : 0)
	);

	$effect(() => {
		if (!sentinel) return;
		observer?.disconnect();
		observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					void loadMore();
				}
			},
			{ rootMargin: '300px' }
		);
		observer.observe(sentinel);
		return () => observer?.disconnect();
	});

	onMount(async () => {
		try {
			const [loadedSections, loadedSources] = await Promise.all([
				apiJSON<Section[]>('/sections'),
				apiJSON<Source[]>('/sources')
			]);
			sections = loadedSections;
			sources = loadedSources;
			await resetAndLoad();
		} catch (err) {
			await handleError(err);
		} finally {
			loading = false;
		}
	});

	onDestroy(() => {
		observer?.disconnect();
	});

	function sectionBadgePriority(article: Article): string {
		const fromMeta = article.metadata && typeof article.metadata.priority === 'string' ? article.metadata.priority : undefined;
		return (fromMeta ?? priorityLabel(article.relevance_score)).toUpperCase();
	}

	function priorityClass(priority: string): string {
		switch (priority) {
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

	function buildQuery(nextPage: number): string {
		const params = new URLSearchParams();
		params.set('page', String(nextPage));
		params.set('per_page', String(perPage));
		if (selectedSections.length > 0) params.set('sections', selectedSections.join(','));
		if (sourceRef) params.set('source_ref', sourceRef);
		if (status) params.set('status', status);
		if (fromDate) params.set('from', fromDate);
		if (toDate) params.set('to', toDate);
		if (likedOnly) params.set('liked_only', 'true');
		return params.toString();
	}

	async function resetAndLoad() {
		articles = [];
		page = 1;
		hasMore = true;
		error = '';
		await loadMore();
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const query = buildQuery(page);
			const payload = await apiJSON<{ articles: Article[]; total_pages: number }>(`/articles?${query}`);
			articles = [...articles, ...payload.articles];
			hasMore = page < payload.total_pages;
			page += 1;
		} catch (err) {
			await handleError(err);
		} finally {
			loadingMore = false;
		}
	}

	function toggleSection(name: string) {
		if (selectedSections.includes(name)) {
			selectedSections = selectedSections.filter((item) => item !== name);
		} else {
			selectedSections = [...selectedSections, name];
		}
		void resetAndLoad();
	}

	function clearSectionFilter() {
		selectedSections = [];
		void resetAndLoad();
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
				articles = articles;
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
			articles = articles;
		} catch (err) {
			await handleError(err);
		}
	}

	async function handleError(err: unknown) {
		const message = err instanceof Error ? err.message : 'Error inesperado';
		if (message.includes('UNAUTHORIZED')) {
			await goto('/login');
			return;
		}
		error = message;
	}

	function relevanceWidth(score?: number): string {
		if (score == null) return '0%';
		return `${Math.round(Math.max(0, Math.min(1, score)) * 100)}%`;
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
</script>

<section class="briefing-page">
	<div class="panel surface-pad">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div>
				<h1 class="text-xl font-extrabold tracking-tight text-[rgba(255,255,255,0.92)]">Feed de Noticias</h1>
				<p class="mt-1 text-sm text-[rgba(255,255,255,0.48)]">Explora todas las señales ingestadas con filtros HUD.</p>
			</div>

			<button class="btn-ghost" onclick={() => (filtersOpen = !filtersOpen)}>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/></svg>
				Filtros
				{#if activeFilterCount > 0}
					<span class="ml-1 inline-flex h-5 min-w-[20px] items-center justify-center rounded-full bg-[rgba(6,182,212,0.2)] px-1 text-[11px] font-bold text-[var(--flux-accent)]">{activeFilterCount}</span>
				{/if}
			</button>
		</div>

		{#if filtersOpen}
			<div class="mt-4 rounded-2xl border border-[rgba(255,255,255,0.07)] bg-[rgba(255,255,255,0.015)] p-4">
				<div class="mb-4">
					<div class="mb-2 flex items-center justify-between">
						<span class="font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-[rgba(255,255,255,0.34)]">Secciones</span>
						<button class="text-xs font-semibold text-[rgba(255,255,255,0.5)] hover:text-[rgba(255,255,255,0.75)]" onclick={clearSectionFilter}>Todas</button>
					</div>
					<div class="flex flex-wrap gap-1.5">
						{#each sections as sec}
							<label
								class={`section-tab ${selectedSections.includes(sec.name) ? 'active' : ''}`}
								style={`--section-tint: ${sectionTint(sec.name)}; color: ${selectedSections.includes(sec.name) ? sectionColor(sec.name) : ''};`}
							>
								<input
									type="checkbox"
									class="hud-checkbox"
									style={`--section-tint: ${sectionTint(sec.name)};`}
									checked={selectedSections.includes(sec.name)}
									onchange={() => toggleSection(sec.name)}
								/>
								{sec.display_name}
							</label>
						{/each}
					</div>
				</div>

				<div class="filter-grid">
					<div>
						<label for="feed-filter-source" class="mb-1 block font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-[rgba(255,255,255,0.34)]">Fuente</label>
						<select id="feed-filter-source" class="input" bind:value={sourceRef} onchange={() => void resetAndLoad()}>
							<option value="">Todas</option>
							{#each sources as src}
								<option value={src.id}>{src.name}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="feed-filter-status" class="mb-1 block font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-[rgba(255,255,255,0.34)]">Estado</label>
						<select id="feed-filter-status" class="input" bind:value={status} onchange={() => void resetAndLoad()}>
							<option value="">Todos</option>
							<option value="pending">Pending</option>
							<option value="processed">Processed</option>
							<option value="briefed">Briefed</option>
							<option value="archived">Archived</option>
						</select>
					</div>
					<div>
						<label for="feed-filter-from" class="mb-1 block font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-[rgba(255,255,255,0.34)]">Desde</label>
						<input id="feed-filter-from" type="date" class="input" bind:value={fromDate} onchange={() => void resetAndLoad()} />
					</div>
					<div>
						<label for="feed-filter-to" class="mb-1 block font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-[rgba(255,255,255,0.34)]">Hasta</label>
						<input id="feed-filter-to" type="date" class="input" bind:value={toDate} onchange={() => void resetAndLoad()} />
					</div>
				</div>

				<label class="mt-4 inline-flex cursor-pointer items-center gap-2 text-sm font-semibold text-[rgba(255,255,255,0.55)]">
					<input type="checkbox" class="hud-checkbox" bind:checked={likedOnly} onchange={() => void resetAndLoad()} />
					Solo artículos con like
				</label>
			</div>
		{/if}
	</div>

	{#if error}
		<div class="alert error">{error}</div>
	{/if}

	{#if loading}
		<div class="panel surface-pad text-center">
			<span class="loading-pulse text-sm text-[rgba(255,255,255,0.45)]">Cargando artículos...</span>
		</div>
	{:else if articles.length === 0}
		<div class="empty-state">No se encontraron artículos con estos filtros.</div>
	{:else}
		<div class="grid gap-3 md:grid-cols-2">
			{#each articles as article, idx (article.id)}
				{@const source = sourceBadge(article.source_type)}
				{@const priority = sectionBadgePriority(article)}
				<article
					class="signal-card reveal reveal--x is-visible"
					style={`--section-tint: ${sectionTint(article.section?.name)}; transition-delay: ${Math.min(idx * 18, 220)}ms;`}
				>
					<div class="signal-card__meta">
						<span class={source.className}>{source.icon} {source.label}</span>
						<span class={`priority-badge ${priorityClass(priority)}`}>{priority}</span>
						<span class="signal-card__time">{formatRelativeTime(article.published_at ?? article.ingested_at)}</span>
						<span class="flex-1"></span>
						<span class="signal-card__source">{article.section?.display_name ?? 'Sin sección'}</span>
					</div>

					<h2 class="signal-card__title">
						<a href={article.url} target="_blank" rel="noreferrer">{article.title}</a>
					</h2>
					<p class="signal-card__summary">{article.summary || 'Sin resumen disponible para esta señal.'}</p>

					<div class="mt-3 flex items-center justify-between gap-3">
						<div class="min-w-[130px] flex-1">
							<div class="mb-1 flex items-center justify-between text-[11px] text-[rgba(255,255,255,0.38)]">
								<span>Relevancia</span>
								<span class="font-mono">{article.relevance_score?.toFixed(3) ?? '—'}</span>
							</div>
							<div class="h-1.5 rounded-full bg-[rgba(255,255,255,0.07)]">
								<div class="h-full rounded-full bg-[rgba(6,182,212,0.65)]" style={`width: ${relevanceWidth(article.relevance_score)};`}></div>
							</div>
						</div>
						<span class="text-[11px] text-[rgba(255,255,255,0.3)]">{formatDateTime(article.ingested_at)}</span>
					</div>

					<div class="feedback-row">
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
				</article>
			{/each}
		</div>
	{/if}

	<div bind:this={sentinel} class="h-6"></div>
	{#if loadingMore}
		<div class="py-3 text-center">
			<span class="loading-pulse font-mono text-xs uppercase tracking-[0.12em] text-[rgba(255,255,255,0.4)]">Cargando más...</span>
		</div>
	{:else if !hasMore && articles.length > 0}
		<div class="py-2 text-center font-mono text-xs uppercase tracking-[0.12em] text-[rgba(255,255,255,0.35)]">
			Fin del feed · {articles.length} artículos
		</div>
	{/if}
</section>
