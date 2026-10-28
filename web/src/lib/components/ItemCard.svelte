<script lang="ts">
	// Card for any API item: 2:3 poster for films/series, square cover for albums, round for artists.
	import { firstImage } from '$lib/api/images';
	import type { Item } from '$lib/api/types';
	import { KIND_ICON, KIND_LABEL, isOffline, itemHref } from '$lib/media';
	import { metaLine, plural } from '$lib/format';
	import PosterCard from './PosterCard.svelte';

	interface Props {
		item: Item;
		sizes?: string;
		/** Secondary line override. */
		subtitle?: string;
	}

	let { item, sizes, subtitle }: Props = $props();

	const square = $derived(item.kind === 'artist' || item.kind === 'album' || item.kind === 'track');
	const image = $derived(firstImage(item, square ? 'poster' : 'poster', 'thumb'));
	const line = $derived.by(() => {
		if (subtitle) return subtitle;
		if (item.kind === 'series' && item.childCount) return metaLine(item.year, plural(item.childCount, 'saison', 'saisons'));
		if (item.kind === 'artist') return item.childCount ? plural(item.childCount, 'album', 'albums') : 'Artiste';
		if (item.kind === 'season' && item.childCount) return plural(item.childCount, 'épisode', 'épisodes');
		return undefined;
	});
</script>

<PosterCard
	href={itemHref(item)}
	title={item.title}
	year={item.year}
	subtitle={line ?? (item.year ? undefined : KIND_LABEL[item.kind].one.replace(/^./, (c) => c.toUpperCase()))}
	{image}
	ratio={square ? '1 / 1' : '2 / 3'}
	round={item.kind === 'artist'}
	icon={KIND_ICON[item.kind]}
	offline={isOffline(item)}
	{sizes}
/>
