<script lang="ts">
	import type { Item } from '$lib/api/types';
	import { formatTrackTime } from '$lib/format';
	import Skeleton from '../Skeleton.svelte';

	interface Props {
		tracks: Item[];
		loading?: boolean;
	}

	let { tracks, loading = false }: Props = $props();

	// Group by disc only when the album has several.
	const discs = $derived.by(() => {
		const map = new Map<number, Item[]>();
		for (const t of tracks) {
			const d = t.parentIndexNumber ?? 1;
			if (!map.has(d)) map.set(d, []);
			map.get(d)!.push(t);
		}
		return [...map.entries()].sort((a, b) => a[0] - b[0]);
	});
</script>

<div class="tracks panel" aria-busy={loading}>
	{#if loading}
		{#each [0, 1, 2, 3] as i (i)}<div class="row sk" aria-hidden="true"><Skeleton height="14px" width="60%" radius="sm" /></div>{/each}
	{:else}
		{#each discs as [disc, list] (disc)}
			{#if discs.length > 1}<h3 class="disc micro">Disque {disc}</h3>{/if}
			<ol role="list" aria-label={discs.length > 1 ? `Disque ${disc}` : 'Pistes'}>
				{#each list as t, i (t.id)}
					<li class="row">
						<span class="num tabular">{t.indexNumber ?? i + 1}</span>
						<span class="title">{t.title}</span>
						<span class="dur tabular">{formatTrackTime(t.runtimeMs) ?? '–'}</span>
					</li>
				{/each}
			</ol>
		{/each}
	{/if}
</div>

<style>
	.tracks {
		padding: var(--sp-2);
	}

	.disc {
		padding: var(--sp-3) var(--sp-3) var(--sp-1);
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.row {
		display: grid;
		grid-template-columns: 2.5em minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--sp-3);
		height: 48px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-md);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.row:hover {
		background: color-mix(in srgb, var(--text) 5%, transparent);
	}

	.num {
		color: var(--text-muted);
		font-weight: var(--fw-bold);
		text-align: right;
	}

	.title {
		font-weight: var(--fw-medium);
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.dur {
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.sk {
		grid-template-columns: 1fr;
	}
</style>
