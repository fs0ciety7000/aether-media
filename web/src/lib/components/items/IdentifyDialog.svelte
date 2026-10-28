<script lang="ts">
	// Manual identification: search the metadata providers, pick a candidate, POST identify.
	import { api, ApiError, errorMessage, isAbort } from '$lib/api/client';
	import type { Candidate, Item } from '$lib/api/types';
	import { toasts } from '$lib/toast.svelte';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import Input from '../Input.svelte';
	import Skeleton from '../Skeleton.svelte';

	interface Props {
		open: boolean;
		item: Item;
		/** Called once the identification is queued. */
		onidentified?: (item: Item, candidate: Candidate) => void;
	}

	let { open = $bindable(false), item, onidentified }: Props = $props();

	let title = $state('');
	let year = $state('');
	let candidates = $state<Candidate[] | null>(null);
	let searching = $state(false);
	let choosing = $state<string | null>(null);
	let error = $state<string | null>(null);
	let unavailable = $state(false);
	let ctrl: AbortController | undefined;

	$effect(() => {
		if (!open) {
			ctrl?.abort();
			return;
		}
		title = item.title;
		year = item.year ? String(item.year) : '';
		candidates = null;
		error = null;
		unavailable = false;
	});

	async function search(e?: SubmitEvent) {
		e?.preventDefault();
		ctrl?.abort();
		const c = (ctrl = new AbortController());
		searching = true;
		error = null;
		unavailable = false;
		try {
			const y = parseInt(year, 10);
			candidates = await api.identifySearch(item.id, { title: title.trim() || undefined, year: Number.isFinite(y) ? y : undefined }, c.signal);
		} catch (err) {
			if (isAbort(err)) return;
			unavailable = err instanceof ApiError && err.status === 503;
			error = unavailable ? 'Aucun fournisseur de métadonnées n’est actif. Configurez une clé TMDB pour identifier films et séries.' : errorMessage(err);
			candidates = null;
		} finally {
			if (ctrl === c) searching = false;
		}
	}

	const key = (c: Candidate) => `${c.provider}:${Object.values(c.providerIds).join(',')}`;

	async function choose(c: Candidate) {
		choosing = key(c);
		try {
			await api.identify(item.id, c.providerIds);
			toasts.show(`« ${item.title} » identifié`, { detail: `${c.title}${c.year ? ` (${c.year})` : ''} — métadonnées en cours de mise à jour.` });
			onidentified?.(item, c);
			open = false;
		} catch (err) {
			error = errorMessage(err);
		} finally {
			choosing = null;
		}
	}

	const providerName = (p: string) => ({ tmdb: 'TMDB', musicbrainz: 'MusicBrainz', imdb: 'IMDb', tvdb: 'TVDB' })[p] ?? p;
</script>

<Dialog bind:open title="Identifier « {item.title} »" description={item.path} size="lg">
	<form class="search" onsubmit={search}>
		<Input label="Titre" bind:value={title} autocomplete="off" />
		<Input label="Année" bind:value={year} inputmode="numeric" class="year" autocomplete="off" />
		<Button variant="primary" type="submit" icon="search" disabled={searching || !title.trim()}>Rechercher</Button>
	</form>

	{#if error}
		<p class="error" role="alert"><Icon name={unavailable ? 'key' : 'alert'} size={16} /><span>{error}</span></p>
	{/if}

	<div class="results" aria-live="polite" aria-busy={searching}>
		{#if searching}
			<ul class="grid" role="list" aria-hidden="true">
				{#each [0, 1, 2, 3] as i (i)}
					<li class="cand sk"><Skeleton ratio="2 / 3" radius="md" /><Skeleton lines={3} /></li>
				{/each}
			</ul>
		{:else if candidates && !candidates.length}
			<p class="none">Aucun résultat. Essayez un autre titre, sans l’année, ou le titre original.</p>
		{:else if candidates}
			<p class="micro hint">{candidates.length} proposition{candidates.length > 1 ? 's' : ''} — choisissez la bonne correspondance.</p>
			<ul class="grid" role="list">
				{#each candidates as c (key(c))}
					{@const pct = Math.round(Math.max(0, Math.min(1, c.score > 1 ? c.score / 100 : c.score)) * 100)}
					<li>
						<button type="button" class="cand" onclick={() => choose(c)} disabled={!!choosing} aria-label="Choisir {c.title}{c.year ? ` (${c.year})` : ''}, correspondance {pct} %">
							<span class="img">
								{#if c.imageUrl}<img src={c.imageUrl} alt="" loading="lazy" />{:else}<Icon name="image" size={22} />{/if}
							</span>
							<span class="body">
								<span class="name">{c.title}</span>
								<span class="meta tabular">{[c.year, providerName(c.provider)].filter(Boolean).join(' · ')}</span>
								{#if c.overview}<span class="overview">{c.overview}</span>{/if}
								<span class="score" aria-hidden="true">
									<span class="bar"><span style:transform="scaleX({pct / 100})"></span></span>
									<span class="tabular">{pct} %</span>
								</span>
							</span>
							{#if choosing === key(c)}<span class="busy micro">Identification…</span>{/if}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</Dialog>

<style>
	.search {
		display: flex;
		align-items: flex-end;
		gap: var(--sp-3);
	}

	.search > :global(.field:first-child) {
		flex: 1;
	}

	.search > :global(.field.year) {
		flex: none;
		width: 110px;
	}

	.search > :global(.btn) {
		flex: none;
	}

	.error {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		margin-top: var(--sp-4);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--danger) 12%, transparent);
		color: var(--text);
		font-size: var(--fs-small);
	}

	.error :global(svg) {
		margin-top: 2px;
		color: var(--danger);
	}

	.results {
		margin-top: var(--sp-5);
	}

	.hint {
		margin-bottom: var(--sp-3);
		color: var(--text-muted);
	}

	.none {
		color: var(--text-muted);
		text-align: center;
		padding: var(--sp-6) 0;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 360px), 1fr));
		gap: var(--gap);
	}

	.cand {
		position: relative;
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr);
		gap: var(--sp-3);
		width: 100%;
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		text-align: left;
		transition:
			box-shadow var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.cand.sk {
		grid-template-columns: 76px 1fr;
	}

	button.cand:hover:not(:disabled) {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.img {
		display: grid;
		place-items: center;
		aspect-ratio: 2 / 3;
		border-radius: var(--r-sm);
		overflow: hidden;
		background: var(--bg-sunken);
		color: var(--text-faint);
	}

	.img img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
	}

	.name {
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.meta {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.overview {
		font-size: var(--fs-small);
		color: var(--text-muted);
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.score {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		margin-top: auto;
		padding-top: var(--sp-2);
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.bar {
		flex: 1;
		height: 4px;
		border-radius: var(--r-full);
		background: color-mix(in srgb, var(--text) 12%, transparent);
		overflow: hidden;
	}

	.bar span {
		display: block;
		height: 100%;
		background: var(--accent);
		transform-origin: left;
	}

	.busy {
		position: absolute;
		top: var(--sp-3);
		right: var(--sp-3);
		color: var(--accent-ink);
	}

	@media (max-width: 767px) {
		.search {
			flex-wrap: wrap;
		}

		.search > :global(.field:first-child) {
			flex-basis: calc(100% - 110px - var(--sp-3));
		}

		.search > :global(.btn) {
			width: 100%;
		}
	}
</style>
