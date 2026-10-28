<script lang="ts">
	// Réglages › Tâches (admins): the job queue (running, queued, recent failures), live through
	// the admin-only "jobs.changed" event, with retry / cancel and manual maintenance tasks.
	import { onMount } from 'svelte';
	import { api, errorMessage } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import { system } from '$lib/api/stores.svelte';
	import type { AdminTask, Job, JobQueue } from '$lib/api/types';
	import { jobKindLabel } from '$lib/admin';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import SegmentedControl from '$lib/components/SegmentedControl.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import JobRow from '$lib/components/admin/JobRow.svelte';
	import { plural } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';

	let queue = $state<JobQueue | null>(null);
	let error = $state<unknown>(null);
	let busyJob = $state<number | null>(null);
	let runningTask = $state<AdminTask | null>(null);
	let filter = $state('all');
	let now = $state(Date.now());
	let ctrl: AbortController | null = null;

	async function load() {
		ctrl?.abort();
		ctrl = new AbortController();
		try {
			queue = await api.jobQueue(ctrl.signal);
			error = null;
		} catch (err) {
			if (!(err instanceof DOMException)) error = err;
		}
	}

	onMount(() => {
		system.ensure();
		load();
		const off = events.debounced('jobs.changed', load, 300);
		const tick = setInterval(() => (now = Date.now()), 15_000);
		// Safety net when the event stream is down.
		const poll = setInterval(() => {
			if (events.status !== 'open' && document.visibilityState === 'visible') load();
		}, 10_000);
		return () => {
			off();
			ctrl?.abort();
			clearInterval(tick);
			clearInterval(poll);
		};
	});

	const TASKS: { id: AdminTask; label: string; hint: string; icon: IconName }[] = [
		{ id: 'scan', label: 'Analyser toutes les bibliothèques', hint: 'Analyse complète : relit aussi les fichiers inchangés.', icon: 'layers' },
		{ id: 'metadata', label: 'Rafraîchir les métadonnées', hint: 'Affiches, résumés et distributions de tous les titres.', icon: 'wand' },
		{ id: 'sync', label: 'Synchroniser Trakt et Simkl', hint: 'Pour chaque compte relié par les utilisateurs.', icon: 'refresh' }
	];

	async function run(task: AdminTask) {
		runningTask = task;
		try {
			const r = await api.runTask(task);
			const label = TASKS.find((t) => t.id === task)?.label ?? task;
			toasts.show(r.queued ? `${label} : lancé` : `${label} : rien à lancer`, {
				detail: r.queued ? plural(r.queued, 'tâche ajoutée à la file', 'tâches ajoutées à la file') : task === 'sync' ? 'Aucun compte relié, ou synchronisation déjà en file.' : 'Déjà en file, ou aucune bibliothèque.'
			});
			load();
		} catch (err) {
			toasts.show('Impossible de lancer la tâche', { detail: errorMessage(err) });
		} finally {
			runningTask = null;
		}
	}

	async function retry(job: Job) {
		busyJob = job.id;
		try {
			await api.retryJob(job.id);
			toasts.show('Tâche relancée', { detail: `${jobKindLabel(job.kind)} · ${job.label || job.key}` });
			load();
		} catch (err) {
			toasts.show('Impossible de relancer', { detail: errorMessage(err) });
		} finally {
			busyJob = null;
		}
	}

	async function cancel(job: Job) {
		busyJob = job.id;
		try {
			await api.cancelJob(job.id);
			toasts.show(job.status === 'failed' ? 'Échec retiré de la liste' : job.status === 'running' ? 'Annulation demandée' : 'Tâche retirée de la file');
			load();
		} catch (err) {
			toasts.show('Impossible d’annuler', { detail: errorMessage(err) });
			load();
		} finally {
			busyJob = null;
		}
	}

	const kinds = $derived(queue ? Object.keys(queue.byKind).sort() : []);
	const filterOptions = $derived([{ value: 'all', label: 'Toutes' }, ...kinds.map((k) => ({ value: k, label: jobKindLabel(k) }))]);
	const pick = (list: Job[]) => (filter === 'all' ? list : list.filter((j) => j.kind === filter));
	const running = $derived(queue ? pick(queue.running) : []);
	const queued = $derived(queue ? pick(queue.queued) : []);
	const failed = $derived(queue ? pick(queue.failed) : []);
	const syncAvailable = $derived(!!(system.info?.sync?.trakt || system.info?.sync?.simkl));
</script>

<svelte:head>
	<title>Tâches — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<p class="micro eyebrow">Réglages</p>
		<h1>Tâches</h1>
		<p class="lead">Analyses, métadonnées, images et synchronisations passent par une file d’attente qui survit aux redémarrages. Elle se met à jour en direct.</p>
	</header>

	<div class="tasks" role="list" aria-label="Lancer manuellement">
		{#each TASKS as t (t.id)}
			{@const disabled = runningTask !== null || (t.id === 'sync' && !syncAvailable)}
			<div class="task panel" role="listitem">
				<span class="t-icon" aria-hidden="true"><Icon name={t.icon} size={20} /></span>
				<div class="t-text">
					<strong>{t.label}</strong>
					<span>{t.id === 'sync' && !syncAvailable ? 'Aucun service configuré (Réglages › Synchronisation).' : t.hint}</span>
				</div>
				<Button icon="play" {disabled} onclick={() => run(t.id)}>{runningTask === t.id ? 'Lancement…' : 'Lancer'}</Button>
			</div>
		{/each}
	</div>

	{#if error && !queue}
		<EmptyState icon="alert" title="File d’attente indisponible" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !queue}
		<div class="stats" aria-busy="true"><Skeleton height="88px" radius="lg" /><Skeleton height="88px" radius="lg" /><Skeleton height="88px" radius="lg" /></div>
	{:else}
		<div class="stats" aria-live="polite">
			<StatTile label="En cours" value={queue.counts.running ?? 0} icon="activity" tone={queue.counts.running ? 'accent' : 'default'} />
			<StatTile label="En attente" value={queue.counts.queued ?? 0} icon="clock" />
			<StatTile label="En échec" value={queue.counts.failed ?? 0} icon="alert" tone={queue.counts.failed ? 'danger' : 'default'} />
		</div>

		{#if kinds.length > 1}
			<SegmentedControl options={filterOptions} bind:value={filter} label="Filtrer par type de tâche" size="sm" />
		{/if}

		<section class="panel list" aria-labelledby="h-running">
			<h2 id="h-running">En cours</h2>
			{#if running.length}
				<ul role="list">
					{#each running as job (job.id)}<JobRow {job} {now} busy={busyJob === job.id} oncancel={cancel} />{/each}
				</ul>
			{:else}
				<p class="empty">Aucune tâche en cours.</p>
			{/if}
		</section>

		<section class="panel list" aria-labelledby="h-queued">
			<h2 id="h-queued">En attente{#if queue.counts.queued > queue.queued.length}<span class="more"> · {queue.queued.length} premières sur {queue.counts.queued.toLocaleString('fr-FR')}</span>{/if}</h2>
			{#if queued.length}
				<ul role="list">
					{#each queued as job (job.id)}<JobRow {job} {now} busy={busyJob === job.id} oncancel={cancel} />{/each}
				</ul>
			{:else}
				<p class="empty">La file est vide.</p>
			{/if}
		</section>

		<section class="panel list" aria-labelledby="h-failed">
			<h2 id="h-failed">Échecs récents</h2>
			{#if failed.length}
				<ul role="list">
					{#each failed as job (job.id)}<JobRow {job} {now} busy={busyJob === job.id} onretry={retry} oncancel={cancel} />{/each}
				</ul>
			{:else}
				<p class="empty"><Icon name="check-circle" size={16} />Aucun échec.</p>
			{/if}
		</section>
	{/if}
</section>

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

	.tasks {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
		gap: var(--sp-3);
	}

	.task {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-4);
		border-radius: var(--r-lg);
	}

	.t-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 40px;
		height: 40px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.t-text {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: 0;
		color: var(--text-muted);
		font-size: var(--fs-micro);
	}

	.t-text strong {
		color: var(--text);
		font-size: var(--fs-small);
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--sp-3);
	}

	.list {
		display: grid;
		gap: var(--sp-3);
		padding: var(--sp-5);
	}

	h2 {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
	}

	.more {
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
	}

	ul {
		display: grid;
		gap: var(--sp-2);
	}

	.empty {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.empty :global(svg) {
		color: var(--success);
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

		.list {
			padding: var(--sp-4);
		}

		.task {
			flex-wrap: wrap;
		}
	}
</style>
