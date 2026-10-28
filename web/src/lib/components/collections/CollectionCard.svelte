<script lang="ts">
	// Collection tile: 2:3 artwork (the collection's poster, or a 2×2 mosaic of its first posters),
	// name and count. Manual collections without artwork fetch their first items for the mosaic.
	import { api } from '$lib/api/client';
	import { firstImage } from '$lib/api/images';
	import type { Collection, Image } from '$lib/api/types';
	import { plural } from '$lib/format';
	import Artwork from '../Artwork.svelte';
	import Icon from '../Icon.svelte';

	interface Props {
		collection: Collection;
		/** Posters for the mosaic (skips the fetch; design gallery). */
		posters?: Image[];
		sizes?: string;
	}

	let { collection, posters, sizes = '(max-width: 479px) 45vw, 184px' }: Props = $props();

	let fetched = $state<Image[]>([]);
	const poster = $derived(firstImage(collection, 'poster'));
	/** Manual collections always show their contents; TMDB ones have a real poster. */
	const mosaic = $derived(posters ?? (collection.kind === 'manual' || !poster ? fetched : []));

	$effect(() => {
		if (posters || collection.count < 2 || (collection.kind === 'tmdb' && poster)) return;
		const ctrl = new AbortController();
		api
			.collectionItems(collection.id, ctrl.signal)
			.then((r) => (fetched = r.items.map((it) => firstImage(it, 'poster')).filter((i): i is Image => !!i).slice(0, 4)))
			.catch(() => {});
		return () => ctrl.abort();
	});
</script>

<a class="collection-card" href="/collections/{encodeURIComponent(collection.id)}">
	<div class="frame">
		{#if mosaic.length >= 3}
			<div class="mosaic" class:three={mosaic.length === 3} aria-hidden="true">
				{#each mosaic.slice(0, 4) as img, i (i)}
					<span class="cell"><Artwork image={img} ratio="2 / 3" sizes="92px" /></span>
				{/each}
			</div>
		{:else if poster || mosaic[0]}
			<Artwork image={poster ?? mosaic[0]} ratio="2 / 3" {sizes} />
		{:else}
			<div class="empty" aria-hidden="true"><Icon name="collection" size={34} /></div>
		{/if}
		<span class="count tabular" aria-hidden="true">{collection.count}</span>
	</div>
	<div class="meta">
		<span class="title">{collection.name}</span>
		<span class="sub">
			{plural(collection.count, 'titre', 'titres')}{collection.kind === 'tmdb' ? ' · automatique' : collection.shared ? ' · partagée' : ''}
		</span>
	</div>
</a>

<style>
	.collection-card {
		display: block;
		width: 100%;
		border-radius: var(--r-lg);
		color: var(--text-muted);
	}

	.collection-card:focus-visible {
		outline-offset: 4px;
	}

	.frame {
		position: relative;
		aspect-ratio: 2 / 3;
		border-radius: var(--r-lg);
		overflow: hidden;
		background: var(--tint), var(--surface);
		box-shadow: var(--shadow-1), inset 0 0 0 1px var(--line);
		isolation: isolate;
		transition:
			box-shadow var(--dur-base) var(--ease),
			transform var(--dur-base) var(--ease);
	}

	a:hover .frame {
		box-shadow: var(--shadow-2);
		transform: translateY(-3px);
	}

	.mosaic {
		display: grid;
		grid-template-columns: 1fr 1fr;
		grid-template-rows: 1fr 1fr;
		gap: 2px;
		height: 100%;
		background: var(--bg-sunken);
	}

	.mosaic .cell {
		min-height: 0;
		overflow: hidden;
	}

	.mosaic .cell :global(*) {
		height: 100%;
	}

	.three .cell:first-child {
		grid-row: 1 / span 2;
	}

	.empty {
		display: grid;
		place-items: center;
		height: 100%;
		color: var(--text-faint);
	}

	.count {
		position: absolute;
		right: 8px;
		bottom: 8px;
		min-width: 26px;
		height: 24px;
		padding: 0 8px;
		border-radius: var(--r-full);
		background: rgba(4, 14, 22, 0.72);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
		font-size: var(--fs-micro);
		font-weight: var(--fw-heavy);
		line-height: 24px;
		text-align: center;
	}

	.meta {
		display: grid;
		gap: 1px;
		padding: 10px 4px 0;
		line-height: var(--lh-small);
	}

	.title {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.sub {
		font-size: var(--fs-micro);
	}
</style>
