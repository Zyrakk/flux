<svelte:options runes={true} />
<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { apiJSON } from '$lib/api';
	import { formatRelativeTime, sectionTint } from '$lib/format';
	import type { Section, Source } from '$lib/types';

	let sources = $state<Source[]>([]);
	let sections = $state<Section[]>([]);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);

	let draggedSectionId = $state<string | null>(null);

	let newSourceName = $state('');
	let newSourceURL = $state('');
	let newSourceSectionIDs = $state<string[]>([]);

	let newSectionName = $state('');
	let newSectionDisplayName = $state('');
	let newSectionKeywords = $state('');

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

	function sourceState(source: Source): { dot: string; label: string } {
		if (!source.enabled) return { dot: 'error', label: 'Disabled' };
		if (source.error_count > 0) return { dot: 'warning', label: 'Warning' };
		return { dot: 'ok', label: 'Healthy' };
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

<section class="briefing-page">
	<div class="panel surface-pad">
		<h1 class="text-xl font-extrabold tracking-tight text-[rgba(255,255,255,0.92)]">Panel de Administración</h1>
		<p class="mt-2 max-w-2xl text-sm text-[rgba(255,255,255,0.5)]">
			Gestiona fuentes RSS, secciones y reglas de briefing desde una vista centralizada.
		</p>
	</div>

	{#if error}
		<div class="alert error">{error}</div>
	{/if}

	<div class="panel overflow-hidden">
		<div class="flex items-center justify-between px-5 pb-3 pt-5">
			<h2 class="text-base font-extrabold text-[rgba(255,255,255,0.9)]">Fuentes</h2>
			<span class="font-mono text-[10px] font-bold uppercase tracking-[0.14em] text-[rgba(255,255,255,0.35)]">{sources.length} configuradas</span>
		</div>

		{#if loading}
			<div class="px-5 pb-5 text-center">
				<span class="loading-pulse text-sm text-[rgba(255,255,255,0.45)]">Cargando fuentes...</span>
			</div>
		{:else}
			<div class="overflow-x-auto px-3 pb-3">
				<table class="table-hud">
					<thead>
						<tr>
							<th>Estado</th>
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
								<td>
									<div class="inline-flex items-center gap-2">
										<span class={`status-dot ${state.dot}`}></span>
										<span class="text-[11px] text-[rgba(255,255,255,0.42)]">{state.label}</span>
									</div>
								</td>
								<td>
									<div class="font-semibold text-[rgba(255,255,255,0.85)]">{source.name}</div>
									{#if source.last_error}
										<div class="mt-1 max-w-[300px] truncate text-[11px] text-[rgba(255,255,255,0.38)]" title={source.last_error}>
											{source.last_error}
										</div>
									{/if}
								</td>
								<td>
									<span class="source-badge source-badge--rss">{source.source_type.toUpperCase()}</span>
								</td>
								<td class="hidden sm:table-cell text-xs text-[rgba(255,255,255,0.52)]">
									{source.sections.map((s) => s.display_name).join(', ') || '—'}
								</td>
								<td class="hidden md:table-cell text-xs text-[rgba(255,255,255,0.42)]">
									{formatRelativeTime(source.last_fetched_at)}
								</td>
								<td class="hidden lg:table-cell">
									<div class="text-[11px] leading-tight text-[rgba(255,255,255,0.4)]">
										<span>Total: {source.stats.total_ingested}</span>
										<span class="mx-1">•</span>
										<span>24h: {source.stats.last_24h}</span>
										<span class="mx-1">•</span>
										<span>{source.stats.pass_rate_pct.toFixed(1)}%</span>
									</div>
								</td>
								<td class="text-right">
									<button class="btn-ghost !px-3 !py-1.5 !text-[10px]" onclick={() => toggleSource(source)}>
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
		<div class="panel surface-pad">
			<h2 class="text-base font-extrabold text-[rgba(255,255,255,0.9)]">Añadir Fuente RSS</h2>
			<div class="mt-4 space-y-3">
				<input class="input" placeholder="Nombre" bind:value={newSourceName} />
				<input class="input" placeholder="URL RSS" bind:value={newSourceURL} />

				<div>
					<div class="mb-2 font-mono text-[10px] font-bold uppercase tracking-[0.16em] text-[rgba(255,255,255,0.34)]">Secciones</div>
					<div class="flex flex-wrap gap-1.5">
						{#each sections as section}
							<label
								class={`section-tab ${newSourceSectionIDs.includes(section.id) ? 'active' : ''}`}
								style={`--section-tint: ${sectionTint(section.name)};`}
							>
								<input
									type="checkbox"
									class="hud-checkbox"
									style={`--section-tint: ${sectionTint(section.name)};`}
									checked={newSourceSectionIDs.includes(section.id)}
									onchange={() => toggleNewSourceSection(section.id)}
								/>
								{section.display_name}
							</label>
						{/each}
					</div>
				</div>

				<button class="btn-primary w-full" disabled={saving} onclick={createSource}>
					{saving ? 'Validando...' : 'Añadir fuente'}
				</button>
			</div>
		</div>

		<div class="panel surface-pad">
			<h2 class="text-base font-extrabold text-[rgba(255,255,255,0.9)]">Crear Sección</h2>
			<div class="mt-4 space-y-3">
				<input class="input" placeholder="name (slug)" bind:value={newSectionName} />
				<input class="input" placeholder="display_name (ej: Seguridad)" bind:value={newSectionDisplayName} />
				<textarea
					class="input"
					rows="3"
					placeholder="seed keywords separadas por coma"
					bind:value={newSectionKeywords}
				></textarea>
				<button class="btn-primary w-full" onclick={createSection}>Crear sección</button>
			</div>
		</div>
	</div>

	<div class="panel surface-pad">
		<div class="flex flex-wrap items-end justify-between gap-3">
			<div>
				<h2 class="text-base font-extrabold text-[rgba(255,255,255,0.9)]">Gestión de Secciones</h2>
				<p class="mt-1 text-sm text-[rgba(255,255,255,0.5)]">
					Arrastra para reordenar, activa o desactiva y ajusta el máximo por briefing.
				</p>
			</div>
		</div>

		<div class="mt-4 space-y-2">
			{#each sections as section (section.id)}
				<div
					class="panel-subtle rounded-2xl p-3 transition-all duration-200"
					draggable="true"
					role="listitem"
					ondragstart={() => onDragStart(section.id)}
					ondragover={(event) => event.preventDefault()}
					ondrop={() => onDrop(section.id)}
				>
					<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
						<div class="flex items-center gap-3">
							<span class="text-sm text-[rgba(255,255,255,0.4)]">⠿</span>
							<div>
								<div class="text-sm font-bold text-[rgba(255,255,255,0.86)]">{section.display_name}</div>
								<div class="text-xs text-[rgba(255,255,255,0.42)]">
									{section.name} · orden: {section.sort_order}
								</div>
							</div>
						</div>

						<div class="flex flex-wrap items-center gap-2.5">
							<label class="inline-flex cursor-pointer items-center gap-1.5 text-xs font-semibold text-[rgba(255,255,255,0.58)]">
								<input
									type="checkbox"
									class="hud-checkbox"
									style={`--section-tint: ${sectionTint(section.name)};`}
									checked={section.enabled}
									onchange={() => toggleSectionEnabled(section)}
								/>
								Activa
							</label>

							<div class="flex items-center gap-1.5">
								<input
									type="number"
									class="input !w-20 !px-2 !py-1.5 text-center text-xs"
									min="1"
									bind:value={section.max_briefing_articles}
								/>
								<button class="btn-ghost !px-3 !py-1.5 !text-[10px]" onclick={() => saveSectionMax(section)}>
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
