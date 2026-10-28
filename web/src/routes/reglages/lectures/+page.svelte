<script lang="ts">
	// Réglages › Lectures (admins): playback sessions in progress, refreshed on the admin-only
	// "playback.sessions" event and every few seconds (positions move). An admin can stop one.
	import { onMount } from 'svelte';
	import { api, errorMessage } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import type { ActiveSession } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import SessionCard from '$lib/components/admin/SessionCard.svelte';
	import { formatBitrate } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';

	let sessions = $state<ActiveSession[] | null>(null);
	let error = $state<unknown>(null);
	let toStop = $state<ActiveSession | null>(null);
	let stopOpen = $state(false);
	let stopping = $state(false);

	async function load() {
		try {
			sessions = await api.playbackSessions();
			error = null;
		} catch (err) {
			error = err;
		}
	}

	onMount(() => {
		load();
		const off = events.debounced('playback.sessions', load, 250);
		const poll = setInterval(() => {
			if (document.visibilityState === 'visible') load();
		}, 5000);
		return () => {
			off();
			clearInterval(poll);
		};
	});

	function ask(s: ActiveSession) {
		toStop = s;
		stopOpen = true;
	}

	async function stop() {
		if (!toStop) return;
		stopping = true;
		try {
			await api.stopPlaybackSession(toStop.id);
			toasts.show('Lecture arrêtée', { detail: `${toStop.user?.name ?? 'L’utilisateur'} voit un message sur son lecteur ; sa position est conservée.` });
			stopOpen = false;
			load();
		} catch (err) {
			toasts.show('Impossible d’arrêter la lecture', { detail: errorMessage(err) });
		} finally {
			stopping = false;
		}
	}

	const transcoding = $derived(sessions?.filter((s) => s.method === 'transcode').length ?? 0);
	const total = $derived(sessions?.reduce((n, s) => n + (s.bitrate || 0), 0) ?? 0);
</script>

<svelte:head>
	<title>Lectures en cours — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<p class="micro eyebrow">Réglages</p>
		<h1>Lectures en cours</h1>
		<p class="lead">Qui regarde quoi, et comment le serveur le diffuse : lecture directe du fichier, remux (même flux dans un autre conteneur) ou transcodage.</p>
	</header>

	{#if error && !sessions}
		<EmptyState icon="alert" title="Lectures indisponibles" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !sessions}
		<div class="list" aria-busy="true"><Skeleton height="160px" radius="lg" /></div>
	{:else}
		<div class="stats">
			<StatTile label="Lectures" value={sessions.length} icon="activity" tone={sessions.length ? 'accent' : 'default'} />
			<StatTile label="Transcodages" value={transcoding} icon="cpu" />
			<StatTile label="Débit total" value={formatBitrate(total) ?? '—'} icon="gauge" hint="estimation" />
		</div>
		{#if sessions.length}
			<div class="list" aria-live="polite">
				{#each sessions as s (s.id)}<SessionCard session={s} onstop={ask} busy={stopping && toStop?.id === s.id} />{/each}
			</div>
		{:else}
			<EmptyState icon="play" title="Aucune lecture en cours" role="status" compact>
				<p>Les lectures apparaissent ici dès qu’un utilisateur lance une vidéo ou un morceau.</p>
			</EmptyState>
		{/if}
	{/if}
</section>

<ConfirmDialog bind:open={stopOpen} title="Arrêter cette lecture ?" confirmLabel={stopping ? 'Arrêt…' : 'Arrêter'} danger busy={stopping} onconfirm={stop}>
	<p>
		La lecture de <strong>{toStop?.item?.seriesTitle ?? toStop?.item?.title ?? 'cet élément'}</strong> par <strong>{toStop?.user?.name ?? 'cet utilisateur'}</strong> s’interrompt
		immédiatement. Sa position est conservée : il pourra reprendre plus tard.
	</p>
</ConfirmDialog>

<style>
	.settings {
		display: grid;
		gap: var(--gap);
		align-content: start;
		max-width: 1080px;
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

	.stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--sp-3);
	}

	.list {
		display: grid;
		gap: var(--sp-3);
	}

	@media (max-width: 767px) {
		.stats {
			gap: var(--sp-2);
		}

		.stats :global(.stat) {
			padding: var(--sp-3);
		}

		.stats :global(.ic) {
			display: none;
		}

		.stats :global(.value) {
			font-size: 1.125rem;
		}
	}
</style>
