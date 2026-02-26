<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import { fly, slide } from 'svelte/transition';
	import { apiFetch, apiJSON } from '$lib/api';
	import { formatDateTime, formatRelativeTime, sectionColor, sourceBadge } from '$lib/format';
	import type { Article, FeedbackAction, Section, Source } from '$lib/types';

	let articles: Article[] = [];
	let sections: Section[] = [];
	let sources: Source[] = [];

	let loading = true;
	let loadingMore = false;
	let hasMore = true;
	let error = '';
	let page = 1;
	const perPage = 30;

	let selectedSections: string[] = [];
	let sourceRef = '';
	let status = '';
	let fromDate = '';
	let toDate = '';
	let likedOnly = false;

	let filtersOpen = false;
	let sentinel: HTMLDivElement | null = null;
	let observer: IntersectionObserver | null = null;

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

	$: if (sentinel) {
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
	}

	onDestroy(() => {
		observer?.disconnect();
	});

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

	$: activeFilterCount =
		selectedSections.length +
		(sourceRef ? 1 : 0) +
		(status ? 1 : 0) +
		(fromDate ? 1 : 0) +
		(toDate ? 1 : 0) +
		(likedOnly ? 1 : 0);
</script>

<section class="briefing-page animate-fade-up">
	<div class="panel surface-pad">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div>
				<h1 class="text-xl font-extrabold tracking-tight text-slate-900">Feed de Noticias</h1>
				<p class="mt-1 text-sm text-slate-600">Explora todas las noticias ingestadas con filtros avanzados.</p>
			</div>

			<button class="btn-ghost" on:click={() => (filtersOpen = !filtersOpen)}>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/></svg>
				Filtros
				{#if activeFilterCount > 0}
					<span class="ml-1 inline-flex h-5 min-w-[20px] items-center justify-center rounded-full bg-sky-500 px-1 text-[11px] font-bold text-white">{activeFilterCount}</span>
				{/if}
			</button>
		</div>

		{#if filtersOpen}
			<div class="mt-4 rounded-2xl border border-slate-200/80 bg-white/60 p-4" transition:slide={{ duration: 220 }}>
				<div class="mb-4">
					<div class="mb-2 flex items-center justify-between">
						<span class="text-xs font-bold uppercase tracking-wide text-slate-500">Secciones</span>
						<button class="text-xs font-semibold text-slate-500 hover:text-slate-700" on:click={clearSectionFilter}>Todas</button>
					</div>
					<div class="flex flex-wrap gap-1.5">
						{#each sections as sec}
							<label class="tab-pill cursor-pointer {selectedSections.includes(sec.name) ? 'active' : ''}">
								<input
									type="checkbox"
									class="sr-only"
									checked={selectedSections.includes(sec.name)}
									on:change={() => toggleSection(sec.name)}
								/>
								{sec.display_name}
							</label>
						{/each}
					</div>
				</div>

				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
					<div>
						<label for="feed-filter-source" class="mb-1 block text-xs font-bold uppercase tracking-wide text-slate-500">Fuente</label>
						<select id="feed-filter-source" class="input" bind:value={sourceRef} on:change={() => void resetAndLoad()}>
							<option value="">Todas</option>
							{#each sources as src}
								<option value={src.id}>{src.name}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="feed-filter-status" class="mb-1 block text-xs font-bold uppercase tracking-wide text-slate-500">Estado</label>
						<select id="feed-filter-status" class="input" bind:value={status} on:change={() => void resetAndLoad()}>
							<option value="">Todos</option>
							<option value="pending">Pending</option>
							<option value="processed">Processed</option>
							<option value="briefed">Briefed</option>
							<option value="archived">Archived</option>
						</select>
					</div>
					<div>
						<label for="feed-filter-from" class="mb-1 block text-xs font-bold uppercase tracking-wide text-slate-500">Desde</label>
						<input id="feed-filter-from" type="date" class="input" bind:value={fromDate} on:change={() => void resetAndLoad()} />
					</div>
					<div>
						<label for="feed-filter-to" class="mb-1 block text-xs font-bold uppercase tracking-wide text-slate-500">Hasta</label>
						<input id="feed-filter-to" type="date" class="input" bind:value={toDate} on:change={() => void resetAndLoad()} />
					</div>
				</div>

				<label class="mt-4 inline-flex cursor-pointer items-center gap-2 text-sm font-semibold text-slate-600">
					<input type="checkbox" bind:checked={likedOnly} on:change={() => void resetAndLoad()} />
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
			<span class="loading-pulse text-sm text-slate-500">Cargando artículos...</span>
		</div>
	{:else if articles.length === 0}
		<div class="empty-state">No se encontraron artículos con estos filtros.</div>
	{:else}
		<div class="news-grid">
			{#each articles as article (article.id)}
				{@const source = sourceBadge(article.source_type)}
				<article class="news-card" in:fly={{ y: 12, duration: 320 }}>
					<div class="news-card__top">
						<span class="badge {source.className}">{source.icon} {source.label}</span>
						<span class="badge {sectionColor(article.section?.name)}">{article.section?.display_name ?? 'Sin sección'}</span>
						<span class="text-xs font-semibold text-slate-500">
							{formatRelativeTime(article.published_at ?? article.ingested_at)}
						</span>
						<span class="text-xs font-medium text-slate-400">{formatDateTime(article.ingested_at)}</span>
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
								<div class="relevance-fill" style="width: {relevanceWidth(article.relevance_score)}"></div>
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

	<div bind:this={sentinel} class="h-6"></div>
	{#if loadingMore}
		<div class="py-3 text-center">
			<span class="loading-pulse text-xs font-semibold uppercase tracking-wide text-slate-500">Cargando más...</span>
		</div>
	{:else if !hasMore && articles.length > 0}
		<div class="py-2 text-center text-xs font-semibold uppercase tracking-wide text-slate-500">
			Fin del feed · {articles.length} artículos
		</div>
	{/if}
</section>
