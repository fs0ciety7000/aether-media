<script lang="ts">
	// Réglages › Journaux (admins): the last ~2 000 server log records kept in memory, filtered by
	// level and text, followed live (admin SSE stream) and downloadable. Secrets are masked server-side.
	import { onMount } from 'svelte';
	import { api, errorMessage, logsUrl } from '$lib/api/client';
	import type { LogEntry } from '$lib/api/types';
	import { LOG_LEVELS } from '$lib/admin';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import SearchField from '$lib/components/SearchField.svelte';
	import SegmentedControl from '$lib/components/SegmentedControl.svelte';
	import Switch from '$lib/components/Switch.svelte';
	import LogViewer from '$lib/components/admin/LogViewer.svelte';
	import { plural } from '$lib/format';

	const MAX = 2000;

	let level = $state('info');
	let q = $state('');
	let applied = $state('');
	let follow = $state(true);
	let entries = $state<LogEntry[]>([]);
	let held = $state(0);
	let capacity = $state(MAX);
	let error = $state<unknown>(null);
	let loading = $state(true);
	let live = $state<'off' | 'connecting' | 'open'>('off');

	let source: EventSource | null = null;
	let ctrl: AbortController | null = null;
	let qTimer: ReturnType<typeof setTimeout> | undefined;

	async function load() {
		ctrl?.abort();
		ctrl = new AbortController();
		loading = true;
		try {
			const page = await api.logs({ level, q: applied, limit: MAX }, ctrl.signal);
			entries = page.entries;
			held = page.held;
			capacity = page.capacity;
			error = null;
		} catch (err) {
			if (!(err instanceof DOMException)) error = err;
		} finally {
			loading = false;
		}
	}

	function disconnect() {
		source?.close();
		source = null;
		live = 'off';
	}

	function connect() {
		disconnect();
		if (!follow || typeof EventSource === 'undefined') return;
		live = 'connecting';
		const src = new EventSource(logsUrl('stream', level, applied));
		source = src;
		src.onopen = () => (live = 'open');
		src.addEventListener('log', (e) => {
			try {
				const entry = JSON.parse((e as MessageEvent).data) as LogEntry;
				if (entries.length && entry.seq <= entries[entries.length - 1].seq) return;
				entries = entries.length >= MAX ? [...entries.slice(entries.length - MAX + 1), entry] : [...entries, entry];
				held = Math.min(capacity, held + 1);
			} catch {
				/* ignore */
			}
		});
		src.onerror = () => {
			live = src.readyState === EventSource.CLOSED ? 'off' : 'connecting';
		};
	}

	async function refresh() {
		await load();
		connect();
	}

	onMount(() => {
		refresh();
		return () => {
			disconnect();
			ctrl?.abort();
			clearTimeout(qTimer);
		};
	});

	function onLevel() {
		refresh();
	}

	// Search: debounced, then reload + reconnect with the new filter.
	$effect(() => {
		const v = q;
		clearTimeout(qTimer);
		qTimer = setTimeout(() => {
			if (v.trim() === applied) return;
			applied = v.trim();
			refresh();
		}, 350);
	});

	function onFollow(on: boolean) {
		if (on) refresh();
		else disconnect();
	}

	const download = $derived(logsUrl('download', level, applied));
</script>

<svelte:head>
	<title>Journaux — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<p class="micro eyebrow">Réglages</p>
		<h1>Journaux</h1>
		<p class="lead">
			Les {capacity.toLocaleString('fr-FR')} derniers messages du serveur, gardés en mémoire depuis le démarrage. Jetons, clés et mots de passe y sont masqués.
		</p>
	</header>

	<div class="toolbar panel">
		<SegmentedControl options={LOG_LEVELS} bind:value={level} label="Niveau minimal" onchange={onLevel} />
		<div class="search"><SearchField bind:value={q} label="Rechercher dans les journaux" placeholder="Rechercher (message, chemin, identifiant…)" /></div>
		<div class="right">
			<div class="follow">
				<Switch bind:checked={follow} label="Suivi en direct" onchange={onFollow} />
				<span class={['pulse', live]} aria-hidden="true"></span>
				<span class="visually-hidden" aria-live="polite">{live === 'open' ? 'Suivi en direct actif' : follow ? 'Connexion au flux…' : 'Suivi en direct arrêté'}</span>
			</div>
			{#if !follow}<Button variant="tertiary" icon="refresh" onclick={load}>Actualiser</Button>{/if}
			<Button href={download} icon="download" download>Télécharger</Button>
		</div>
	</div>

	{#if error && !entries.length}
		<EmptyState icon="alert" title="Journaux indisponibles" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={refresh}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else}
		<div aria-busy={loading}>
			<LogViewer {entries} {follow} highlight={applied} />
		</div>
		<p class="count micro">
			<Icon name="terminal" size={14} />
			{plural(entries.length, 'entrée affichée', 'entrées affichées')} · {held.toLocaleString('fr-FR')} en mémoire sur {capacity.toLocaleString('fr-FR')}
		</p>
	{/if}
</section>

<style>
	.settings {
		display: grid;
		gap: var(--gap);
		align-content: start;
		max-width: 1280px;
	}

	.head {
		display: grid;
		gap: var(--sp-1);
		margin-bottom: var(--sp-2);
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.lead {
		max-width: 76ch;
		color: var(--text-muted);
	}

	.toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
	}

	.search {
		flex: 1 1 260px;
		min-width: 0;
	}

	.right {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
		margin-left: auto;
	}

	.follow {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}

	.pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--text-faint);
	}

	.pulse.open {
		background: var(--accent);
		animation: pulse 1.6s var(--ease) infinite alternate;
	}

	.pulse.connecting {
		background: color-mix(in srgb, var(--accent) 45%, transparent);
	}

	@keyframes pulse {
		from {
			opacity: 0.35;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.pulse.open {
			animation: none;
		}
	}

	.count {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--text-muted);
	}

	@media (max-width: 767px) {
		.right {
			margin-left: 0;
			width: 100%;
			justify-content: space-between;
		}
	}
</style>
