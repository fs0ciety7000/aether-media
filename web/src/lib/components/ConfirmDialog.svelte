<script lang="ts">
	import type { Snippet } from 'svelte';
	import Button from './Button.svelte';
	import Dialog from './Dialog.svelte';

	interface Props {
		open: boolean;
		title: string;
		confirmLabel: string;
		danger?: boolean;
		busy?: boolean;
		children: Snippet;
		onconfirm: () => void;
	}

	let { open = $bindable(false), title, confirmLabel, danger = false, busy = false, children, onconfirm }: Props = $props();
</script>

<Dialog bind:open {title} size="sm" initialFocus=".foot .cancel">
	<div class="text">{@render children()}</div>
	{#snippet footer()}
		<Button variant="tertiary" class="cancel" onclick={() => (open = false)}>Annuler</Button>
		<Button variant="primary" class={danger ? 'danger' : undefined} disabled={busy} onclick={onconfirm}>{confirmLabel}</Button>
	{/snippet}
</Dialog>

<style>
	.text {
		color: var(--text-muted);
	}

	.text :global(strong) {
		color: var(--text);
	}

	:global(.btn.primary.danger) {
		background: var(--danger);
		color: var(--accent-contrast);
	}

	:global(.btn.primary.danger:hover:not(:disabled)) {
		background: color-mix(in srgb, var(--danger) 86%, var(--text));
	}
</style>
