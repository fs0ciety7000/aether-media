<script lang="ts">
	// A figure with its label (queue counters, server facts). Tone colours the value only.
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	interface Props {
		label: string;
		value: string | number;
		hint?: string;
		icon?: IconName;
		tone?: 'default' | 'accent' | 'danger';
		children?: Snippet;
	}

	let { label, value, hint, icon, tone = 'default', children }: Props = $props();
</script>

<div class={['stat', tone]}>
	{#if icon}<span class="ic" aria-hidden="true"><Icon name={icon} size={18} /></span>{/if}
	<div class="txt">
		<span class="label">{label}</span>
		<span class="value tabular">{typeof value === 'number' ? value.toLocaleString('fr-FR') : value}</span>
		{#if hint}<span class="hint">{hint}</span>{/if}
		{@render children?.()}
	</div>
</div>

<style>
	.stat {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		min-width: 0;
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--tint), var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.ic {
		display: grid;
		place-items: center;
		flex: none;
		width: 36px;
		height: 36px;
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.accent .ic {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.danger .ic {
		background: color-mix(in srgb, var(--danger) 14%, transparent);
		color: var(--danger);
	}

	.txt {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	.label {
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
	}

	.value {
		font-size: 1.5rem;
		font-weight: var(--fw-heavy);
		line-height: 1.2;
		overflow-wrap: anywhere;
	}

	.danger .value {
		color: var(--danger);
	}

	.hint {
		color: var(--text-muted);
		font-size: var(--fs-micro);
		overflow-wrap: anywhere;
	}
</style>
