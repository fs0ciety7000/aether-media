<script lang="ts">
	// One source of a library in the settings: kind icon, name, location, live status, actions.
	import type { Source } from '$lib/api/types';
	import { formatRelative } from '$lib/format';
	import { sourceErrorMessage, sourceKindInfo, sourceLocation } from '$lib/sources';
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';
	import SourceStatusPill from './SourceStatusPill.svelte';

	interface Props {
		source: Source;
		/** Clock for relative times (refreshed by the page). */
		now?: number;
		onedit?: (s: Source) => void;
		ondelete?: (s: Source) => void;
	}

	let { source, now, onedit, ondelete }: Props = $props();

	const info = $derived(sourceKindInfo(source.kind));
	const location = $derived(sourceLocation(source));
	const showLocation = $derived(location && location !== source.name);
	const seen = $derived(formatRelative(source.lastSeen, now ?? Date.now()));
	const extra = $derived.by(() => {
		const parts: string[] = [info.label];
		if (source.agent?.version) parts.push(`agent ${source.agent.version}`);
		if (source.status !== 'online' && seen) parts.push(`vue ${seen}`);
		return parts.join(' · ');
	});
</script>

<div class={['src', source.status]} data-source-id={source.id}>
	<span class="s-icon" aria-hidden="true"><Icon name={info.icon} size={18} /></span>
	<div class="s-text">
		<p class="s-name">{source.name}</p>
		{#if showLocation}<p class="s-loc"><code>{location}</code></p>{/if}
		<p class="s-meta">{extra}</p>
		{#if source.status === 'offline' && source.lastError}
			<p class="s-err"><Icon name="alert" size={13} />{sourceErrorMessage(source.lastError)}</p>
		{/if}
	</div>
	{#if !source.derived}
		<span class="s-status"><SourceStatusPill status={source.status} lastError={source.lastError} lastSeen={source.lastSeen} {now} /></span>
		<span class="s-actions">
			<Button variant="tertiary" icon="edit" onclick={() => onedit?.(source)} aria-label="Modifier la source {source.name}"><span class="lbl">Modifier</span></Button>
			<Button variant="tertiary" icon="trash" onclick={() => ondelete?.(source)} aria-label="Supprimer la source {source.name}"><span class="lbl">Supprimer</span></Button>
		</span>
	{/if}
</div>

<style>
	.src {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		transition: background-color var(--dur-base) var(--ease);
	}

	.s-icon {
		display: grid;
		place-items: center;
		width: 38px;
		height: 38px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.offline .s-icon {
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.s-text {
		display: grid;
		gap: 1px;
		min-width: 0;
	}

	.s-name {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.s-loc,
	.s-meta {
		font-size: var(--fs-micro);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.75rem;
		color: var(--text-muted);
	}

	.s-err {
		display: flex;
		align-items: flex-start;
		gap: 5px;
		margin-top: 2px;
		font-size: var(--fs-micro);
		color: var(--danger);
	}

	.s-err :global(svg) {
		flex: none;
		margin-top: 1px;
	}

	.s-actions {
		display: flex;
		gap: 2px;
	}

	@media (max-width: 767px) {
		.src {
			grid-template-columns: auto minmax(0, 1fr) auto;
			grid-template-areas: 'icon text text' 'icon status actions';
			row-gap: var(--sp-2);
			align-items: start;
		}

		.s-icon {
			grid-area: icon;
		}

		.s-text {
			grid-area: text;
		}

		.s-status {
			grid-area: status;
			align-self: center;
		}

		.s-actions {
			grid-area: actions;
			justify-self: end;
		}

		/* Icon-only actions on phones (the aria-label carries the text). */
		.lbl {
			display: none;
		}

		.s-actions :global(.btn) {
			width: 40px;
			padding: 0 !important;
		}
	}
</style>
