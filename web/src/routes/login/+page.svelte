<script lang="ts">
	import { goto } from '$app/navigation';
	import { setAuthToken } from '$lib/api';

	let token = '';
	let error = '';

	async function login() {
		error = '';
		setAuthToken(token);
		await goto('/');
	}

	function onKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			void login();
		}
	}
</script>

<div class="flex min-h-[70vh] items-center justify-center px-3">
	<div class="briefing-hero w-full max-w-md animate-fade-up">
		<div class="mb-6 text-center">
			<div class="site-brand__mark mx-auto h-14 w-14 text-xl">F</div>
			<h1 class="mt-4 text-2xl font-extrabold tracking-tight text-slate-900">Flux Access</h1>
			<p class="mt-2 text-sm text-slate-600">Introduce tu token para acceder al briefing.</p>
		</div>

		{#if error}
			<div class="alert error mb-4">{error}</div>
		{/if}

		<div class="space-y-4">
			<input
				class="input text-center font-mono tracking-wider"
				type="password"
				placeholder="Token"
				bind:value={token}
				on:keydown={onKeyDown}
				autocomplete="current-password"
			/>

			<button class="btn-primary w-full" on:click={login}>Acceder</button>
		</div>

		<p class="mt-5 text-center text-xs font-semibold uppercase tracking-wide text-slate-500">
			Puedes dejarlo vacío si usas auth por reverse-proxy.
		</p>
	</div>
</div>
