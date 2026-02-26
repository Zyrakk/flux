<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { clearAuthToken, getAuthToken } from '$lib/api';

	let hasToken = false;

	onMount(() => {
		hasToken = getAuthToken() !== '';
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/service-worker.js').catch(() => {});
		}
	});

	afterNavigate(() => {
		hasToken = getAuthToken() !== '';
	});

	async function logout() {
		clearAuthToken();
		hasToken = false;
		if (browser) {
			await goto('/login');
		}
	}

	const navItems = [
		{ href: '/', label: 'Briefing', icon: '◆' },
		{ href: '/feed', label: 'Feed', icon: '▤' },
		{ href: '/admin/sources', label: 'Admin', icon: '⚙' }
	];

	function isActive(href: string, pathname: string): boolean {
		if (href === '/') return pathname === '/';
		return pathname.startsWith(href);
	}
</script>

<svelte:head>
	<title>Flux</title>
</svelte:head>

<div class="site-shell">
	<header class="site-header">
		<div class="site-header__inner">
			<a href="/" class="site-brand">
				<span class="site-brand__mark">F</span>
				<span class="site-brand__text">
					<span class="site-brand__title">Flux Intelligence</span>
					<span class="site-brand__subtitle">Daily Signal Briefing</span>
				</span>
			</a>

			<div class="flex items-center gap-2">
				<nav class="site-nav">
					{#each navItems as item}
						<a
							href={item.href}
							class="site-nav__link {isActive(item.href, $page.url.pathname) ? 'active' : ''}"
						>
							<span class="site-nav__link-icon">{item.icon}</span>
							{item.label}
						</a>
					{/each}
				</nav>

				{#if hasToken}
					<button class="btn-ghost !rounded-full !px-3 !py-2 !text-[11px]" on:click={logout}>
						Salir
					</button>
				{/if}
			</div>
		</div>
	</header>

	<main class="site-main">
		{#key $page.url.pathname}
			<div class="route-stage" in:fade={{ duration: 260 }} out:fade={{ duration: 140 }}>
				<slot />
			</div>
		{/key}
	</main>

	<footer class="site-footer">Flux · read less, know more</footer>
</div>
