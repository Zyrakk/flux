<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { apiJSON } from '$lib/api';
	import { formatRelativeTime } from '$lib/format';
	import type { Section, Source } from '$lib/types';

	let sources: Source[] = [];
	let sections: Section[] = [];
	let loading = true;
	let error = '';
	let saving = false;

	let draggedSectionId: string | null = null;

	let newSourceName = '';
	let newSourceURL = '';
	let newSourceSectionIDs: string[] = [];

	let newSectionName = '';
	let newSectionDisplayName = '';
	let newSectionKeywords = '';

	onMount(async () => {
		await loadAll();
		loading = false;
	});

	async function loadAll() {
		try {
			error = '';
			const [loadedSources, loadedSections] = await Promise.all([
				apiJSON<Source[]>('/sources'),
				apiJSON<Section[]>('/sections')
			]);
			sources = loadedSources;
			sections = [...loadedSections].sort((a, b) => a.sort_order - b.sort_order);
		} catch (err) {
			await handleError(err);
		}
	}

	async function toggleSource(source: Source) {
		try {
			await apiJSON(`/sources/${source.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled: !source.enabled })
			});
			source.enabled = !source.enabled;
			sources = sources;
		} catch (err) {
			await handleError(err);
		}
	}

	async function createSource() {
		if (!newSourceName.trim() || !newSourceURL.trim()) {
			error = 'Nombre y URL son obligatorios';
			return;
		}
		saving = true;
		try {
			await apiJSON('/sources/validate-rss', {
				method: 'POST',
				body: JSON.stringify({ url: newSourceURL.trim() })
			});

			await apiJSON('/sources', {
				method: 'POST',
				body: JSON.stringify({
					source_type: 'rss',
					name: newSourceName.trim(),
					config: { url: newSourceURL.trim() },
					section_ids: newSourceSectionIDs
				})
			});

			newSourceName = '';
			newSourceURL = '';
			newSourceSectionIDs = [];
			await loadAll();
		} catch (err) {
			await handleError(err);
		} finally {
			saving = false;
		}
	}

	function toggleNewSourceSection(sectionID: string) {
		if (newSourceSectionIDs.includes(sectionID)) {
			newSourceSectionIDs = newSourceSectionIDs.filter((id) => id !== sectionID);
		} else {
			newSourceSectionIDs = [...newSourceSectionIDs, sectionID];
		}
	}

	async function toggleSectionEnabled(section: Section) {
		try {
			await apiJSON(`/sections/${section.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled: !section.enabled })
			});
			section.enabled = !section.enabled;
			sections = sections;
		} catch (err) {
			await handleError(err);
		}
	}

	async function saveSectionMax(section: Section) {
		try {
			await apiJSON(`/sections/${section.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ max_briefing_articles: section.max_briefing_articles })
			});
		} catch (err) {
			await handleError(err);
		}
	}

	async function createSection() {
		if (!newSectionName.trim() || !newSectionDisplayName.trim()) {
			error = 'name y display_name son obligatorios';
			return;
		}

		const keywords = newSectionKeywords
			.split(',')
			.map((v) => v.trim())
			.filter(Boolean);

		try {
			await apiJSON('/sections', {
				method: 'POST',
				body: JSON.stringify({
					name: newSectionName.trim().toLowerCase(),
					display_name: newSectionDisplayName.trim(),
					seed_keywords: keywords,
					max_briefing_articles: 3
				})
			});
			newSectionName = '';
			newSectionDisplayName = '';
			newSectionKeywords = '';
			await loadAll();
		} catch (err) {
			await handleError(err);
		}
	}

	function onDragStart(sectionID: string) {
		draggedSectionId = sectionID;
	}

	async function onDrop(targetSectionID: string) {
		if (!draggedSectionId || draggedSectionId === targetSectionID) return;

		const next = [...sections];
		const from = next.findIndex((s) => s.id === draggedSectionId);
		const to = next.findIndex((s) => s.id === targetSectionID);
		if (from < 0 || to < 0) {
			draggedSectionId = null;
			return;
		}

		const [moved] = next.splice(from, 1);
		next.splice(to, 0, moved);
		next.forEach((section, idx) => {
			section.sort_order = idx + 1;
		});
		sections = next;
		draggedSectionId = null;

		try {
			await apiJSON('/sections/reorder', {
				method: 'POST',
				body: JSON.stringify({ section_ids: sections.map((section) => section.id) })
			});
		} catch (err) {
			await handleError(err);
		}
	}

	function sourceState(source: Source): { dot: string } {
		if (!source.enabled) return { dot: 'error' };
		if (source.error_count > 0) return { dot: 'warning' };
		return { dot: 'ok' };
	}

	async function handleError(err: unknown) {
		const message = err instanceof Error ? err.message : 'Error inesperado';
		if (message.includes('UNAUTHORIZED')) {
			await goto('/login');
			return;
		}
		error = message;
	}
</script>

<section class="briefing-page animate-fade-up">
	<div class="briefing-hero">
		<h1 class="briefing-hero__title">Panel de Administración</h1>
		<p class="mt-2 max-w-2xl text-sm text-slate-600">
			Gestiona fuentes RSS, secciones y reglas de briefing desde una vista centralizada.
		</p>
	</div>

	{#if error}
		<div class="alert error">{error}</div>
	{/if}

	<div class="panel overflow-hidden" in:fly={{ y: 12, duration: 300 }}>
		<div class="flex items-center justify-between px-5 pb-3 pt-5">
			<h2 class="text-base font-extrabold text-slate-900">Fuentes</h2>
			<span class="text-xs font-bold uppercase tracking-wide text-slate-500">{sources.length} configuradas</span>
		</div>

		{#if loading}
			<div class="px-5 pb-5 text-center">
				<span class="loading-pulse text-sm text-slate-500">Cargando fuentes...</span>
			</div>
		{:else}
			<div class="overflow-x-auto px-3 pb-3">
				<table class="glass-table">
					<thead>
						<tr>
							<th class="w-8"></th>
							<th>Nombre</th>
							<th>Tipo</th>
							<th class="hidden sm:table-cell">Secciones</th>
							<th class="hidden md:table-cell">Último fetch</th>
							<th class="hidden lg:table-cell">Stats</th>
							<th class="text-right">Acción</th>
						</tr>
					</thead>
					<tbody>
						{#each sources as source (source.id)}
							{@const state = sourceState(source)}
							<tr>
								<td><span class="status-dot {state.dot}"></span></td>
								<td>
									<div class="font-semibold text-slate-900">{source.name}</div>
									{#if source.last_error}
										<div class="mt-1 max-w-[240px] truncate text-[11px] text-slate-500" title={source.last_error}>
											{source.last_error}
										</div>
									{/if}
								</td>
								<td>
									<span class="badge">{source.source_type.toUpperCase()}</span>
								</td>
								<td class="hidden sm:table-cell text-xs">
									{source.sections.map((s) => s.display_name).join(', ') || '—'}
								</td>
								<td class="hidden md:table-cell text-xs text-slate-500">
									{formatRelativeTime(source.last_fetched_at)}
								</td>
								<td class="hidden lg:table-cell">
									<div class="text-[11px] leading-tight text-slate-500">
										<span>Total: {source.stats.total_ingested}</span>
										<span class="mx-1">•</span>
										<span>24h: {source.stats.last_24h}</span>
										<span class="mx-1">•</span>
										<span>{source.stats.pass_rate_pct.toFixed(1)}%</span>
									</div>
								</td>
								<td class="text-right">
									<button class="btn-ghost !px-3 !py-1.5 !text-[11px]" on:click={() => toggleSource(source)}>
										{source.enabled ? 'Deshabilitar' : 'Habilitar'}
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<div class="panel surface-pad" in:fly={{ y: 14, duration: 320, delay: 40 }}>
			<h2 class="text-base font-extrabold text-slate-900">Añadir Fuente RSS</h2>
			<div class="mt-4 space-y-3">
				<input class="input" placeholder="Nombre" bind:value={newSourceName} />
				<input class="input" placeholder="URL RSS" bind:value={newSourceURL} />

				<div>
					<div class="mb-2 text-xs font-bold uppercase tracking-wide text-slate-500">Secciones</div>
					<div class="flex flex-wrap gap-1.5">
						{#each sections as section}
							<label class="tab-pill cursor-pointer {newSourceSectionIDs.includes(section.id) ? 'active' : ''}">
								<input
									type="checkbox"
									class="sr-only"
									checked={newSourceSectionIDs.includes(section.id)}
									on:change={() => toggleNewSourceSection(section.id)}
								/>
								{section.display_name}
							</label>
						{/each}
					</div>
				</div>

				<button class="btn-primary w-full" disabled={saving} on:click={createSource}>
					{saving ? 'Validando...' : 'Añadir fuente'}
				</button>
			</div>
		</div>

		<div class="panel surface-pad" in:fly={{ y: 14, duration: 320, delay: 80 }}>
			<h2 class="text-base font-extrabold text-slate-900">Crear Sección</h2>
			<div class="mt-4 space-y-3">
				<input class="input" placeholder="name (slug)" bind:value={newSectionName} />
				<input class="input" placeholder="display_name (ej: Seguridad)" bind:value={newSectionDisplayName} />
				<textarea
					class="input"
					rows="3"
					placeholder="seed keywords separadas por coma"
					bind:value={newSectionKeywords}
				></textarea>
				<button class="btn-primary w-full" on:click={createSection}>Crear sección</button>
			</div>
		</div>
	</div>

	<div class="panel surface-pad" in:fly={{ y: 16, duration: 340, delay: 120 }}>
		<div class="flex flex-wrap items-end justify-between gap-3">
			<div>
				<h2 class="text-base font-extrabold text-slate-900">Gestión de Secciones</h2>
				<p class="mt-1 text-sm text-slate-600">
					Arrastra para reordenar, activa o desactiva y ajusta el máximo por briefing.
				</p>
			</div>
		</div>

		<div class="mt-4 space-y-2">
			{#each sections as section (section.id)}
				<div
					class="panel-subtle draggable-section rounded-2xl p-3 transition-all duration-200"
					draggable="true"
					role="listitem"
					on:dragstart={() => onDragStart(section.id)}
					on:dragover|preventDefault
					on:drop={() => onDrop(section.id)}
					in:fly={{ y: 8, duration: 220 }}
				>
					<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
						<div class="flex items-center gap-3">
							<span class="text-sm text-slate-400">⠿</span>
							<div>
								<div class="text-sm font-bold text-slate-900">{section.display_name}</div>
								<div class="text-xs text-slate-500">{section.name} · orden: {section.sort_order}</div>
							</div>
						</div>

						<div class="flex flex-wrap items-center gap-2.5">
							<label class="inline-flex cursor-pointer items-center gap-1.5 text-xs font-semibold text-slate-600">
								<input type="checkbox" checked={section.enabled} on:change={() => toggleSectionEnabled(section)} />
								Activa
							</label>

							<div class="flex items-center gap-1.5">
								<input
									type="number"
									class="input !w-20 !px-2 !py-1.5 text-center text-xs"
									min="1"
									bind:value={section.max_briefing_articles}
								/>
								<button class="btn-ghost !px-3 !py-1.5 !text-[11px]" on:click={() => saveSectionMax(section)}>
									Guardar
								</button>
							</div>
						</div>
					</div>
				</div>
			{/each}
		</div>
	</div>
</section>
