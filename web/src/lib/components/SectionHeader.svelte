<svelte:options runes={true} />
<script lang="ts">
	type SectionHeaderProps = {
		label: string;
		count: number;
		tint: string;
		sublabel?: string;
	};

	let { label, count, tint, sublabel = `${count} SIGNALS DETECTED` }: SectionHeaderProps = $props();
</script>

<div class="section-header" style={`--section-tint: ${tint};`}>
	<div class="section-header__dot-wrap">
		<div class="section-header__dot"></div>
		<div class="section-header__dot-ring"></div>
	</div>
	<h2 class="section-header__label">[{label}]</h2>
	<span class="section-header__sub">- {sublabel}</span>
	<div class="section-header__line"></div>
	<div class="section-header__count">{count}</div>
</div>
