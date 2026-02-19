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

<div class="flex min-h-[60vh] items-center justify-center">
	<div class="glass-elevated w-full max-w-sm p-8 animate-fade-up">
		<!-- Logo -->
		<div class="mb-6 text-center">
			<div class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl" style="background: linear-gradient(135deg, #06b6d4, #0891b2); box-shadow: 0 4px 20px -4px rgba(6,182,212,0.4);">
				<span class="font-mono text-lg font-bold text-slate-950">F</span>
			</div>
			<h1 class="mt-4 text-lg font-semibold" style="color: var(--flux-text);">Flux</h1>
			<p class="mt-1 text-xs" style="color: var(--flux-text-muted);">Introduce tu token para acceder</p>
		</div>

		{#if error}
			<div class="mb-4 rounded-xl p-3 text-center text-sm" style="background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.2); color: #fca5a5;">
				{error}
			</div>
		{/if}

		<div class="space-y-4">
			<input
				class="input w-full text-center font-mono tracking-wider"
				type="password"
				placeholder="Token"
				bind:value={token}
				on:keydown={onKeyDown}
				autocomplete="current-password"
			/>

			<button
				class="btn-primary w-full"
				on:click={login}
			>
				Acceder
			</button>
		</div>

		<p class="mt-5 text-center text-[11px]" style="color: var(--flux-text-muted);">
			Deja vacío si usas auth por reverse-proxy.
		</p>
	</div>
</div>