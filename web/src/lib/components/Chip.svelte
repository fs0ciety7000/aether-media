<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon from './Icon.svelte';

	interface Props extends HTMLButtonAttributes {
		selected?: boolean;
		/** Optional count shown after the label. */
		count?: number;
		children: Snippet;
	}

	let { selected = $bindable(false), count, children, class: className, onclick, ...rest }: Props = $props();
</script>

<button
	type="button"
	class={['chip', { selected }, className]}
	aria-pressed={selected}
	onclick={(e) => {
		selected = !selected;
		onclick?.(e);
	}}
	{...rest}
>
	<span class="check" aria-hidden="true"><Icon name="check" size={14} /></span>
	<span>{@render children()}</span>
	{#if count !== undefined}<span class="count tabular">{count}</span>{/if}
</button>

<style>
	.chip {
		display: inline-flex;
		align-items: center;
		height: 32px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-sm);
		background: var(--surface);
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: 500;
		white-space: nowrap;
		box-shadow: inset 0 0 0 1px var(--line);
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease),
			padding var(--dur-base) var(--ease);
	}

	.chip:hover:not(:disabled),
	.chip[data-demo='hover'] {
		color: var(--text);
		background: color-mix(in srgb, var(--surface) 92%, var(--text));
	}

	.check {
		display: inline-flex;
		width: 0;
		overflow: hidden;
		opacity: 0;
		transition:
			width var(--dur-base) var(--ease),
			opacity var(--dur-base) var(--ease),
			margin var(--dur-base) var(--ease);
	}

	.selected {
		background: var(--accent-soft);
		color: var(--accent-ink);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 22%, transparent);
	}

	.selected:hover:not(:disabled) {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.selected .check {
		width: 14px;
		margin-right: 6px;
		opacity: 1;
	}

	.count {
		margin-left: var(--sp-2);
		font-size: var(--fs-micro);
		opacity: 0.75;
	}

	.chip:disabled {
		opacity: 0.4;
	}
</style>
