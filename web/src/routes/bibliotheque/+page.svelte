<script lang="ts">
	// Library index (the mobile tab bar has no sidebar).
	import { libraries } from '$lib/api/stores.svelte';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { plural } from '$lib/format';
	import { KIND_LABEL, libraryKindInfo } from '$lib/media';
	import type { Kind } from '$lib/api/types';

	const PRIMARY: Record<string, Kind[]> = { movies: ['movie'], shows: ['series', 'episode'], music: ['artist', 'album'], mixed: ['movie', 'series'] };

	function summary(kind: string, counts: Record<string, number>) {
		const parts = (PRIMARY[kind] ?? []).filter((k) => counts[k]).map((k) => plural(counts[k], KIND_LABEL[k].one, KIND_LABEL[k].many));
		return parts.length ? parts.join(' · ') : 'Aucun titre pour l’instant';
	}
</script>

<svelte:head>
	<title>Bibliothèques — Aether</title>
</svelte:head>

<section class="page">
	<h1>Bibliothèques</h1>
	{#if !libraries.loaded}
		<div class="list">
			{#each [0, 1, 2] as i (i)}<Skeleton height="84px" radius="lg" />{/each}
		</div>
	{:else if !libraries.list.length}
		<EmptyState icon="layers" title="Aucune bibliothèque">
			<p>Ajoutez un dossier de films, de séries ou de musique pour commencer.</p>
			{#snippet actions()}<Button variant="primary" icon="plus" href="/reglages/bibliotheques?ajouter=1">Ajouter une bibliothèque</Button>{/snippet}
		</EmptyState>
	{:else}
		<ul class="list" role="list">
			{#each libraries.list as lib (lib.id)}
				{@const info = libraryKindInfo(lib.kind)}
				<li>
					<a class="lib panel" href="/bibliotheque/{lib.id}">
						<span class="icon" aria-hidden="true"><Icon name={info.icon} size={22} /></span>
						<span class="text">
							<strong>{lib.name}</strong>
							<span>{info.label} · {summary(lib.kind, lib.counts)}</span>
						</span>
						<Icon name="chevron-right" size={18} />
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	h1 {
		margin-bottom: var(--sp-5);
	}

	.list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
		gap: var(--gap);
	}

	.lib {
		display: flex;
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		color: var(--text-muted);
		transition: box-shadow var(--dur-fast) var(--ease);
	}

	.lib:hover {
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 48px;
		height: 48px;
		border-radius: 14px;
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.text {
		display: grid;
		flex: 1;
		min-width: 0;
		font-size: var(--fs-small);
	}

	.text strong {
		color: var(--text);
		font-size: var(--fs-h3);
	}
</style>
