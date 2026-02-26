<svelte:options runes={true} />
<script lang="ts">
	import { goto } from '$app/navigation';
	import { setAuthToken } from '$lib/api';

	let token = $state('');
	let error = $state('');

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

<div class="flex min-h-[72vh] items-center justify-center px-3">
	<div class="login-panel">
		<div class="mb-7 text-center">
			<div class="site-brand__mark mx-auto h-14 w-14 text-xl">F</div>
			<h1 class="login-title mt-4">Flux Access</h1>
			<p class="mt-2 text-sm text-[rgba(255,255,255,0.5)]">Introduce tu token para acceder al briefing.</p>
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
				onkeydown={onKeyDown}
				autocomplete="current-password"
			/>

			<button class="btn-primary w-full !py-2.5" onclick={login}>Acceder</button>
		</div>

		<p class="mt-5 text-center font-mono text-[10px] font-semibold uppercase tracking-[0.14em] text-[rgba(255,255,255,0.32)]">
			Puedes dejarlo vacío si usas auth por reverse-proxy.
		</p>
	</div>
</div>
