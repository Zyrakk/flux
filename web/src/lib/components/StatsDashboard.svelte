<svelte:options runes={true} />
<script lang="ts">
	import AnimatedCounter from '$lib/components/AnimatedCounter.svelte';

	export type StatItem = {
		label: string;
		value: number | string;
		color: string;
		mono?: boolean;
	};

	type StatsDashboardProps = {
		stats: StatItem[];
	};

	let { stats }: StatsDashboardProps = $props();
</script>

<div class="stats-dashboard">
	{#each stats as stat (stat.label)}
		<div class="stat-tile">
			<div class="stat-tile__label">{stat.label}</div>
			<div class="stat-tile__value">
				<AnimatedCounter value={stat.value} color={stat.color} mono={stat.mono ?? true} />
			</div>
		</div>
	{/each}
</div>
