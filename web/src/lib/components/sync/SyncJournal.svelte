<script lang="ts">
	// Recent synchronisation events: conflicts (with the side kept), imports, exports, errors.
	// Compact rows, filterable by kind, ten at a time.
	import type { SyncEvent, SyncService } from '$lib/api/types';
	import { formatDate, formatRelative } from '$lib/format';
	import { EVENT_KIND, FEATURE_LABEL, SERVICE_INFO, winnerLabel } from '$lib/sync';
	import Chip from '../Chip.svelte';
	import Icon from '../Icon.svelte';
	import Skeleton from '../Skeleton.svelte';

	interface Props {
		events: SyncEvent[] | null;
		service: SyncService;
		error?: string;
		/** Current time (ms), for the relative dates. */
		now?: number;
		/** Rows shown before "Afficher plus". */
		step?: number;
	}

	let { events, service, error, now = Date.now(), step = 10 }: Props = $props();

	const KINDS: SyncEvent['kind'][] = ['conflict', 'error', 'import', 'export'];
	const PLURAL: Record<SyncEvent['kind'], string> = { conflict: 'Conflits', error: 'Erreurs', import: 'Imports', export: 'Exports' };

	let filter = $state<SyncEvent['kind'] | null>(null);
	let extra = $state(0);
	const shown = $derived(step + extra);

	const counts = $derived(KINDS.map((k) => ({ kind: k, n: (events ?? []).filter((e) => e.kind === k).length })).filter((c) => c.n > 0));
	const filtered = $derived((events ?? []).filter((e) => !filter || e.kind === filter));
	const visible = $derived(filtered.slice(0, shown));
	const service_ = $derived(SERVICE_INFO[service].name);

	function fullDate(iso: string) {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : `${formatDate(iso)} à ${d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}`;
	}
</script>

<div class="journal">
	{#if events === null}
		<div class="rows" aria-busy="true"><Skeleton height="44px" radius="md" /><Skeleton height="44px" radius="md" /></div>
	{:else if error && !events.length}
		<p class="empty" role="alert"><Icon name="alert" size={16} />{error}</p>
	{:else if !events.length}
		<p class="empty"><Icon name="info" size={16} />Rien à signaler pour l’instant. Les imports, exports, conflits et erreurs des 30 derniers jours apparaîtront ici.</p>
	{:else}
		{#if counts.length > 1}
			<div class="filters" role="group" aria-label="Filtrer le journal">
				<Chip toggle={false} selected={!filter} count={events.length} onclick={() => ((filter = null), (extra = 0))}>Tout</Chip>
				{#each counts as c (c.kind)}
					<Chip toggle={false} selected={filter === c.kind} count={c.n} onclick={() => ((filter = filter === c.kind ? null : c.kind), (extra = 0))}>{PLURAL[c.kind]}</Chip>
				{/each}
			</div>
		{/if}
		<ul class="rows" role="list">
			{#each visible as e (e.id)}
				{@const k = EVENT_KIND[e.kind] ?? EVENT_KIND.import}
				{@const win = winnerLabel(e, service)}
				<li class="row {e.kind}">
					<span class="kind" title={k.label}><Icon name={k.icon} size={16} /><span class="visually-hidden">{k.label}</span></span>
					<div class="main">
						<p class="what">
							{#if e.itemTitle}
								{#if e.itemId}<a href="/item/{e.itemId}">{e.itemTitle}</a>{:else}<span class="title">{e.itemTitle}</span>{/if}
							{:else}
								<span class="title">{k.label}</span>
							{/if}
							<span class="feature">{FEATURE_LABEL[e.feature] ?? e.feature}</span>
							{#if win}<span class="winner" class:remote={e.winner === 'remote'}>{win}</span>{/if}
						</p>
						{#if e.message}<p class="msg">{e.message}</p>{/if}
					</div>
					<time class="when" datetime={e.time} title={fullDate(e.time)}>{formatRelative(e.time, now) ?? ''}</time>
				</li>
			{/each}
		</ul>
		{#if filtered.length > shown}
			<button type="button" class="more" onclick={() => (extra += step)}>Afficher plus ({filtered.length - shown} de plus)</button>
		{/if}
		<p class="note micro">Conflit : le titre a changé des deux côtés depuis la dernière synchronisation ; la modification la plus récente l’emporte (Aether ou {service_}).</p>
	{/if}
</div>

<style>
	.journal {
		display: grid;
		gap: var(--sp-3);
	}

	.filters {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.rows {
		display: grid;
		gap: 2px;
	}

	.row {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding: var(--sp-2) var(--sp-3);
		border-radius: var(--r-md);
	}

	.row:nth-child(odd) {
		background: color-mix(in srgb, var(--bg) 35%, transparent);
	}

	.kind {
		display: grid;
		place-items: center;
		flex: none;
		width: 28px;
		height: 28px;
		margin-top: 1px;
		border-radius: var(--r-sm);
		background: var(--surface-raised);
		color: var(--text-muted);
	}

	.conflict .kind {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.error .kind {
		background: color-mix(in srgb, var(--danger) 14%, transparent);
		color: var(--danger);
	}

	.main {
		display: grid;
		gap: 1px;
		flex: 1;
		min-width: 0;
	}

	.what {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 2px var(--sp-2);
		font-size: var(--fs-small);
	}

	.what a,
	.title {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.what a:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.feature {
		color: var(--text-muted);
		font-size: var(--fs-micro);
	}

	.feature::before {
		content: '·';
		margin-right: var(--sp-2);
	}

	.winner {
		padding: 0 8px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		line-height: 20px;
	}

	.msg {
		color: var(--text-muted);
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		overflow-wrap: anywhere;
	}

	.error .msg {
		color: var(--danger);
	}

	.when {
		flex: none;
		padding-top: 5px;
		color: var(--text-faint);
		font-size: var(--fs-micro);
		white-space: nowrap;
	}

	.empty {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.empty :global(svg) {
		flex: none;
		margin-top: 2px;
	}

	.more {
		justify-self: start;
		padding: 4px 0;
		color: var(--accent-ink);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		border-radius: var(--r-xs);
	}

	.note {
		color: var(--text-faint);
	}

	@media (max-width: 479px) {
		.row {
			flex-wrap: wrap;
			padding: var(--sp-2);
		}

		.main {
			flex-basis: calc(100% - 40px);
		}

		.when {
			padding: 0 0 0 40px;
		}
	}
</style>
