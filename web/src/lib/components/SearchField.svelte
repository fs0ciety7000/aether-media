<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import Icon from './Icon.svelte';
	import Input from './Input.svelte';

	interface Props extends Omit<HTMLInputAttributes, 'size'> {
		value?: string;
		size?: 'md' | 'lg';
		/** Focus the field when "/" is pressed anywhere. */
		shortcut?: boolean;
		label?: string;
	}

	let {
		value = $bindable(''),
		size = 'md',
		shortcut = false,
		label = 'Rechercher',
		placeholder = 'Films, séries, artistes…',
		...rest
	}: Props = $props();

	let element = $state<HTMLInputElement>();

	function onKeydown(e: KeyboardEvent) {
		if (!shortcut || e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
		const target = e.target as HTMLElement | null;
		if (target?.closest('input, textarea, select, [contenteditable="true"]')) return;
		e.preventDefault();
		element?.focus();
	}

	function clear() {
		value = '';
		element?.focus();
	}
</script>

<svelte:window onkeydown={onKeydown} />

<Input bind:value bind:element type="search" {size} {placeholder} aria-label={label} autocomplete="off" spellcheck={false} {...rest}>
	{#snippet leading()}
		<Icon name="search" size={size === 'lg' ? 20 : 18} />
	{/snippet}
	{#snippet trailing()}
		{#if value}
			<button class="clear" type="button" aria-label="Effacer la recherche" onclick={clear}>
				<Icon name="x" size={16} />
			</button>
		{:else if shortcut}
			<kbd aria-hidden="true">/</kbd>
		{/if}
	{/snippet}
</Input>

<style>
	.clear {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: var(--r-sm);
		color: var(--text-muted);
		transition: color var(--dur-fast) var(--ease);
	}

	.clear:hover {
		color: var(--text);
	}

	kbd {
		display: grid;
		place-items: center;
		min-width: 22px;
		height: 22px;
		margin-right: var(--sp-1);
		border-radius: var(--r-sm);
		font-family: var(--font-ui);
		font-size: var(--fs-micro);
		color: var(--text-muted);
		box-shadow: inset 0 0 0 1px var(--line);
	}
</style>
