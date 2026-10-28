<script lang="ts">
	// Styled native <select> (keeps platform pickers and keyboard behaviour).
	import type { HTMLSelectAttributes } from 'svelte/elements';
	import Icon from './Icon.svelte';

	interface Props extends HTMLSelectAttributes {
		value?: string;
		options: { value: string; label: string }[];
		label: string;
		/** Show the label visually (otherwise aria-label only). */
		showLabel?: boolean;
	}

	let { value = $bindable(''), options, label, showLabel = false, class: className, ...rest }: Props = $props();
	const uid = $props.id();
</script>

<div class={['select', className]}>
	{#if showLabel}<label class="label" for="sel-{uid}">{label}</label>{/if}
	<div class="control">
		<select id="sel-{uid}" bind:value aria-label={showLabel ? undefined : label} {...rest}>
			{#each options as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
		</select>
		<Icon name="chevron-down" size={16} />
	</div>
</div>

<style>
	.select {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
	}

	.label {
		font-size: var(--fs-small);
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	.control {
		position: relative;
		display: flex;
		align-items: center;
		color: var(--text-muted);
	}

	select {
		appearance: none;
		width: 100%;
		height: 38px;
		padding: 0 36px 0 14px;
		border: 0;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text);
		font-size: var(--fs-body);
		font-weight: var(--fw-medium);
		cursor: pointer;
		transition:
			box-shadow var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	select:hover {
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	select:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	option {
		background: var(--surface-raised);
		color: var(--text);
	}

	.control :global(svg) {
		position: absolute;
		right: 14px;
		pointer-events: none;
	}
</style>
