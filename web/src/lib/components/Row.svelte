<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

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

	let { title, href, items, item, itemWidth = 'clamp(132px, 13vw, 184px)', key = (_, i) => i }: Props = $props();

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

	$effect(() => {
		if (!track) return;
		items.length;
		update();
		const ro = new ResizeObserver(update);
		ro.observe(track);
		return () => ro.disconnect();
	});
</script>

<section class="row" aria-labelledby="row-{uid}">
	<header>
		<h2 id="row-{uid}">{title}</h2>
		{#if href}
			<a class="see-all" {href}>Tout voir<Icon name="chevron-right" size={16} /></a>
		{/if}
	</header>

	<div class="viewport">
		<ul class="track" role="list" bind:this={track} onscroll={update} style:--item-w={itemWidth}>
			{#each items as it, i (key(it, i))}
				<li>{@render item(it, i)}</li>
			{/each}
		</ul>

		<button class="arrow prev" type="button" aria-label="Précédents" tabindex="-1" disabled={!canPrev} onclick={() => page(-1)}>
			<Icon name="chevron-left" />
		</button>
		<button class="arrow next" type="button" aria-label="Suivants" tabindex="-1" disabled={!canNext} onclick={() => page(1)}>
			<Icon name="chevron-right" />
		</button>
	</div>
</section>

<style>
	.row {
		position: relative;
		min-width: 0;
	}

	header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--sp-4);
		padding: 0 var(--gutter);
		margin-bottom: var(--sp-4);
	}

	h2 {
		font-weight: 500;
		letter-spacing: -0.005em;
	}

	.see-all {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		flex: none;
		font-size: var(--fs-small);
		color: var(--text-muted);
		border-radius: var(--r-sm);
		transition: color var(--dur-fast) var(--ease);
	}

	.see-all:hover {
		color: var(--text);
	}

	.see-all :global(svg) {
		transition: transform var(--dur-fast) var(--ease);
	}

	.see-all:hover :global(svg) {
		transform: translateX(2px);
	}

	.viewport {
		position: relative;
	}

	.track {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: var(--item-w);
		gap: var(--sp-4);
		margin: 0;
		/* Room for hover scale and shadows, without clipping. */
		padding: var(--sp-2) var(--gutter) var(--sp-5);
		margin-block: calc(-1 * var(--sp-2)) calc(-1 * var(--sp-5));
		overflow-x: auto;
		overscroll-behavior-x: contain;
		scroll-snap-type: x mandatory;
		scroll-padding-inline: var(--gutter);
		scrollbar-width: none;
	}

	@media (min-width: 768px) {
		.track {
			gap: var(--sp-5);
		}
	}

	.track::-webkit-scrollbar {
		display: none;
	}

	li {
		scroll-snap-align: start;
		min-width: 0;
	}

	.arrow {
		position: absolute;
		top: calc(50% - 40px);
		display: grid;
		place-items: center;
		width: 44px;
		height: 44px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		color: var(--text);
		box-shadow: var(--shadow-2), inset 0 0 0 1px var(--line);
		opacity: 0;
		pointer-events: none;
		transition:
			opacity var(--dur-base) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.prev {
		left: max(var(--sp-2), calc(var(--gutter) / 2 - 22px));
	}

	.next {
		right: max(var(--sp-2), calc(var(--gutter) / 2 - 22px));
	}

	@media (hover: hover) {
		.row:hover .arrow:not(:disabled) {
			opacity: 1;
			pointer-events: auto;
		}
	}

	.arrow:hover {
		background: color-mix(in srgb, var(--surface-raised) 88%, var(--text));
	}
</style>
