<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage } from '$lib/api/client';
	import { libraries, scans } from '$lib/api/stores.svelte';
	import type { Kind, Library, Source } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import AddLibraryDialog from '$lib/components/settings/AddLibraryDialog.svelte';
	import SourceDialog from '$lib/components/settings/SourceDialog.svelte';
	import SourceRow from '$lib/components/sources/SourceRow.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { formatDate, plural } from '$lib/format';
	import { KIND_LABEL, libraryKindInfo } from '$lib/media';
	import { toasts } from '$lib/toast.svelte';

	let addOpen = $state(false);
	let toDelete = $state<Library | null>(null);
	let deleteOpen = $state(false);
	let deleting = $state(false);

	// Sources (M5b)
	let srcLib = $state<Library | null>(null);
	let srcEdit = $state<Source | null>(null);
	let srcOpen = $state(false);
	let srcDelete = $state<{ lib: Library; src: Source } | null>(null);
	let srcDeleteOpen = $state(false);
	let srcDeleting = $state(false);
	/** Clock for "vue il y a…" (relative times stay fresh). */
	let now = $state(Date.now());

	onMount(() => {
		libraries.ensure();
		scans.ensure();
		const clock = setInterval(() => (now = Date.now()), 30_000);
		if (page.url.searchParams.has('ajouter')) {
			addOpen = true;
			const url = new URL(page.url);
			url.searchParams.delete('ajouter');
			goto(url, { replaceState: true, keepFocus: true, noScroll: true });
		}
		return () => clearInterval(clock);
	});

	function addSource(lib: Library) {
		srcLib = lib;
		srcEdit = null;
		srcOpen = true;
	}

	function editSource(lib: Library, src: Source) {
		srcLib = lib;
		srcEdit = src;
		srcOpen = true;
	}

	function sourceSaved(src: Source, created: boolean) {
		if (!srcLib) return;
		libraries.upsertSource(srcLib.id, src);
		if (created) {
			scans.queued(srcLib.id);
			toasts.show(`Source « ${src.name} » ajoutée`, { detail: 'L’analyse a commencé.' });
		} else toasts.show(`Source « ${src.name} » mise à jour`);
	}

	function askDeleteSource(lib: Library, src: Source) {
		srcDelete = { lib, src };
		srcDeleteOpen = true;
	}

	async function confirmDeleteSource() {
		if (!srcDelete) return;
		const { lib, src } = srcDelete;
		srcDeleting = true;
		try {
			await api.deleteSource(lib.id, src.id);
			srcDeleteOpen = false;
			libraries.removeSource(lib.id, src.id);
			toasts.show(`Source « ${src.name} » supprimée`, { detail: 'Ses titres disparaîtront au prochain scan.' });
		} catch (err) {
			toasts.show('Suppression impossible', { detail: errorMessage(err) });
		} finally {
			srcDeleting = false;
		}
	}

	function offlineCount(lib: Library) {
		return lib.sources.filter((s) => s.status === 'offline').length;
	}

	const ORDER: Kind[] = ['movie', 'series', 'season', 'episode', 'artist', 'album', 'track'];

	function counts(lib: Library) {
		return ORDER.filter((k) => lib.counts?.[k]).map((k) => plural(lib.counts[k], KIND_LABEL[k].one, KIND_LABEL[k].many));
	}

	async function scan(lib: Library) {
		scans.queued(lib.id);
		try {
			await api.scanLibrary(lib.id);
			toasts.show(`Analyse de « ${lib.name} » lancée`);
		} catch (err) {
			scans.byLibrary[lib.id] = { running: false, processed: 0, total: 0 };
			toasts.show('Analyse impossible', { detail: errorMessage(err) });
		}
	}

	function askDelete(lib: Library) {
		toDelete = lib;
		deleteOpen = true;
	}

	async function confirmDelete() {
		if (!toDelete) return;
		const lib = toDelete;
		deleting = true;
		try {
			await api.deleteLibrary(lib.id);
			deleteOpen = false;
			toasts.show(`Bibliothèque « ${lib.name} » supprimée`, { detail: 'Vos fichiers n’ont pas été touchés.' });
			await libraries.refresh();
		} catch (err) {
			toasts.show('Suppression impossible', { detail: errorMessage(err) });
		} finally {
			deleting = false;
		}
	}

	async function created(lib: Library) {
		scans.queued(lib.id);
		toasts.show(`Bibliothèque « ${lib.name} » créée`, { detail: 'L’analyse a commencé.' });
		await libraries.refresh();
	}

	function pct(lib: Library) {
		const s = scans.byLibrary[lib.id];
		if (!s?.total) return undefined;
		return Math.round((s.processed / s.total) * 100);
	}
</script>

<svelte:head>
	<title>Bibliothèques — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<div>
			<p class="micro eyebrow">Réglages</p>
			<h1>Bibliothèques</h1>
			<p class="lead">Les sources qu’Aether analyse et surveille : dossiers du serveur, machines avec l’agent Aether, seedbox SFTP. Supprimer une bibliothèque ne supprime aucun fichier.</p>
		</div>
		<Button variant="primary" icon="plus" onclick={() => (addOpen = true)}>Ajouter</Button>
	</header>

	{#if !libraries.loaded}
		<div class="list">{#each [0, 1] as i (i)}<Skeleton height="148px" radius="lg" />{/each}</div>
	{:else if libraries.error && !libraries.list.length}
		<EmptyState icon="alert" title="Bibliothèques indisponibles" role="alert">
			<p>{errorMessage(libraries.error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={() => libraries.refresh()}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !libraries.list.length}
		<EmptyState icon="layers" title="Aucune bibliothèque">
			<p>Ajoutez un dossier de films, de séries ou de musique : l’analyse démarre aussitôt.</p>
			{#snippet actions()}<Button variant="primary" icon="plus" onclick={() => (addOpen = true)}>Ajouter une bibliothèque</Button>{/snippet}
		</EmptyState>
	{:else}
		<ul class="list" role="list">
			{#each libraries.list as lib (lib.id)}
				{@const info = libraryKindInfo(lib.kind)}
				{@const s = scans.byLibrary[lib.id]}
				{@const p = pct(lib)}
				<li class="lib panel" aria-labelledby="lib-{lib.id}">
					<div class="top">
						<span class="icon" aria-hidden="true"><Icon name={info.icon} size={22} /></span>
						<div class="title">
							<h2 id="lib-{lib.id}"><a href="/bibliotheque/{lib.id}">{lib.name}</a></h2>
							<p class="meta">
								{info.label} · {plural(lib.sources.length, 'source', 'sources')}{#if offlineCount(lib)}<span class="off">{' · '}{plural(offlineCount(lib), 'hors ligne', 'hors ligne')}</span>{/if} · créée le {formatDate(lib.createdAt)}
							</p>
						</div>
						<div class="actions">
							<Button icon="refresh" disabled={s?.running} onclick={() => scan(lib)} aria-label="Analyser {lib.name}">
								{s?.running ? 'Analyse…' : 'Analyser'}
							</Button>
							<Button variant="tertiary" icon="trash" onclick={() => askDelete(lib)} aria-label="Supprimer {lib.name}" />
						</div>
					</div>

					<div class="sources">
						<ul class="src-list" role="list" aria-label="Sources de {lib.name}">
							{#each lib.sources as src (src.id)}
								<li><SourceRow source={src} {now} onedit={(s) => editSource(lib, s)} ondelete={(s) => askDeleteSource(lib, s)} /></li>
							{/each}
						</ul>
						<div><Button variant="tertiary" icon="plus" onclick={() => addSource(lib)} aria-label="Ajouter une source à {lib.name}">Ajouter une source</Button></div>
					</div>

					<div class="foot">
						{#if s?.running}
							<div class="progress" role="progressbar" aria-label="Analyse de {lib.name}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={p} aria-valuetext={p === undefined ? 'En attente' : `${p} %`}>
								<span class="bar" class:indeterminate={p === undefined}><span style:transform={p === undefined ? undefined : `scaleX(${p / 100})`}></span></span>
								<span class="ptext tabular">{p === undefined ? 'En attente…' : `${s.processed} / ${s.total} fichiers`}</span>
							</div>
						{:else}
							<p class="counts">{counts(lib).join(' · ') || 'Aucun titre pour l’instant'}</p>
							{#if s?.finishedAt}<p class="done micro" role="status"><Icon name="check" size={14} />Analyse terminée{s.added ? ` · ${plural(s.added, 'nouveau fichier', 'nouveaux fichiers')}` : ''}</p>{/if}
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<AddLibraryDialog bind:open={addOpen} oncreated={created} />

<SourceDialog bind:open={srcOpen} library={srcLib} source={srcEdit} onsaved={sourceSaved} />

<ConfirmDialog bind:open={srcDeleteOpen} title="Supprimer la source ?" confirmLabel="Supprimer la source" danger busy={srcDeleting} onconfirm={confirmDeleteSource}>
	<p>
		« <strong>{srcDelete?.src.name}</strong> » sera retirée de « {srcDelete?.lib.name} ». Les titres provenant de cette source
		disparaîtront de la bibliothèque au prochain scan. Les fichiers eux-mêmes ne sont pas modifiés.
	</p>
</ConfirmDialog>

<ConfirmDialog bind:open={deleteOpen} title="Supprimer la bibliothèque ?" confirmLabel="Supprimer" danger busy={deleting} onconfirm={confirmDelete}>
	<p>
		« <strong>{toDelete?.name}</strong> » et ses fiches seront retirées d’Aether. Les fichiers sur le disque ne sont pas modifiés ; vous
		pourrez recréer la bibliothèque à tout moment.
	</p>
</ConfirmDialog>

<style>
	.settings {
		display: grid;
		gap: var(--sp-5);
		align-content: start;
		max-width: 1100px;
	}

	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		flex-wrap: wrap;
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.lead {
		margin-top: var(--sp-1);
		color: var(--text-muted);
		max-width: 60ch;
	}

	.list {
		display: grid;
		gap: var(--gap);
	}

	.lib {
		display: grid;
		gap: var(--sp-4);
		padding: var(--sp-5);
	}

	.top {
		display: flex;
		align-items: center;
		gap: var(--sp-4);
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

	.title {
		flex: 1;
		min-width: 0;
	}

	h2 a:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.meta {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.actions {
		display: flex;
		gap: var(--sp-2);
	}

	.sources {
		display: grid;
		gap: var(--sp-2);
	}

	.src-list {
		display: grid;
		gap: var(--sp-2);
	}

	.off {
		color: var(--danger);
	}

	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
		flex-wrap: wrap;
		min-height: 24px;
		padding-top: var(--sp-3);
		box-shadow: 0 -1px 0 var(--line);
	}

	.counts {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.done {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		color: var(--success);
	}

	.progress {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
	}

	.bar {
		position: relative;
		flex: 1;
		height: 6px;
		border-radius: var(--r-full);
		background: color-mix(in srgb, var(--text) 10%, transparent);
		overflow: hidden;
	}

	.bar span {
		display: block;
		height: 100%;
		border-radius: inherit;
		background: var(--accent);
		transform-origin: left;
		transition: transform var(--dur-base) var(--ease);
	}

	.bar.indeterminate span {
		width: 30%;
		animation: slide 1.4s var(--ease) infinite;
	}

	@keyframes slide {
		from {
			transform: translateX(-100%);
		}
		to {
			transform: translateX(340%);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.bar.indeterminate span {
			width: 100%;
			opacity: 0.5;
			animation: none;
		}
	}

	.ptext {
		flex: none;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	@media (max-width: 767px) {
		.lib {
			padding: var(--sp-4);
		}

		.top {
			flex-wrap: wrap;
		}

		.actions {
			width: 100%;
		}

		.actions :global(.btn:first-child) {
			flex: 1;
		}

		.head :global(.btn) {
			width: 100%;
		}
	}
</style>
