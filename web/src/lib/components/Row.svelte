<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import IconButton from './IconButton.svelte';
	import SectionHeader from './SectionHeader.svelte';

	interface Props {
		title: string;
		/** "Tout voir" target. */
		href?: string;
		items: T[];
		item: Snippet<[T, number]>;
		/** Width of one item (CSS length). */
		itemWidth?: string;
		key?: (item: T, index: number) => unknown;
	}

	let { title, href, items, item, itemWidth = 'clamp(140px, 12.5vw, 184px)', key = (_, i) => i }: Props = $props();

	const uid = $props.id();
	let track = $state<HTMLElement>();
	let canPrev = $state(false);
	let canNext = $state(false);

	function update() {
		if (!track) return;
		canPrev = track.scrollLeft > 4;
		canNext = track.scrollLeft + track.clientWidth < track.scrollWidth - 4;
	}

	function page(direction: 1 | -1) {
		if (!track) return;
		const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
		track.scrollBy({ left: direction * track.clientWidth * 0.8, behavior: reduced ? 'auto' : 'smooth' });
	}

	// Measured after layout (ResizeObserver, next frame), never synchronously while
	// mounting: a forced reflow per row was the main cost of the home page on mobile.
	let frame = 0;
	function schedule() {
		if (frame) return;
		frame = requestAnimationFrame(() => {
			frame = 0;
			update();
		});
	}

	$effect(() => {
		if (!track) return;
		const ro = new ResizeObserver(schedule);
		ro.observe(track);
		return () => {
			ro.disconnect();
			cancelAnimationFrame(frame);
			frame = 0;
		};
	});

	$effect(() => {
		void items.length;
		schedule();
	});
</script>

<section class="row" aria-labelledby="row-{uid}">
	<SectionHeader {title} {href} id="row-{uid}">
		{#snippet actions()}
			{#if canPrev || canNext}
				<span class="arrows">
					<IconButton icon="chevron-left" size="sm" aria-label="Précédents — {title}" aria-disabled={!canPrev} onclick={() => canPrev && page(-1)} />
					<IconButton icon="chevron-right" size="sm" aria-label="Suivants — {title}" aria-disabled={!canNext} onclick={() => canNext && page(1)} />
				</span>
			{/if}
		{/snippet}
	</SectionHeader>

	<ul class="track" role="list" bind:this={track} onscroll={schedule} style:--item-w={itemWidth}>
		{#each items as it, i (key(it, i))}
			<li>{@render item(it, i)}</li>
		{/each}
	</ul>
</section>

<style>
	.row {
		position: relative;
		min-width: 0;
	}

	.arrows {
		display: none;
		gap: 6px;
	}

	@media (hover: hover) and (min-width: 768px) {
		.arrows {
			display: inline-flex;
		}
	}

	/* The track bleeds into the page gutters so cards scroll edge to edge. */
	.track {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: var(--item-w);
		gap: var(--gap);
		margin: calc(-1 * var(--sp-2)) calc(-1 * var(--gutter)) calc(-1 * var(--sp-4));
		/* Room for hover lift and shadows, without clipping. */
		padding: var(--sp-2) var(--gutter) var(--sp-4);
		overflow-x: auto;
		overscroll-behavior-x: contain;
		scroll-snap-type: x mandatory;
		scroll-padding-inline: var(--gutter);
		scrollbar-width: none;
	}

	.track::-webkit-scrollbar {
		display: none;
	}

	li {
		scroll-snap-align: start;
		min-width: 0;
	}
</style>
