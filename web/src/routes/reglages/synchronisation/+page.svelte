<script lang="ts">
	// Réglages › Synchronisation (admins): Trakt and Simkl application credentials.
	import { onMount } from 'svelte';
	import { api, errorMessage } from '$lib/api/client';
	import type { SyncApp, SyncApps, SyncService } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import SyncAppCard from '$lib/components/sync/SyncAppCard.svelte';
	import { SYNC_SERVICES } from '$lib/sync';

	let apps = $state<SyncApps | null>(null);
	let error = $state<unknown>(null);

	async function load() {
		try {
			apps = await api.syncApps();
			error = null;
		} catch (err) {
			error = err;
		}
	}

	onMount(load);

	const NONE: SyncApp = { configured: false, source: 'none' };

	function changed(service: SyncService, app: SyncApp) {
		apps = { ...(apps ?? {}), [service]: app };
	}
</script>

<svelte:head>
	<title>Synchronisation — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<p class="micro eyebrow">Réglages</p>
		<h1>Synchronisation</h1>
		<p class="lead">
			Pour que les utilisateurs relient leur compte Trakt ou Simkl, Aether doit être déclaré comme application auprès de chaque service. Une seule application suffit pour tout le serveur ; chacun relie ensuite son propre compte depuis
			<a href="/profil/synchronisation">Profil › Synchronisation</a>.
		</p>
	</header>

	{#if error && !apps}
		<EmptyState icon="alert" title="Réglages indisponibles" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !apps}
		<div class="list" aria-busy="true"><Skeleton height="260px" radius="lg" /><Skeleton height="200px" radius="lg" /></div>
	{:else}
		<div class="list">
			{#each SYNC_SERVICES as service (service)}
				<SyncAppCard {service} app={apps[service] ?? NONE} onchange={(a) => changed(service, a)} />
			{/each}
		</div>
		<p class="tip">
			<Icon name="shield" size={16} />
			<span>Les identifiants et les jetons des utilisateurs sont chiffrés dans la base du serveur. Ceux définis dans la configuration (variables <code>AETHER_TRAKT_*</code>, <code>AETHER_SIMKL_*</code> ou section <code>[sync]</code>) sont prioritaires.</span>
		</p>
	{/if}
</section>

<style>
	.settings {
		display: grid;
		gap: var(--gap);
		align-content: start;
		max-width: 980px;
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
		max-width: 72ch;
		color: var(--text-muted);
	}

	.lead a {
		color: var(--accent-ink);
		font-weight: var(--fw-bold);
	}

	.list {
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

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.8125em;
		color: var(--text);
	}
</style>
