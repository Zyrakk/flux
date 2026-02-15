<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
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
		if (selectedSections.length > 0) {
			params.set('sections', selectedSections.join(','));
		}
		if (sourceRef) {
			params.set('source_ref', sourceRef);
		}
		if (status) {
			params.set('status', status);
		}
		if (fromDate) {
			params.set('from', fromDate);
		}
		if (toDate) {
			params.set('to', toDate);
		}
		if (likedOnly) {
			params.set('liked_only', 'true');
		}
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
		if (loadingMore || !hasMore) {
			return;
		}
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
		if (score == null) {
			return '0%';
		}
		const normalized = Math.max(0, Math.min(1, score));
		return `${Math.round(normalized * 100)}%`;
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

<section class="space-y-4">
	<div class="surface p-4">
		<h1 class="text-xl font-semibold">Feed Completo</h1>
		<p class="mt-1 text-sm text-text-2">Todos los artículos ingestados, con filtros y feedback.</p>

		<div class="mt-4 space-y-4">
			<div>
				<div class="mb-2 flex items-center justify-between">
					<span class="text-sm font-medium text-text-1">Secciones</span>
					<button class="btn-secondary !px-2 !py-1 text-xs" type="button" on:click={clearSectionFilter}>Todas</button>
				</div>
				<div class="flex flex-wrap gap-2">
					{#each sections as sec}
						<label class="badge cursor-pointer border border-slate-700 bg-slate-900/60 text-text-1">
							<input
								type="checkbox"
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
					<label class="mb-1 block text-xs text-text-2" for="source">Fuente</label>
					<select
						id="source"
						class="input w-full"
						bind:value={sourceRef}
						on:change={() => void resetAndLoad()}
					>
						<option value="">Todas</option>
						{#each sources.filter((s) => s.enabled) as source}
							<option value={source.id}>{source.name}</option>
						{/each}
					</select>
				</div>

				<div>
					<label class="mb-1 block text-xs text-text-2" for="status">Status</label>
					<select
						id="status"
						class="input w-full"
						bind:value={status}
						on:change={() => void resetAndLoad()}
					>
						<option value="">Todos</option>
						<option value="pending">pending</option>
						<option value="processed">processed</option>
						<option value="briefed">briefed</option>
						<option value="archived">archived</option>
					</select>
				</div>

				<div>
					<label class="mb-1 block text-xs text-text-2" for="from">Desde</label>
					<input id="from" class="input w-full" type="date" bind:value={fromDate} on:change={() => void resetAndLoad()} />
				</div>

				<div>
					<label class="mb-1 block text-xs text-text-2" for="to">Hasta</label>
					<input id="to" class="input w-full" type="date" bind:value={toDate} on:change={() => void resetAndLoad()} />
				</div>
			</div>

			<label class="inline-flex cursor-pointer items-center gap-2 text-sm text-text-1">
				<input type="checkbox" bind:checked={likedOnly} on:change={() => void resetAndLoad()} />
				Solo liked
			</label>
		</div>
	</div>

	{#if error}
		<div class="surface border-red-500/40 bg-red-500/10 p-3 text-sm text-red-100">{error}</div>
	{/if}

	<div class="space-y-3">
		{#if loading}
			<div class="surface p-4 text-sm text-text-1">Cargando...</div>
		{:else if articles.length === 0}
			<div class="surface p-4 text-sm text-text-1">No hay artículos para estos filtros.</div>
		{:else}
			{#each articles as article (article.id)}
				{@const source = sourceBadge(article.source_type)}
				<div class="surface p-4">
					<div class="flex flex-wrap items-center gap-2 text-xs">
						<span class="badge {source.className}">{source.icon} {source.label}</span>
						<span class="badge {sectionColor(article.section?.name)}">{article.section?.display_name ?? 'Sin sección'}</span>
						<span class="text-text-2">{formatRelativeTime(article.published_at ?? article.ingested_at)}</span>
						<span class="text-text-2">{formatDateTime(article.ingested_at)}</span>
					</div>

					<h2 class="mt-2 text-base font-semibold">
						<a href={article.url} target="_blank" rel="noreferrer">{article.title}</a>
					</h2>

					{#if article.summary}
						<p class="mt-2 text-sm text-text-1">{article.summary}</p>
					{/if}

					<div class="mt-3">
						<div class="mb-1 flex items-center justify-between text-xs text-text-2">
							<span>Relevance score</span>
							<span class="font-mono">{article.relevance_score?.toFixed(3) ?? 'N/A'}</span>
						</div>
						<div class="h-2 w-full rounded-full bg-slate-800">
							<div class="h-2 rounded-full bg-orange-400" style={`width: ${relevanceWidth(article.relevance_score)}`}></div>
						</div>
					</div>

					<div class="mt-3 flex flex-wrap gap-2">
						<button
							type="button"
							class="btn-secondary !py-1.5 !text-xs {article.feedback.liked ? '!border-emerald-400 !text-emerald-200' : ''}"
							on:click={() => onFeedback(article, 'like')}
						>
							👍 Like ({article.feedback.likes})
						</button>
						<button
							type="button"
							class="btn-secondary !py-1.5 !text-xs {article.feedback.disliked ? '!border-red-400 !text-red-200' : ''}"
							on:click={() => onFeedback(article, 'dislike')}
						>
							👎 Dislike ({article.feedback.dislikes})
						</button>
						<button
							type="button"
							class="btn-secondary !py-1.5 !text-xs {article.feedback.saved ? '!border-indigo-400 !text-indigo-200' : ''}"
							on:click={() => onFeedback(article, 'save')}
						>
							🔖 Guardar ({article.feedback.saves})
						</button>
					</div>
				</div>
			{/each}
		{/if}
	</div>

	<div bind:this={sentinel} class="h-6"></div>
	{#if loadingMore}
		<div class="text-center text-sm text-text-2">Cargando más...</div>
	{:else if !hasMore && articles.length > 0}
		<div class="text-center text-sm text-text-2">Fin del feed.</div>
	{/if}
</section>
