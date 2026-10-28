<script lang="ts">
	// Folder browser (folders only): breadcrumb, parent button, clickable sub-folders.
	// The folder being viewed is the selection (`value`); the roots level selects nothing.
	// Data comes from `load` (GET /fs/browse on the server, GET /agents/{id}/browse on an agent).
	import { onDestroy, tick, untrack, type Snippet } from 'svelte';
	import type { FolderListing } from '$lib/api/types';
	import Button from './Button.svelte';
	import Icon from './Icon.svelte';
	import IconButton from './IconButton.svelte';
	import Skeleton from './Skeleton.svelte';

	interface Props {
		load: (path: string, signal: AbortSignal) => Promise<FolderListing>;
		/** Selected folder (the one being viewed); '' at the roots. */
		value?: string;
		/** Folder to open first ('' = the roots). */
		initialPath?: string;
		/** Name of the top level (list of roots). */
		rootsLabel?: string;
		/** Accessible name of the whole picker. */
		label?: string;
		/** Error → message. */
		errorText?: (err: unknown) => string;
		/** Under the list: receives the selected path (e.g. a "Choose this folder" button). */
		footer?: Snippet<[string]>;
		/** Called whenever the viewed folder changes. */
		onnavigate?: (path: string) => void;
	}

	let {
		load,
		value = $bindable(''),
		initialPath = '',
		rootsLabel = 'Emplacements',
		label = 'Choisir un dossier',
		errorText = () => 'Ce dossier est illisible.',
		footer,
		onnavigate
	}: Props = $props();

	let path = $state('');
	let listing = $state<FolderListing | null>(null);
	let roots = $state<string[] | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let ctrl: AbortController | null = null;
	let listEl = $state<HTMLElement>();
	let announce = $state('');

	onDestroy(() => ctrl?.abort());

	$effect(() => {
		// Reload when the data source changes (another agent), starting at initialPath.
		void load;
		untrack(() => open(initialPath, false));
	});

	async function open(p: string, focusList = true) {
		ctrl?.abort();
		const c = (ctrl = new AbortController());
		loading = true;
		error = null;
		try {
			const res = await load(p, c.signal);
			if (c.signal.aborted) return;
			listing = res;
			path = res.path || p;
			if (!path) roots = res.dirs.map((d) => d.path);
			value = path;
			onnavigate?.(path);
			announce = path ? `${baseName(path)} : ${count(res.dirs.length)}` : `${rootsLabel} : ${count(res.dirs.length)}`;
		} catch (err) {
			if (c.signal.aborted || (err instanceof DOMException && err.name === 'AbortError')) return;
			error = errorText(err);
			path = p;
		} finally {
			if (!c.signal.aborted) loading = false;
		}
		if (focusList) {
			await tick();
			listEl?.querySelector<HTMLElement>('button')?.focus();
		}
	}

	const count = (n: number) => (n === 0 ? 'aucun sous-dossier' : n === 1 ? '1 sous-dossier' : `${n} sous-dossiers`);

	const sep = $derived(path.includes('\\') && !path.includes('/') ? '\\' : '/');

	function baseName(p: string): string {
		const parts = p.split(/[\\/]+/).filter(Boolean);
		return parts.at(-1) ?? p;
	}

	/** Breadcrumb from the root that contains the current folder. */
	const crumbs = $derived.by(() => {
		const out: { label: string; path: string; title?: string }[] = [{ label: rootsLabel, path: '' }];
		if (!path) return out;
		const trim = (r: string) => (r.length > 1 ? r.replace(/[\\/]+$/, '') : r);
		const root = (roots ?? [])
			.filter((r) => path === r || trim(path) === trim(r) || path.startsWith(trim(r) + sep) || (r.endsWith(sep) && path.startsWith(r)))
			.sort((a, b) => b.length - a.length)[0];
		let base = root ? trim(root) : '';
		if (root) out.push({ label: root, path: root, title: root });
		const rest = path.slice(base.length).split(/[\\/]+/).filter(Boolean);
		for (const seg of rest) {
			if (!base) base = path.startsWith('/') ? '/' + seg : seg;
			else base = base.endsWith(sep) ? base + seg : base + sep + seg;
			out.push({ label: seg, path: base });
		}
		return out;
	});

	const parent = $derived(path ? (listing?.parent ?? (crumbs.length > 1 ? crumbs[crumbs.length - 2].path : '')) : null);
</script>

<div class="picker" role="group" aria-label={label}>
	<div class="bar">
		<IconButton
			icon="arrow-left"
			variant="surface"
			size="sm"
			aria-label="Dossier parent"
			disabled={parent === null || loading}
			onclick={() => parent !== null && open(parent)}
		/>
		<nav class="crumbs" aria-label="Chemin">
			<ol role="list">
				{#each crumbs as c, i (c.path + i)}
					<li>
						{#if i > 0}<Icon name="chevron-right" size={14} />{/if}
						{#if i < crumbs.length - 1}
							<button type="button" class="crumb" title={c.title} onclick={() => open(c.path)}>{c.label}</button>
						{:else}
							<span class="crumb current" aria-current="location" title={c.title}>{c.label}</span>
						{/if}
					</li>
				{/each}
			</ol>
		</nav>
	</div>

	<div class="list-wrap" aria-busy={loading}>
		{#if loading && !listing}
			<div class="rows" aria-hidden="true">
				{#each [0, 1, 2, 3] as i (i)}<Skeleton height="44px" radius="md" />{/each}
			</div>
		{:else if error}
			<div class="msg bad" role="alert">
				<Icon name="alert" size={18} />
				<div>
					<p>{error}</p>
					<div class="msg-actions">
						<Button variant="secondary" icon="refresh" onclick={() => open(path)}>Réessayer</Button>
						{#if path}<Button variant="tertiary" onclick={() => open('')}>Revenir à « {rootsLabel} »</Button>{/if}
					</div>
				</div>
			</div>
		{:else if listing}
			{#if listing.dirs.length}
				<ul class="rows" role="list" bind:this={listEl} class:dim={loading}>
					{#each listing.dirs as d (d.path)}
						<li>
							<button type="button" class="row" onclick={() => open(d.path)} title={d.path} disabled={loading}>
								<span class="r-icon" aria-hidden="true"><Icon name={path ? 'folder' : 'server'} size={18} /></span>
								<span class="r-name">{path ? d.name : d.path}</span>
								<Icon name="chevron-right" size={16} />
							</button>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="msg empty" bind:this={listEl}>
					<Icon name="folder" size={18} />
					{path ? 'Aucun sous-dossier ici. Vous pouvez choisir ce dossier.' : 'Aucun emplacement accessible sur cette machine.'}
				</p>
			{/if}
		{/if}
	</div>

	<p class="visually-hidden" role="status" aria-live="polite">{announce}</p>

	{#if footer}<div class="foot">{@render footer(value)}</div>{/if}
</div>

<style>
	.picker {
		display: grid;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		min-width: 0;
	}

	.bar {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		min-width: 0;
	}

	.crumbs {
		flex: 1;
		min-width: 0;
		overflow-x: auto;
		scrollbar-width: none;
	}

	.crumbs::-webkit-scrollbar {
		display: none;
	}

	.crumbs ol {
		display: flex;
		align-items: center;
		gap: 2px;
		width: max-content;
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	.crumbs li {
		display: inline-flex;
		align-items: center;
		gap: 2px;
	}

	.crumb {
		display: inline-block;
		max-width: 22ch;
		padding: 4px 8px;
		border-radius: var(--r-xs);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		vertical-align: middle;
	}

	button.crumb {
		cursor: pointer;
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	button.crumb:hover {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 6%, transparent);
	}

	.crumb.current {
		color: var(--text);
		font-weight: var(--fw-bold);
	}

	.list-wrap {
		max-height: 280px;
		overflow-y: auto;
		margin: 0 calc(-1 * var(--sp-1));
		padding: 0 var(--sp-1);
	}

	.rows {
		display: grid;
		gap: 4px;
		transition: opacity var(--dur-fast) var(--ease);
	}

	.rows.dim {
		opacity: 0.55;
	}

	.row {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		min-height: 44px;
		padding: 0 var(--sp-3) 0 var(--sp-2);
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		text-align: left;
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease);
	}

	.row:hover:not(:disabled) {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		color: var(--text);
	}

	.r-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 30px;
		height: 30px;
		border-radius: var(--r-sm);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.r-name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: var(--fw-medium);
		color: var(--text);
	}

	.msg {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-md);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.msg > :global(svg) {
		flex: none;
		margin-top: 1px;
	}

	.msg.bad {
		background: color-mix(in srgb, var(--danger) 10%, var(--surface));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 35%, transparent);
	}

	.msg.bad > :global(svg) {
		color: var(--danger);
	}

	.msg-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		margin-top: var(--sp-2);
	}

	.msg.empty {
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.foot {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-3);
		padding-top: var(--sp-3);
		box-shadow: 0 -1px 0 var(--line);
	}

	@media (max-width: 767px) {
		.foot :global(.btn) {
			width: 100%;
		}

		.list-wrap {
			max-height: 50vh;
		}
	}
</style>
