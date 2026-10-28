<script lang="ts">
	// Password input with a show / hide toggle.
	import type { HTMLInputAttributes } from 'svelte/elements';
	import Icon from '../Icon.svelte';
	import Input from '../Input.svelte';

	interface Props extends Omit<HTMLInputAttributes, 'size' | 'type'> {
		value?: string;
		label: string;
		hint?: string;
		size?: 'md' | 'lg';
	}

	let { value = $bindable(''), label, hint, size = 'lg', ...rest }: Props = $props();
	let shown = $state(false);
</script>

<Input bind:value {label} {hint} {size} type={shown ? 'text' : 'password'} spellcheck="false" autocapitalize="off" {...rest}>
	{#snippet trailing()}
		<button
			type="button"
			class="reveal"
			aria-pressed={shown}
			aria-label={shown ? 'Masquer le mot de passe' : 'Afficher le mot de passe'}
			title={shown ? 'Masquer' : 'Afficher'}
			onclick={() => (shown = !shown)}
		>
			<Icon name={shown ? 'eye-off' : 'eye'} size={18} />
		</button>
	{/snippet}
</Input>

<style>
	.reveal {
		display: grid;
		place-items: center;
		width: 34px;
		height: 34px;
		border-radius: var(--r-sm);
		color: var(--text-muted);
	}

	.reveal:hover {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 7%, transparent);
	}
</style>
