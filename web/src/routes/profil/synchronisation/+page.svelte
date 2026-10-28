<script lang="ts">
	// Profil › Synchronisation: the user's own Trakt and Simkl connections, kept live by `sync.status`.
	import { onMount } from 'svelte';
	import { api, errorMessage } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import type { SyncConnection, SyncStatusEvent } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import SyncServiceCard from '$lib/components/sync/SyncServiceCard.svelte';
	import { SYNC_SERVICES } from '$lib/sync';
	import { session } from '$lib/session.svelte';

	let list = $state<SyncConnection[] | null>(null);
	let error = $state<unknown>(null);

	async function load() {
		try {
			const got = await api.mySync();
			// Always trakt then simkl, whatever the server sends.
			list = SYNC_SERVICES.map((s) => got.find((c) => c.service === s)).filter((c): c is SyncConnection => !!c);
			error = null;
		} catch (err) {
			error = err;
		}
	}

	function update(c: SyncConnection) {
		if (!list) return;
		list = list.map((x) => (x.service === c.service ? c : x));
	}

	onMount(() => {
		load();
		// The server finished (or started) a synchronisation, or the connection changed state.
		const off = events.on('sync.status', (ev) => {
			const d = ev.data as SyncStatusEvent | undefined;
			if (!d || !list || (session.user && d.userId !== session.user.id)) return;
			const cur = list.find((c) => c.service === d.service);
			if (!cur) return;
			const statusChanged = d.status && d.status !== cur.status;
			update({ ...cur, status: d.status ?? cur.status, syncing: !!d.syncing, lastError: d.lastError || cur.lastError });
			// Finished, or a state we only know partly (account, dates): read it back.
			if (!d.syncing || statusChanged) refreshAll();
		});
		return off;
	});

	let refreshTimer: ReturnType<typeof setTimeout> | undefined;
	function refreshAll() {
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(async () => {
			try {
				// One read refreshes both services (events for both may arrive together).
				for (const c of await api.mySync()) update(c);
			} catch {
				/* the next event will tell */
			}
		}, 300);
	}
</script>

<svelte:head>
	<title>Synchronisation — Profil — Aether</title>
</svelte:head>

<div class="page sync">
	<header class="head">
		<p class="micro eyebrow">Profil</p>
		<h1>Synchronisation</h1>
		<p class="lead">
			Reliez votre compte Trakt ou Simkl : ce que vous regardez ici y est reporté, et ce que vous marquez là-bas revient dans Aether. Chaque membre du serveur relie son propre compte.
		</p>
	</header>

	{#if error && !list}
		<EmptyState icon="alert" title="Synchronisation indisponible" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !list}
		<div class="cards" aria-busy="true"><Skeleton height="220px" radius="lg" /><Skeleton height="180px" radius="lg" /></div>
	{:else}
		<div class="cards">
			{#each list as c (c.service)}
				<SyncServiceCard connection={c} isAdmin={session.isAdmin} onupdate={update} />
			{/each}
		</div>
		<p class="tip">
			<Icon name="info" size={16} />
			<span>Les titres sont reconnus par leurs identifiants TMDB, IMDb ou TVDB : un titre non identifié ou absent de vos bibliothèques est ignoré. Vos données Aether ne sont jamais effacées par une déconnexion.</span>
		</p>
	{/if}
</div>

<style>
	.sync {
		display: grid;
		gap: var(--gap);
		align-content: start;
		max-width: 980px;
		padding-top: var(--sp-2);
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
		max-width: 68ch;
		color: var(--text-muted);
	}

	.cards {
		display: grid;
		gap: var(--gap);
	}

	.tip {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		max-width: 80ch;
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.tip :global(svg) {
		flex: none;
		margin-top: 2px;
	}
</style>
