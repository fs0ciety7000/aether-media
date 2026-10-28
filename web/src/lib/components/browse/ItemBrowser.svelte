<script lang="ts" module>
	import type { SortKey } from '$lib/api/types';

	export const SORTS: { value: SortKey; label: string }[] = [
		{ value: 'title', label: 'Titre' },
		{ value: 'dateAdded', label: 'Date d’ajout' },
		{ value: 'year', label: 'Année' },
		{ value: 'premiereDate', label: 'Date de sortie' },
		{ value: 'rating', label: 'Note' },
		{ value: 'runtime', label: 'Durée' },
		{ value: 'lastPlayed', label: 'Vu récemment' },
		{ value: 'random', label: 'Aléatoire' }
	];

	/** Watch-state filters (M6): URL `vu` → API params. */
	export const WATCH_FILTERS = [
		{ value: 'non', label: 'Non vus', query: { played: false } },
		{ value: 'encours', label: 'En cours', query: { inProgress: true } },
		{ value: 'oui', label: 'Vus', query: { played: true } }
	] as const;
</script>

<script lang="ts">
	// Poster grid with a sticky toolbar (sort, order, filter chips) synced to the URL query,
	// total count, cursor-based infinite scroll and skeletons.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import type { Item, ItemQuery, Kind, Library, LibraryFilters, Resolution, SortOrder } from '$lib/api/types';
	import { plural } from '$lib/format';
	import { KIND_LABEL } from '$lib/media';
	import Button from '../Button.svelte';
	import Chip from '../Chip.svelte';
	import EmptyState from '../EmptyState.svelte';
	import IconButton from '../IconButton.svelte';
	import ItemCard from '../ItemCard.svelte';
	import Select from '../Select.svelte';
	import Skeleton from '../Skeleton.svelte';
	import { session } from '$lib/session.svelte';

	interface Props {
		title: string;
		eyebrow?: string;
		/** Library being browsed (kind, filters); cross-library browsing otherwise. */
		library?: Library;
		/** Fixed constraints (e.g. a person). */
		base?: Pick<ItemQuery, 'person'>;
	}

	let { title, eyebrow, library, base = {} }: Props = $props();

	const PAGE = 60;
	const RESOLUTIONS: Resolution[] = ['4K', '1080p', '720p', 'SD'];

	// ——— URL state ———
	const sp = $derived(page.url.searchParams);
	const sort = $derived((SORTS.find((s) => s.value === sp.get('sort'))?.value ?? 'title') as SortKey);
	const defaultOrder = $derived<SortOrder>(sort === 'title' ? 'asc' : 'desc');
	const order = $derived<SortOrder>(sp.get('order') === 'asc' ? 'asc' : sp.get('order') === 'desc' ? 'desc' : defaultOrder);
	const genre = $derived(sp.get('genre') ?? undefined);
	const resolution = $derived((RESOLUTIONS as string[]).includes(sp.get('res') ?? '') ? (sp.get('res') as Resolution) : undefined);
	const hdr = $derived(sp.get('hdr') === '1');
	const yearFrom = $derived(num(sp.get('de')));
	const yearTo = $derived(num(sp.get('a')));
	const view = $derived(sp.get('vue') === 'albums' ? 'albums' : 'artistes');
	const watch = $derived(WATCH_FILTERS.find((w) => w.value === sp.get('vu')));
	const favorite = $derived(sp.get('favoris') === '1');
	const inList = $derived(sp.get('liste') === '1');
	const isMusic = $derived(library?.kind === 'music');
	const isVideo = $derived(!isMusic);

	function num(v: string | null): number | undefined {
		const n = v ? parseInt(v, 10) : NaN;
		return Number.isFinite(n) ? n : undefined;
	}

	const kinds = $derived.by<Kind[] | undefined>(() => {
		switch (library?.kind) {
			case 'movies':
				return ['movie'];
			case 'shows':
				return ['series'];
			case 'music':
				return view === 'albums' ? ['album'] : ['artist'];
			default:
				return undefined;
		}
	});

	const query = $derived<ItemQuery>({
		library: library?.id,
		person: base.person,
		kind: kinds,
		genre,
		resolution,
		hdr: hdr || undefined,
		yearFrom,
		yearTo,
		...(isVideo ? watch?.query : undefined),
		favorite: favorite || undefined,
		watchlist: (isVideo && inList) || undefined,
		sort,
		order: order === defaultOrder ? undefined : order,
		limit: PAGE
	});
	const queryKey = $derived(JSON.stringify(query));
	const activeFilters = $derived(!!(genre || resolution || hdr || yearFrom || yearTo || watch || favorite || inList));

	function update(changes: Record<string, string | undefined>) {
		const url = new URL(page.url);
		for (const [k, v] of Object.entries(changes)) {
			if (v === undefined || v === '') url.searchParams.delete(k);
			else url.searchParams.set(k, v);
		}
		goto(url, { replaceState: true, keepFocus: true, noScroll: true });
	}

	function clearFilters() {
		update({ genre: undefined, res: undefined, hdr: undefined, de: undefined, a: undefined, vu: undefined, favoris: undefined, liste: undefined });
	}

	// ——— Filters available in this library ———
	let filters = $state<LibraryFilters | null>(null);
	$effect(() => {
		const id = library?.id;
		filters = null;
		if (!id) return;
		const ctrl = new AbortController();
		api
			.filters(id, ctrl.signal)
			.then((f) => (filters = f))
			.catch(() => {});
		return () => ctrl.abort();
	});

	const genres = $derived.by(() => {
		const list = filters?.genres.map((g) => ({ name: g.name, count: g.count as number | undefined })) ?? [];
		// Keep a genre from the URL visible even when this library does not list it.
		if (genre && !list.some((g) => g.name === genre)) list.unshift({ name: genre, count: undefined });
		return list;
	});

	// ——— Data ———
	let items = $state<Item[]>([]);
	let total = $state<number | undefined>();
	let cursor = $state<string | undefined>();
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state<unknown>(null);
	let pages = 0;
	let ctrl: AbortController | undefined;

	async function load(silent = false) {
		ctrl?.abort();
		const c = (ctrl = new AbortController());
		const q = JSON.parse(queryKey) as ItemQuery;
		if (!silent) {
			loading = true;
			items = [];
			total = undefined;
		}
		error = null;
		try {
			const res = await api.items(q, c.signal);
			items = res.items;
			total = res.total;
			cursor = res.nextCursor;
			pages = 1;
		} catch (err) {
			if (!isAbort(err)) error = err;
		} finally {
			if (ctrl === c) loading = false;
		}
	}

	async function loadMore() {
		if (!cursor || loadingMore || loading) return;
		loadingMore = true;
		const c = ctrl!;
		try {
			const res = await api.items({ ...(JSON.parse(queryKey) as ItemQuery), cursor }, c.signal);
			const seen = new Set(items.map((i) => i.id));
			items = [...items, ...res.items.filter((i) => !seen.has(i.id))];
			cursor = res.nextCursor;
			total = res.total ?? total;
			pages++;
		} catch (err) {
			if (!isAbort(err)) error = err;
		} finally {
			loadingMore = false;
		}
	}

	$effect(() => {
		void queryKey;
		load();
		return () => ctrl?.abort();
	});

	// The user's data changed elsewhere (Trakt / Simkl import, another device) while a watch filter
	// is on: the set of matching titles may have moved.
	$effect(() => {
		const filtered = !!(watch || favorite || inList);
		if (!filtered) return;
		return events.debounced(
			'userdata.changed',
			() => {
				if (pages <= 1) load(true);
			},
			1500,
			(ev) => {
				const uid = (ev.data as { userId?: number } | undefined)?.userId;
				return !session.user || uid === session.user.id;
			}
		);
	});

	// Fresh scan or metadata: reload quietly while the user is still on the first page.
	$effect(() => {
		const id = library?.id;
		return events.debounced(
			['library.scan.finished', 'item.updated', 'item.image', 'source.status'],
			() => {
				if (pages <= 1) load(true);
			},
			1500,
			(ev) => !id || (ev.data as { libraryId?: number } | undefined)?.libraryId === undefined || (ev.data as { libraryId?: number }).libraryId === id
		);
	});

	// Infinite scroll.
	let sentinel = $state<HTMLElement>();
	$effect(() => {
		if (!sentinel) return;
		const io = new IntersectionObserver((entries) => entries.some((e) => e.isIntersecting) && loadMore(), { rootMargin: '900px 0px' });
		io.observe(sentinel);
		return () => io.disconnect();
	});

	const nounKind = $derived<Kind>(kinds?.length === 1 ? kinds[0] : 'movie');
	const countLabel = $derived.by(() => {
		if (total === undefined) return undefined;
		if (kinds?.length === 1) return plural(total, KIND_LABEL[nounKind].one, KIND_LABEL[nounKind].many);
		return plural(total, 'titre', 'titres');
	});

	// Year inputs are applied on change (Enter or blur), not on every keystroke.
	function onYear(key: 'de' | 'a', e: Event) {
		const v = (e.currentTarget as HTMLInputElement).value.trim();
		update({ [key]: v && /^\d{4}$/.test(v) ? v : undefined });
	}

	const sortOptions = $derived(SORTS.filter((s) => isVideo || s.value !== 'runtime'));
	const sizes = '(max-width: 479px) 45vw, (max-width: 1023px) 30vw, 180px';
</script>

<div class="browser">
	<header class="head">
		<div class="titles">
			{#if eyebrow}<p class="micro eyebrow">{eyebrow}</p>{/if}
			<h1>{title}</h1>
		</div>
		<p class="count tabular" aria-live="polite">
			{#if countLabel}{countLabel}{:else if loading}<span class="visually-hidden">Chargement…</span>{/if}
		</p>
	</header>

	<div class="toolbar">
		<div class="line">
			{#if isMusic}
				<div class="segmented" role="group" aria-label="Affichage">
					<button type="button" aria-pressed={view === 'artistes'} onclick={() => update({ vue: undefined })}>Artistes</button>
					<button type="button" aria-pressed={view === 'albums'} onclick={() => update({ vue: 'albums' })}>Albums</button>
				</div>
			{/if}
			<div class="chips" role="group" aria-label="Genres">
				{#if genres.length}
					<Chip toggle={false} selected={!genre} onclick={() => update({ genre: undefined })}>Tous</Chip>
					{#each genres as g (g.name)}
						<Chip toggle={false} selected={genre === g.name} count={g.count} onclick={() => update({ genre: genre === g.name ? undefined : g.name })}>
							{g.name}
						</Chip>
					{/each}
				{/if}
			</div>
			<div class="sorting">
				<Select
					label="Trier par"
					value={sort}
					options={sortOptions}
					onchange={(e) => update({ sort: e.currentTarget.value === 'title' ? undefined : e.currentTarget.value, order: undefined })}
				/>
				<IconButton
					icon={order === 'asc' ? 'sort-asc' : 'sort-desc'}
					size="sm"
					aria-label={order === 'asc' ? 'Ordre croissant (inverser)' : 'Ordre décroissant (inverser)'}
					title={order === 'asc' ? 'Croissant' : 'Décroissant'}
					disabled={sort === 'random'}
					onclick={() => update({ order: order === 'asc' ? 'desc' : 'asc' })}
				/>
				{#if sort === 'random'}
					<IconButton icon="refresh" size="sm" aria-label="Nouveau tirage" onclick={() => load()} />
				{/if}
			</div>
		</div>

		{#if isVideo || filters?.years || activeFilters}
			<div class="line secondary" role="group" aria-label="Autres filtres">
				{#if isVideo}
					<span class="watch" role="group" aria-label="État de visionnage">
						{#each WATCH_FILTERS as w (w.value)}
							<Chip toggle={false} selected={watch?.value === w.value} onclick={() => update({ vu: watch?.value === w.value ? undefined : w.value })}>{w.label}</Chip>
						{/each}
						<Chip toggle={false} selected={favorite} onclick={() => update({ favoris: favorite ? undefined : '1' })}>Favoris</Chip>
						<Chip toggle={false} selected={inList} onclick={() => update({ liste: inList ? undefined : '1' })}>Ma liste</Chip>
					</span>
				{/if}
				{#if filters?.years}
					<span class="years">
						<span class="micro years-label" aria-hidden="true">Années</span>
						<label class="visually-hidden" for="year-from">Année minimale</label>
						<input
							id="year-from"
							type="number"
							inputmode="numeric"
							placeholder={String(filters.years.min)}
							min={filters.years.min}
							max={filters.years.max}
							value={yearFrom ?? ''}
							onchange={(e) => onYear('de', e)}
						/>
						<span aria-hidden="true">–</span>
						<label class="visually-hidden" for="year-to">Année maximale</label>
						<input
							id="year-to"
							type="number"
							inputmode="numeric"
							placeholder={String(filters.years.max)}
							min={filters.years.min}
							max={filters.years.max}
							value={yearTo ?? ''}
							onchange={(e) => onYear('a', e)}
						/>
					</span>
				{/if}
				{#if isVideo && filters}
					{#each RESOLUTIONS.filter((r) => filters?.resolutions.includes(r) || resolution === r) as r (r)}
						<Chip toggle={false} selected={resolution === r} onclick={() => update({ res: resolution === r ? undefined : r })}>{r}</Chip>
					{/each}
					{#if filters.hasHdr || hdr}
						<Chip toggle={false} selected={hdr} onclick={() => update({ hdr: hdr ? undefined : '1' })}>HDR</Chip>
					{/if}
				{/if}
				{#if activeFilters}
					<Button variant="tertiary" icon="x" onclick={clearFilters}>Effacer les filtres</Button>
				{/if}
			</div>
		{/if}
	</div>

	{#if error && !items.length}
		<EmptyState icon="alert" title="Impossible de charger cette bibliothèque" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={() => load()}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !loading && !items.length}
		{#if activeFilters}
			<EmptyState icon="sliders" title="Aucun titre ne correspond" role="status">
				<p>
					{#if watch?.value === 'encours'}Rien en cours de lecture ici pour l’instant.{:else if favorite}Aucun favori ici : touchez le cœur sur une fiche pour en ajouter.{:else if watch?.value === 'non'}Vous avez tout vu ici. Bravo !{:else}Essayez d’élargir la période ou de retirer un filtre.{/if}
				</p>
				{#snippet actions()}<Button icon="x" onclick={clearFilters}>Effacer les filtres</Button>{/snippet}
			</EmptyState>
		{:else}
			<EmptyState icon="folder" title="Cette bibliothèque est vide" role="status">
				{#if session.isAdmin}
					<p>Lancez une analyse depuis les réglages, ou vérifiez que les dossiers contiennent des médias reconnus.</p>
				{:else}
					<p>Aucun titre disponible pour l’instant.</p>
				{/if}
				{#snippet actions()}{#if session.isAdmin}<Button href="/reglages/bibliotheques">Gérer les bibliothèques</Button>{/if}{/snippet}
			</EmptyState>
		{/if}
	{:else}
		<ul class="grid" class:square={isMusic} role="list" aria-busy={loading || loadingMore}>
			{#each items as it (it.id)}
				<li><ItemCard item={it} {sizes} /></li>
			{/each}
			{#if loading || loadingMore}
				{#each Array.from({ length: loading ? 14 : 7 }, (_, i) => i) as i (i)}
					<li class="sk" aria-hidden="true">
						<Skeleton ratio={isMusic ? '1 / 1' : '2 / 3'} radius={isMusic && view === 'artistes' ? 'full' : 'lg'} />
						<Skeleton width="70%" height="12px" radius="sm" />
					</li>
				{/each}
			{/if}
		</ul>
		{#if error}<p class="more-error" role="alert">{errorMessage(error)} <button type="button" onclick={loadMore}>Réessayer</button></p>{/if}
		<div class="sentinel" bind:this={sentinel} aria-hidden="true"></div>
	{/if}
</div>

<style>
	.browser {
		padding: var(--sp-2) var(--gutter) var(--sp-9);
	}

	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		margin-bottom: var(--sp-3);
	}

	.titles {
		min-width: 0;
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		margin-bottom: 2px;
	}

	h1 {
		overflow-wrap: anywhere;
	}

	.count {
		flex: none;
		padding-bottom: 6px;
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
	}

	/* Sticky under the top bar; opaque so posters slide beneath it cleanly. */
	.toolbar {
		position: sticky;
		top: calc(var(--header-h) + env(safe-area-inset-top, 0px));
		z-index: 10;
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: 10px;
		margin: 0 calc(-1 * var(--gutter)) var(--sp-5);
		padding: var(--sp-3) var(--gutter);
		background: var(--bg);
		box-shadow: 0 1px 0 var(--line);
	}

	.line {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		min-width: 0;
	}

	.secondary {
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	.sorting {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		flex: none;
		margin-left: auto;
	}

	.sorting :global(.select) {
		width: 180px;
	}

	.segmented {
		display: inline-flex;
		flex: none;
		padding: 3px;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.segmented button {
		height: 32px;
		padding: 0 var(--sp-4);
		border-radius: var(--r-full);
		color: var(--text-muted);
		font-weight: var(--fw-bold);
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.segmented button:hover {
		color: var(--text);
	}

	.segmented button[aria-pressed='true'] {
		background: var(--surface-active);
		color: var(--text);
	}

	/* Genre chips scroll horizontally, fading out at the edge. */
	.chips {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		flex: 1;
		min-width: 0;
		padding: 2px;
		overflow-x: auto;
		scrollbar-width: none;
		mask-image: linear-gradient(to right, #000 calc(100% - 32px), transparent);
	}

	.chips::-webkit-scrollbar {
		display: none;
	}

	.chips > :global(*) {
		flex: none;
	}

	.watch {
		display: inline-flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		padding-right: var(--sp-3);
		margin-right: var(--sp-1);
		box-shadow: 1px 0 0 var(--line);
	}

	.years {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		height: 34px;
		padding: 0 6px 0 14px;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
	}

	.years-label {
		margin-right: 4px;
	}

	.years input {
		width: 56px;
		height: 26px;
		border: 0;
		border-radius: var(--r-full);
		background: transparent;
		text-align: center;
		font-size: var(--fs-small);
		font-variant-numeric: tabular-nums;
		-moz-appearance: textfield;
		appearance: textfield;
	}

	.years input::placeholder {
		color: var(--text-faint);
	}

	.years input::-webkit-inner-spin-button,
	.years input::-webkit-outer-spin-button {
		-webkit-appearance: none;
		margin: 0;
	}

	.years input:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 0;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: var(--sp-5) var(--gap);
	}

	@media (min-width: 1600px) {
		.grid {
			grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
		}
	}

	.sk {
		display: grid;
		gap: 10px;
		align-content: start;
	}

	.sentinel {
		height: 1px;
	}

	.more-error {
		margin-top: var(--sp-5);
		color: var(--danger);
		text-align: center;
	}

	.more-error button {
		color: var(--accent-ink);
		text-decoration: underline;
	}

	@media (max-width: 767px) {
		.toolbar {
			top: 64px;
		}

		.line:first-child {
			flex-wrap: wrap;
		}

		.chips {
			order: 3;
			flex-basis: 100%;
			margin: 0 calc(-1 * var(--gutter));
			padding: 2px var(--gutter);
			mask-image: none;
		}

		.sorting {
			flex: 1;
		}

		.sorting :global(.select) {
			flex: 1;
			width: auto;
		}

		.segmented {
			flex-basis: 100%;
		}

		.segmented button {
			flex: 1;
		}
	}

	@media (max-width: 479px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
