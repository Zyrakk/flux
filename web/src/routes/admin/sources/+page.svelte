<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
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
		if (!draggedSectionId || draggedSectionId === targetSectionID) {
			return;
		}

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

	function sourceState(source: Source): { icon: string; className: string } {
		if (!source.enabled) {
			return { icon: '●', className: 'text-red-400' };
		}
		if (source.error_count > 0) {
			return { icon: '●', className: 'text-amber-400' };
		}
		return { icon: '●', className: 'text-emerald-400' };
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

<section class="space-y-4">
	<div class="surface p-4">
		<h1 class="text-xl font-semibold">Admin · Fuentes y Secciones</h1>
		<p class="mt-1 text-sm text-text-2">Configura fuentes RSS/HN/Reddit, secciones del briefing y orden de aparición.</p>
	</div>

	{#if error}
		<div class="surface border-red-500/40 bg-red-500/10 p-3 text-sm text-red-100">{error}</div>
	{/if}

	<div class="surface overflow-x-auto p-4">
		<h2 class="mb-3 text-lg font-semibold">Fuentes</h2>
		{#if loading}
			<div class="text-sm text-text-2">Cargando fuentes...</div>
		{:else}
			<table class="min-w-full text-left text-sm">
				<thead class="text-text-2">
					<tr>
						<th class="pb-2">Estado</th>
						<th class="pb-2">Nombre</th>
						<th class="pb-2">Tipo</th>
						<th class="pb-2">Secciones</th>
						<th class="pb-2">Último fetch</th>
						<th class="pb-2">Errores</th>
						<th class="pb-2">Stats</th>
						<th class="pb-2">Acción</th>
					</tr>
				</thead>
				<tbody>
					{#each sources as source (source.id)}
						{@const state = sourceState(source)}
						<tr class="border-t border-slate-800 align-top">
							<td class="py-2"><span class={state.className}>{state.icon}</span></td>
							<td class="py-2">
								<div class="font-medium">{source.name}</div>
								{#if source.last_error}
									<div class="max-w-xs truncate text-xs text-text-2" title={source.last_error}>{source.last_error}</div>
								{/if}
							</td>
							<td class="py-2 uppercase text-text-1">{source.source_type}</td>
							<td class="py-2 text-text-1">{source.sections.map((s) => s.display_name).join(', ') || '—'}</td>
							<td class="py-2 text-text-1">{formatRelativeTime(source.last_fetched_at)}</td>
							<td class="py-2 text-text-1">{source.error_count}</td>
							<td class="py-2 text-xs text-text-1">
								<div>Total: {source.stats.total_ingested}</div>
								<div>24h: {source.stats.last_24h}</div>
								<div>% filtro: {source.stats.pass_rate_pct.toFixed(2)}%</div>
							</td>
							<td class="py-2">
								<button class="btn-secondary !py-1.5 !text-xs" on:click={() => toggleSource(source)}>
									{source.enabled ? 'Deshabilitar' : 'Habilitar'}
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<div class="surface p-4">
			<h2 class="text-lg font-semibold">Añadir Fuente RSS</h2>
			<div class="mt-3 space-y-3">
				<input class="input w-full" placeholder="Nombre" bind:value={newSourceName} />
				<input class="input w-full" placeholder="URL RSS" bind:value={newSourceURL} />

				<div>
					<div class="mb-1 text-sm text-text-2">Secciones</div>
					<div class="flex flex-wrap gap-2">
						{#each sections as section}
							<label class="badge cursor-pointer border border-slate-700 bg-slate-900/60 text-text-1">
								<input
									type="checkbox"
									checked={newSourceSectionIDs.includes(section.id)}
									on:change={() => toggleNewSourceSection(section.id)}
								/>
								{section.display_name}
							</label>
						{/each}
					</div>
				</div>

				<button class="btn-primary" disabled={saving} on:click={createSource}>
					{saving ? 'Guardando...' : 'Añadir fuente'}
				</button>
			</div>
		</div>

		<div class="surface p-4">
			<h2 class="text-lg font-semibold">Crear Sección</h2>
			<div class="mt-3 space-y-3">
				<input class="input w-full" placeholder="name (slug)" bind:value={newSectionName} />
				<input class="input w-full" placeholder="display_name (ej: 🧪 AI)" bind:value={newSectionDisplayName} />
				<textarea
					class="input w-full"
					rows="3"
					placeholder="seed keywords separadas por coma"
					bind:value={newSectionKeywords}
				></textarea>
				<button class="btn-primary" on:click={createSection}>Crear sección</button>
			</div>
		</div>
	</div>

	<div class="surface p-4">
		<h2 class="text-lg font-semibold">Gestión de Secciones</h2>
		<p class="mt-1 text-sm text-text-2">Arrastra para reordenar, activa/desactiva y ajusta `max_briefing_articles`.</p>

		<div class="mt-3 space-y-2">
			{#each sections as section (section.id)}
				<div
					class="rounded-lg border border-slate-700 bg-slate-900/60 p-3"
					draggable="true"
					role="listitem"
					on:dragstart={() => onDragStart(section.id)}
					on:dragover|preventDefault
					on:drop={() => onDrop(section.id)}
				>
					<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
						<div>
							<div class="font-medium">{section.display_name}</div>
							<div class="text-xs text-text-2">name: {section.name} · orden: {section.sort_order}</div>
						</div>

						<div class="flex flex-wrap items-center gap-2">
							<label class="inline-flex items-center gap-2 text-xs text-text-1">
								<input type="checkbox" checked={section.enabled} on:change={() => toggleSectionEnabled(section)} />
								enabled
							</label>

							<input
								type="number"
								class="input !w-24"
								min="1"
								bind:value={section.max_briefing_articles}
							/>
							<button class="btn-secondary !py-1.5 !text-xs" on:click={() => saveSectionMax(section)}>
								Guardar max
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	</div>
</section>
