<script lang="ts">
	// One job of the queue (admin): kind, target, timing, error, and the actions it allows.
	import type { Job } from '$lib/api/types';
	import { jobKindLabel } from '$lib/admin';
	import { formatRelative } from '$lib/format';
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';

	interface Props {
		job: Job;
		now?: number;
		busy?: boolean;
		onretry?: (job: Job) => void;
		oncancel?: (job: Job) => void;
	}

	let { job, now = Date.now(), busy = false, onretry, oncancel }: Props = $props();

	const when = $derived.by(() => {
		if (job.status === 'running') return `démarrée ${formatRelative(job.updatedAt, now) ?? ''}`;
		if (job.status === 'failed') return `échec ${formatRelative(job.updatedAt, now) ?? ''}`;
		const at = new Date(job.runAfter).getTime();
		return at > now + 5000 ? `prévue ${formatRelative(job.runAfter, now) ?? ''}` : `en file ${formatRelative(job.createdAt, now) ?? ''}`;
	});
	const target = $derived(job.label || `#${job.key}`);
</script>

<li class={['job', job.status]}>
	<span class="state" aria-hidden="true">
		{#if job.status === 'running'}<span class="spin"></span>{:else if job.status === 'failed'}<Icon name="alert" size={16} />{:else}<Icon name="clock" size={16} />{/if}
	</span>
	<div class="body">
		<p class="line">
			<strong>{jobKindLabel(job.kind)}</strong>
			<span class="target" title={target}>{target}</span>
		</p>
		<p class="meta">
			<span>{when}</span>
			{#if job.attempts > 1 || job.status === 'failed'}<span>{job.attempts} {job.attempts > 1 ? 'essais' : 'essai'}</span>{/if}
			{#if job.status === 'queued' && job.error}<span class="retry">nouvel essai après : {job.error}</span>{/if}
		</p>
		{#if job.status === 'failed' && job.error}<p class="err">{job.error}</p>{/if}
	</div>
	<div class="actions">
		{#if job.status === 'failed'}
			<Button size="md" icon="refresh" disabled={busy} onclick={() => onretry?.(job)} aria-label="Relancer : {jobKindLabel(job.kind)} {target}">Relancer</Button>
			<Button size="md" variant="tertiary" icon="x" disabled={busy} onclick={() => oncancel?.(job)} aria-label="Retirer : {jobKindLabel(job.kind)} {target}">Retirer</Button>
		{:else}
			<Button size="md" variant="tertiary" icon="x" disabled={busy} onclick={() => oncancel?.(job)} aria-label="Annuler : {jobKindLabel(job.kind)} {target}">Annuler</Button>
		{/if}
	</div>
</li>

<style>
	.job {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.state {
		display: grid;
		place-items: center;
		flex: none;
		width: 32px;
		height: 32px;
		margin-top: 2px;
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.running .state {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.failed .state {
		background: color-mix(in srgb, var(--danger) 14%, transparent);
		color: var(--danger);
	}

	.spin {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		border: 2px solid color-mix(in srgb, var(--accent) 30%, transparent);
		border-top-color: var(--accent);
		animation: spin 0.9s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.spin {
			animation: none;
		}
	}

	.body {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: 0;
	}

	.line {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 2px var(--sp-2);
		min-width: 0;
	}

	.target {
		min-width: 0;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--text-muted);
	}

	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 2px var(--sp-3);
		color: var(--text-muted);
		font-size: var(--fs-micro);
	}

	.retry {
		overflow-wrap: anywhere;
	}

	.err {
		margin-top: var(--sp-1);
		padding: var(--sp-2) var(--sp-3);
		border-radius: var(--r-sm);
		background: color-mix(in srgb, var(--danger) 9%, transparent);
		color: var(--text);
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.75rem;
		line-height: 1.45;
		overflow-wrap: anywhere;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		flex: none;
	}

	@media (max-width: 767px) {
		.job {
			flex-wrap: wrap;
		}

		.actions {
			width: 100%;
			padding-left: calc(32px + var(--sp-3));
		}
	}
</style>
