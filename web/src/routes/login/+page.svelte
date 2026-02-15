<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { apiFetch, getAuthToken, setAuthToken } from '$lib/api';

	let token = '';
	let loading = false;
	let error = '';

	onMount(() => {
		token = getAuthToken();
	});

	async function submit() {
		loading = true;
		error = '';
		setAuthToken(token);
		try {
			const response = await apiFetch('/briefings/latest');
			if (!response.ok && response.status !== 404) {
				throw new Error((await response.text()) || `HTTP ${response.status}`);
			}
			await goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'No se pudo validar el token';
		} finally {
			loading = false;
		}
	}

	async function continueWithoutToken() {
		token = '';
		await submit();
	}
</script>

<div class="mx-auto mt-10 w-full max-w-md surface p-5">
	<h1 class="text-xl font-semibold">Acceso a Flux</h1>
	<p class="mt-2 text-sm text-text-2">
		Introduce tu token (`AUTH_TOKEN`) o continúa sin token si la autenticación la hace Traefik/Caddy.
	</p>

	<form class="mt-4 space-y-3" on:submit|preventDefault={submit}>
		<label class="block text-sm font-medium text-text-1" for="token">Token</label>
		<input id="token" class="input w-full font-mono" bind:value={token} placeholder="tu-token" autocomplete="off" />

		{#if error}
			<div class="rounded-lg border border-red-400/40 bg-red-500/10 px-3 py-2 text-sm text-red-200">{error}</div>
		{/if}

		<div class="flex gap-2">
			<button class="btn-primary flex-1" type="submit" disabled={loading}>
				{#if loading}Validando...{:else}Entrar{/if}
			</button>
			<button class="btn-secondary" type="button" disabled={loading} on:click={continueWithoutToken}>
				Sin token
			</button>
		</div>
	</form>
</div>
