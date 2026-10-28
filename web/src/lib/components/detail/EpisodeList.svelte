<script lang="ts">
	import { firstImage } from '$lib/api/images';
	import type { Item } from '$lib/api/types';
	import { formatDate, formatRuntime, metaLine } from '$lib/format';
	import Artwork from '../Artwork.svelte';
	import Icon from '../Icon.svelte';
	import Skeleton from '../Skeleton.svelte';

	interface Props {
		episodes: Item[];
		loading?: boolean;
		label: string;
	}

	let { episodes, loading = false, label }: Props = $props();
</script>

<ol class="episodes" role="list" aria-label={label} aria-busy={loading}>
	{#if loading}
		{#each [0, 1, 2] as i (i)}
			<li class="ep sk" aria-hidden="true">
				<span class="thumb"><Skeleton ratio="16 / 9" radius="md" /></span>
				<span class="body"><Skeleton width="40%" height="14px" radius="sm" /><Skeleton lines={2} /></span>
			</li>
		{/each}
	{:else}
		{#each episodes as ep (ep.id)}
			<li>
				<a class="ep" href="/item/{ep.id}">
					<span class="thumb">
						<Artwork image={firstImage(ep, 'thumb', 'backdrop')} ratio="16 / 9" sizes="(max-width: 767px) 40vw, 240px" placeholder={{ title: ep.title }} />
						<span class="play" aria-hidden="true"><Icon name="play" size={16} filled /></span>
					</span>
					<span class="body">
						<span class="top">
							<span class="num tabular">{ep.indexNumber ?? '–'}</span>
							<span class="title">{ep.title}</span>
							<span class="dur tabular">{formatRuntime(ep.runtimeMs) ?? ''}</span>
						</span>
						{#if ep.airDate || ep.premiereDate}<span class="date">{metaLine(formatDate(ep.airDate ?? ep.premiereDate))}</span>{/if}
						{#if ep.overview}<span class="overview">{ep.overview}</span>{/if}
					</span>
				</a>
			</li>
		{/each}
	{/if}
</ol>

<style>
	.episodes {
		display: grid;
		gap: var(--sp-2);
	}

	.ep {
		display: grid;
		grid-template-columns: clamp(160px, 18vw, 240px) minmax(0, 1fr);
		gap: var(--sp-4);
		align-items: center;
		padding: var(--sp-2);
		border-radius: var(--r-lg);
		transition: background-color var(--dur-fast) var(--ease);
	}

	a.ep:hover {
		background: var(--surface);
	}

	.thumb {
		position: relative;
		display: block;
		border-radius: var(--r-md);
		overflow: hidden;
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.play {
		position: absolute;
		left: 50%;
		top: 50%;
		display: grid;
		place-items: center;
		width: 38px;
		height: 38px;
		margin: -19px 0 0 -19px;
		padding-left: 2px;
		border-radius: 50%;
		background: var(--accent);
		color: var(--accent-contrast);
		opacity: 0;
		transition: opacity var(--dur-base) var(--ease);
	}

	a.ep:hover .play,
	a.ep:focus-visible .play {
		opacity: 1;
	}

	.body {
		display: grid;
		gap: 4px;
		min-width: 0;
	}

	.top {
		display: flex;
		align-items: baseline;
		gap: var(--sp-3);
	}

	.num {
		flex: none;
		min-width: 1.5em;
		color: var(--text-muted);
		font-weight: var(--fw-bold);
	}

	.title {
		flex: 1;
		min-width: 0;
		font-weight: var(--fw-bold);
		font-size: 0.9375rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.dur {
		flex: none;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.date {
		font-size: var(--fs-micro);
		color: var(--text-faint);
		padding-left: calc(1.5em + var(--sp-3));
	}

	.overview {
		padding-left: calc(1.5em + var(--sp-3));
		font-size: var(--fs-small);
		color: var(--text-muted);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.sk {
		pointer-events: none;
	}

	@media (max-width: 767px) {
		.ep {
			grid-template-columns: 132px minmax(0, 1fr);
			gap: var(--sp-3);
			align-items: start;
			padding: var(--sp-1) 0;
		}

		.top {
			flex-wrap: wrap;
			gap: 0 var(--sp-2);
		}

		.title {
			white-space: normal;
			display: -webkit-box;
			-webkit-line-clamp: 2;
			line-clamp: 2;
			-webkit-box-orient: vertical;
		}

		.dur {
			order: 3;
			width: 100%;
			padding-left: calc(1.5em + var(--sp-2));
		}

		.date {
			display: none;
		}

		.overview {
			padding-left: 0;
			grid-column: 1 / -1;
		}
	}
</style>
