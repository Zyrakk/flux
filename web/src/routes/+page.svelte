<script lang="ts">
	import { goto } from '$app/navigation';
	import { marked } from 'marked';
	import { onMount } from 'svelte';
	import { apiFetch, apiJSON } from '$lib/api';
	import { formatDateTime, formatRelativeTime, isSameCalendarDay, sectionColor, sourceBadge } from '$lib/format';
	import type { Article, Briefing, FeedbackAction } from '$lib/types';

	const sections = [
		{ name: 'cybersecurity', displayName: '🔒 Cybersecurity' },
		{ name: 'tech', displayName: '💻 Tech' },
		{ name: 'economy', displayName: '📈 Economy' },
		{ name: 'world', displayName: '🌍 World' }
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
		if (!briefing) {
			return '0 → 0';
		}
		const total = briefing.metadata?.sections?.[sectionName]?.total ?? 0;
		const inBriefing = grouped[sectionName]?.length ?? 0;
		return `${total} → ${inBriefing}`;
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
	<div class="surface p-4 text-sm text-text-1">Cargando briefing...</div>
{:else if error}
	<div class="surface border-red-500/40 bg-red-500/10 p-4 text-sm text-red-100">{error}</div>
{:else if noBriefingToday}
	<div class="surface p-6">
		<h1 class="text-lg font-semibold">No hay briefing todavía.</h1>
		<p class="mt-2 text-sm text-text-1">El próximo se genera a las 03:00.</p>
		<div class="mt-4 flex gap-2">
			<a class="btn-secondary" href="/feed">Abrir feed completo</a>
			<a class="btn-secondary" href="/admin/sources">Revisar fuentes</a>
		</div>
	</div>
{:else if briefing}
	<section class="space-y-4">
		<div class="surface p-4">
			<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
				<div>
					<h1 class="text-2xl font-semibold">Flux — Briefing Diario</h1>
					<p class="mt-1 text-sm text-text-1">
						Generado: <span class="font-mono">{formatDateTime(briefing.generated_at)}</span>
						· Estado:
						<span class="font-semibold {briefing.metadata?.partial ? 'text-amber-300' : 'text-emerald-300'}">
							{briefing.metadata?.partial ? 'Parcial' : 'Completo'}
						</span>
					</p>
				</div>
				<div class="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
					{#each sections as sec}
						<div class="rounded-lg border border-slate-700 bg-slate-900/60 px-2 py-1">
							<div>{sec.displayName}</div>
							<div class="mt-0.5 font-mono text-text-1">{sectionStat(sec.name)}</div>
						</div>
					{/each}
				</div>
			</div>

			<div class="prose prose-invert prose-sm mt-4 max-w-none leading-relaxed">
				{@html markdownHtml}
			</div>
		</div>

		<div class="surface p-2">
			<div class="no-scrollbar flex gap-2 overflow-x-auto pb-1">
				{#each sections as sec}
					<button
						type="button"
						on:click={() => (activeSection = sec.name)}
						class="btn-secondary whitespace-nowrap !rounded-full !px-3 !py-1.5 text-xs {activeSection === sec.name
							? '!border-orange-400 !bg-orange-500/20 !text-orange-100'
							: ''}"
					>
						{sec.displayName}
						<span class="font-mono text-[11px] text-text-2">{grouped[sec.name]?.length ?? 0}</span>
					</button>
				{/each}
			</div>
		</div>

		<div class="space-y-3">
			{#if (grouped[activeSection] ?? []).length === 0}
				<div class="surface p-4 text-sm text-text-1">Sin artículos para esta sección.</div>
			{:else}
				{#each grouped[activeSection] as article (article.id)}
					{@const source = sourceBadge(article.source_type)}
					<div class="surface p-4">
						<div class="flex flex-wrap items-center gap-2">
							<span class="badge {source.className}">{source.icon} {source.label}</span>
							<span class="badge {sectionColor(article.section?.name)}">{article.section?.display_name ?? 'Sin sección'}</span>
							<span class="text-xs text-text-2">{formatRelativeTime(article.published_at ?? article.ingested_at)}</span>
						</div>

						<h2 class="mt-2 text-base font-semibold leading-snug">
							<a href={article.url} target="_blank" rel="noreferrer">{article.title}</a>
						</h2>

						{#if article.summary}
							<p class="mt-2 text-sm text-text-1">{article.summary}</p>
						{/if}

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
	</section>
{/if}
