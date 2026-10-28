<script lang="ts">
	// Online / offline / unknown pill. Detail (last error, last seen) in the tooltip and for screen readers.
	import type { SourceStatus } from '$lib/api/types';
	import { formatRelative } from '$lib/format';
	import { sourceErrorMessage, STATUS_LABEL } from '$lib/sources';

	interface Props {
		status: SourceStatus;
		lastError?: string;
		lastSeen?: string;
		/** Re-render tick for the relative time (pass a changing value to refresh). */
		now?: number;
	}

	let { status, lastError, lastSeen, now }: Props = $props();

	const seen = $derived(formatRelative(lastSeen, now ?? Date.now()));
	const detail = $derived(
		[status !== 'online' && lastError ? sourceErrorMessage(lastError) : undefined, seen ? `Vue ${seen}` : undefined]
			.filter(Boolean)
			.join(' · ')
	);
</script>

<span class={['pill', status]} title={detail || undefined}>
	<span class="dot" aria-hidden="true"></span>
	{STATUS_LABEL[status] ?? STATUS_LABEL.unknown}
	{#if detail}<span class="visually-hidden">({detail})</span>{/if}
</span>

<style>
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		height: 26px;
		padding: 0 11px 0 9px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		line-height: 1;
		white-space: nowrap;
		cursor: default;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--text-faint);
	}

	.online {
		color: var(--text);
	}

	.online .dot {
		background: var(--success);
	}

	.offline {
		color: var(--text);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 45%, transparent);
	}

	.offline .dot {
		background: var(--danger);
	}

	.cert_mismatch {
		color: var(--text);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 60%, transparent);
	}

	.cert_mismatch .dot {
		background: var(--danger);
		border-radius: 2px;
	}

	.unknown .dot {
		background: transparent;
		box-shadow: inset 0 0 0 1.5px var(--text-muted);
	}
</style>
