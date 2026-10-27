<script lang="ts" generics="T">
	// "Catégories" panel: tinted card with a 2-column grid of genre tiles (horizontal scroll on mobile).
	import type { Snippet } from 'svelte';
	import SectionHeader from './SectionHeader.svelte';

	interface Props {
		title?: string;
		href?: string;
		items: T[];
		tile: Snippet<[T]>;
		key?: (item: T, index: number) => unknown;
		class?: string;
	}

	let { title = 'Catégories', href, items, tile, key = (_, i) => i, class: className }: Props = $props();
	const uid = $props.id();
</script>

<section class={['categories', 'panel', className]} aria-labelledby="cat-{uid}">
	<SectionHeader {title} {href} id="cat-{uid}" />
	<ul class="grid" role="list">
		{#each items as it, i (key(it, i))}
			<li>{@render tile(it)}</li>
		{/each}
	</ul>
</section>

<style>
	.categories {
		display: flex;
		flex-direction: column;
		min-width: 0;
		padding: var(--sp-4) var(--sp-4) var(--sp-4);
	}

	.categories :global(.section-header) {
		margin-bottom: var(--sp-3);
		padding-left: 2px;
	}

	.grid {
		flex: 1;
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		grid-auto-rows: minmax(64px, 1fr);
		gap: 10px;
		list-style: none;
		padding: 0;
	}

	li {
		min-width: 0;
	}

	@media (max-width: 767px) {
		.categories {
			padding: 0;
			background: none;
			box-shadow: none;
		}

		.categories :global(.section-header) {
			padding-left: 0;
		}

		.grid {
			display: flex;
			gap: 10px;
			margin: 0 calc(-1 * var(--gutter));
			padding: 0 var(--gutter) 2px;
			overflow-x: auto;
			scroll-snap-type: x mandatory;
			scroll-padding-inline: var(--gutter);
			scrollbar-width: none;
		}

		.grid::-webkit-scrollbar {
			display: none;
		}

		li {
			flex: 0 0 132px;
			height: 76px;
			scroll-snap-align: start;
		}
	}
</style>
