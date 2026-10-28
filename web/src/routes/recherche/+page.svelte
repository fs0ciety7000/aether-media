<script lang="ts">
	// Instant search: debounced (200 ms), in-flight requests cancelled, results grouped by kind,
	// ↑ ↓ to move through results from the search field, Enter to open.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import { firstImage } from '$lib/api/images';
	import type { Item, SearchPerson, SearchResults } from '$lib/api/types';
	import Avatar from '$lib/components/Avatar.svelte';
	import Artwork from '$lib/components/Artwork.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SearchField from '$lib/components/SearchField.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { formatRuntime, formatTrackTime, metaLine, plural } from '$lib/format';
	import { episodeCode, hueFrom, itemHref, KIND_ICON } from '$lib/media';

	type Group = keyof SearchResults['results'];
	const GROUPS: { key: Group; label: string }[] = [
		{ key: 'movie', label: 'Films' },
		{ key: 'series', label: 'Séries' },
		{ key: 'episode', label: 'Épisodes' },
		{ key: 'artist', label: 'Artistes' },
		{ key: 'album', label: 'Albums' },
		{ key: 'track', label: 'Pistes' },
		{ key: 'person', label: 'Personnes' }
	];

	interface Entry {
		key: string;
		href: string;
		title: string;
		meta?: string;
		kind: Group;
		item?: Item;
		person?: SearchPerson;
	}

	const urlQuery = $derived(page.url.searchParams.get('q') ?? '');
	let field = $state('');
	let results = $state<SearchResults | null>(null);
	let loading = $state(false);
	let error = $state<unknown>(null);
	let active = $state(-1);
	let ctrl: AbortController | undefined;
	let urlTimer: ReturnType<typeof setTimeout> | undefined;

	// Page field (phones) mirrors the URL; the top-bar field writes the URL too.
	$effect(() => {
		if (urlQuery.trim() !== field.trim()) field = urlQuery;
	});

	function onField() {
		clearTimeout(urlTimer);
		urlTimer = setTimeout(() => {
			const q = field.trim();
			goto(q ? `/recherche?q=${encodeURIComponent(q)}` : '/recherche', { replaceState: true, keepFocus: true, noScroll: true });
		}, 60);
	}

	$effect(() => {
		const q = urlQuery.trim();
		active = -1;
		ctrl?.abort();
		if (q.length < 2) {
			results = null;
			loading = false;
			error = null;
			return;
		}
		loading = true;
		const c = (ctrl = new AbortController());
		const t = setTimeout(async () => {
			try {
				results = await api.search(q, 8, c.signal);
				error = null;
			} catch (err) {
				if (!isAbort(err)) error = err;
			} finally {
				if (ctrl === c) loading = false;
			}
		}, 200);
		return () => {
			clearTimeout(t);
			c.abort();
		};
	});

	function itemMeta(it: Item): string | undefined {
		switch (it.kind) {
			case 'movie':
				return metaLine(it.year, formatRuntime(it.runtimeMs));
			case 'series':
				return metaLine(it.year, it.childCount ? plural(it.childCount, 'saison', 'saisons') : undefined);
			case 'episode':
				return metaLine(episodeCode(it), it.year);
			case 'artist':
				return it.childCount ? plural(it.childCount, 'album', 'albums') : 'Artiste';
			case 'album':
				return metaLine('Album', it.year);
			case 'track':
				return metaLine('Piste', formatTrackTime(it.runtimeMs));
			default:
				return it.year ? String(it.year) : undefined;
		}
	}

	const groups = $derived.by(() => {
		if (!results) return [];
		return GROUPS.map((g) => {
			const entries: Entry[] =
				g.key === 'person'
					? results!.results.person.map((p) => ({
							key: `p${p.id}`,
							href: `/parcourir?personne=${p.id}&nom=${encodeURIComponent(p.name)}`,
							title: p.name,
							meta: plural(p.count, 'titre', 'titres'),
							kind: g.key,
							person: p
						}))
					: results!.results[g.key].map((it) => ({ key: `i${it.id}`, href: itemHref(it), title: it.title, meta: itemMeta(it), kind: g.key, item: it }));
			return { ...g, entries };
		}).filter((g) => g.entries.length);
	});

	const flat = $derived(groups.flatMap((g) => g.entries));
	const totalCount = $derived(flat.length);
	const indexOf = (e: Entry) => flat.indexOf(e);

	function onKeydown(e: KeyboardEvent) {
		if (!flat.length || e.altKey || e.metaKey || e.ctrlKey) return;
		const target = e.target as HTMLElement;
		const inSearch = target.matches('input[type=search]') || target.closest('.results');
		if (!inSearch) return;
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			const delta = e.key === 'ArrowDown' ? 1 : -1;
			active = active < 0 ? (delta > 0 ? 0 : flat.length - 1) : (active + delta + flat.length) % flat.length;
			document.getElementById(`res-${flat[active].key}`)?.scrollIntoView({ block: 'nearest' });
		} else if (e.key === 'Enter' && active >= 0 && target.matches('input[type=search]')) {
			e.preventDefault();
			goto(flat[active].href);
		}
	}

	const thumbFor = (e: Entry) => {
		if (!e.item) return undefined;
		return e.kind === 'episode' ? firstImage(e.item, 'thumb', 'backdrop') : firstImage(e.item, 'poster', 'thumb');
	};
	const ratioFor = (k: Group) => (k === 'episode' ? '16 / 9' : k === 'artist' || k === 'album' || k === 'track' ? '1 / 1' : '2 / 3');
</script>

<svelte:window onkeydown={onKeydown} />

<svelte:head>
	<title>{urlQuery ? `« ${urlQuery} » — Recherche` : 'Recherche'} — Aether</title>
</svelte:head>

<section class="page search">
	<h1 class="visually-hidden">Recherche</h1>
	<div class="field">
		<SearchField
			bind:value={field}
			size="lg"
			label="Rechercher dans la médiathèque"
			placeholder="Films, séries, artistes, personnes…"
			oninput={onField}
			aria-controls="search-results"
			aria-activedescendant={active >= 0 ? `res-${flat[active]?.key}` : undefined}
		/>
	</div>

	<p class="visually-hidden" aria-live="polite">
		{#if !loading && results}{totalCount ? `${totalCount} résultats` : 'Aucun résultat'}{/if}
		{#if active >= 0 && flat[active]}{flat[active].title}{/if}
	</p>

	{#if urlQuery.trim().length < 2}
		<EmptyState icon="search" title="Rechercher dans la médiathèque">
			<p>Tapez au moins deux lettres : titres, épisodes, artistes, albums, pistes ou personnes. Les accents sont facultatifs.</p>
			<p class="kbd-hint">Astuce : <kbd>/</kbd> place le curseur dans la recherche, <kbd>↑</kbd> <kbd>↓</kbd> parcourent les résultats.</p>
		</EmptyState>
	{:else if error}
		<EmptyState icon="alert" title="La recherche a échoué" role="alert"><p>{errorMessage(error)}</p></EmptyState>
	{:else if loading && !results}
		<div class="groups" aria-busy="true">
			{#each [0, 1] as g (g)}
				<div class="group">
					<Skeleton width="140px" height="20px" radius="sm" />
					<div class="list">
						{#each [0, 1, 2, 3] as i (i)}<div class="res sk"><Skeleton width="48px" ratio="2 / 3" radius="sm" /><Skeleton lines={2} width="70%" /></div>{/each}
					</div>
				</div>
			{/each}
		</div>
	{:else if results && !flat.length}
		<EmptyState icon="search" title="Aucun résultat pour « {urlQuery.trim()} »" role="status">
			<p>Vérifiez l’orthographe, essayez un autre mot-clé ou le titre original.</p>
		</EmptyState>
	{:else if results}
		<p class="summary" aria-hidden="true">
			{plural(totalCount, 'résultat', 'résultats')} pour <strong>« {urlQuery.trim()} »</strong>
		</p>
		<div class="groups results" id="search-results" class:stale={loading}>
			{#each groups as g (g.key)}
				<section class="group" aria-labelledby="grp-{g.key}">
					<h2 id="grp-{g.key}">{g.label} <span class="count tabular">{g.entries.length}</span></h2>
					<ul class="list" role="list">
						{#each g.entries as e (e.key)}
							{@const i = indexOf(e)}
							<li>
								<a id="res-{e.key}" class="res" class:active={i === active} href={e.href} aria-current={i === active ? 'true' : undefined} onmouseenter={() => (active = i)}>
									{#if e.person}
										<Avatar name={e.person.name} src={e.person.image} size={48} hue={hueFrom(e.person.name)} />
									{:else}
										<span class="thumb {g.key}">
											<Artwork image={thumbFor(e)} ratio={ratioFor(g.key)} sizes="96px" placeholder={{ title: e.title, icon: e.item ? KIND_ICON[e.item.kind] : undefined }} />
										</span>
									{/if}
									<span class="text">
										<span class="title">{e.title}</span>
										{#if e.meta}<span class="meta">{e.meta}</span>{/if}
									</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		</div>
	{/if}
</section>

<style>
	.search {
		display: grid;
		gap: var(--sp-5);
		align-content: start;
	}

	/* The top-bar field drives search on larger screens; phones get their own. */
	.field {
		display: none;
	}

	@media (max-width: 767px) {
		.field {
			display: block;
		}
	}

	.summary {
		color: var(--text-muted);
	}

	.summary strong {
		color: var(--text);
	}

	.kbd-hint {
		margin-top: var(--sp-2);
		font-size: var(--fs-small);
	}

	kbd {
		display: inline-grid;
		place-items: center;
		min-width: 22px;
		height: 22px;
		padding: 0 5px;
		border-radius: var(--r-xs);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		font-family: var(--font-ui);
		font-size: var(--fs-micro);
		color: var(--text);
	}

	.groups {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr));
		gap: var(--sp-6) var(--sp-5);
		align-items: start;
		transition: opacity var(--dur-fast) var(--ease);
	}

	.stale {
		opacity: 0.6;
	}

	.group {
		display: grid;
		gap: var(--sp-3);
		min-width: 0;
	}

	h2 {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}

	.count {
		min-width: 24px;
		height: 22px;
		padding: 0 7px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		color: var(--text-muted);
		font-size: var(--fs-micro);
		line-height: 22px;
		text-align: center;
	}

	.list {
		display: grid;
		gap: 4px;
		padding: var(--sp-2);
		border-radius: var(--r-xl);
		background: var(--tint), var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.res {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		min-height: 64px;
		padding: var(--sp-2);
		border-radius: var(--r-md);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.res.active,
	.res:hover {
		background: var(--surface-active);
	}

	.res.sk {
		pointer-events: none;
	}

	.thumb {
		flex: none;
		width: 44px;
		border-radius: var(--r-xs);
		overflow: hidden;
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.thumb.episode {
		width: 86px;
	}

	.thumb.artist,
	.thumb.album,
	.thumb.track {
		width: 52px;
	}

	.thumb.artist {
		border-radius: 50%;
	}

	.text {
		display: grid;
		gap: 1px;
		min-width: 0;
	}

	.title {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.meta {
		font-size: var(--fs-small);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
