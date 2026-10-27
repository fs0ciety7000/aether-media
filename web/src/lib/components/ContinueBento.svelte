<script lang="ts" generics="T">
	// "Reprendre" bento: 2 medium tiles over 3 small ones, a large featured tile spanning both rows.
	// The first item is the featured one (first in reading order, first on mobile).
	import type { Snippet } from 'svelte';
	import SectionHeader from './SectionHeader.svelte';

	interface Props {
		title?: string;
		href?: string;
		/** Up to six items; the first is featured. */
		items: T[];
		tile: Snippet<[T, { featured: boolean; index: number }]>;
		key?: (item: T, index: number) => unknown;
	}

	let { title = 'Reprendre', href, items, tile, key = (_, i) => i }: Props = $props();
	const uid = $props.id();
</script>

<section class="bento-section" aria-labelledby="bento-{uid}">
	<SectionHeader {title} {href} id="bento-{uid}" />
	<ul class="bento" role="list">
		{#each items.slice(0, 6) as it, i (key(it, i))}
			<li class="cell">{@render tile(it, { featured: i === 0, index: i })}</li>
		{/each}
	</ul>
</section>

<style>
	.bento-section {
		min-width: 0;
	}

	.bento {
		display: grid;
		grid-template-columns: repeat(6, minmax(0, 1fr)) minmax(0, 4.4fr);
		grid-template-rows: repeat(2, minmax(0, 1fr));
		gap: var(--gap);
		aspect-ratio: 3.35 / 1;
		list-style: none;
		padding: 0;
	}

	.cell {
		min-width: 0;
		min-height: 0;
	}

	/* Featured: right column, both rows. */
	.cell:nth-child(1) {
		grid-column: 7;
		grid-row: 1 / span 2;
	}

	/* Two medium tiles on top. */
	.cell:nth-child(2) {
		grid-column: 1 / span 3;
		grid-row: 1;
	}
	.cell:nth-child(3) {
		grid-column: 4 / span 3;
		grid-row: 1;
	}

	/* Three small tiles below. */
	.cell:nth-child(4) {
		grid-column: 1 / span 2;
		grid-row: 2;
	}
	.cell:nth-child(5) {
		grid-column: 3 / span 2;
		grid-row: 2;
	}
	.cell:nth-child(6) {
		grid-column: 5 / span 2;
		grid-row: 2;
	}

	@media (max-width: 1023px) and (min-width: 768px) {
		.bento {
			aspect-ratio: 2.3 / 1;
			grid-template-columns: repeat(6, minmax(0, 1fr)) minmax(0, 3.6fr);
		}
	}

	/* Mobile: two columns, featured first and full width. */
	@media (max-width: 767px) {
		.bento {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			grid-template-rows: none;
			aspect-ratio: auto;
		}

		.cell:nth-child(n) {
			grid-column: auto;
			grid-row: auto;
			aspect-ratio: 16 / 11;
		}

		.cell:nth-child(1) {
			grid-column: 1 / -1;
			aspect-ratio: 16 / 9;
		}

		.cell:last-child:nth-child(even) {
			grid-column: 1 / -1;
			aspect-ratio: 16 / 7;
		}
	}
</style>
