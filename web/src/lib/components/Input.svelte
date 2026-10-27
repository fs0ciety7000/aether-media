<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLInputAttributes, 'size'> {
		value?: string;
		label?: string;
		hint?: string;
		size?: 'md' | 'lg';
		/** Leading visual (icon). */
		leading?: Snippet;
		/** Trailing content (clear button, shortcut hint). */
		trailing?: Snippet;
		element?: HTMLInputElement;
		/** Forces the focus look in the design gallery. */
		'data-demo'?: string;
	}

	let {
		value = $bindable(''),
		element = $bindable(),
		label,
		hint,
		size = 'md',
		leading,
		trailing,
		id,
		class: className,
		'data-demo': demo,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const inputId = $derived(id ?? `input-${uid}`);
</script>

<div class={['field', className]}>
	{#if label}
		<label class="label" for={inputId}>{label}</label>
	{/if}
	<div class={['control', size, { 'has-leading': leading, 'has-trailing': trailing, 'demo-focus': demo === 'focus' }]}>
		{#if leading}<span class="leading">{@render leading()}</span>{/if}
		<input
			bind:this={element}
			bind:value
			id={inputId}
			aria-describedby={hint ? `${inputId}-hint` : undefined}
			{...rest}
		/>
		{#if trailing}<span class="trailing">{@render trailing()}</span>{/if}
	</div>
	{#if hint}
		<p class="hint" id="{inputId}-hint">{hint}</p>
	{/if}
</div>

<style>
	.field {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
	}

	.label {
		font-size: var(--fs-small);
		color: var(--text-muted);
		font-weight: 500;
	}

	.control {
		--h: 40px;
		position: relative;
		display: flex;
		align-items: center;
		height: var(--h);
		background: var(--bg-sunken);
		border-radius: var(--r-md);
		color: var(--text-muted);
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.control.lg {
		--h: 48px;
	}

	/* Accent line on focus — drawn inside, along the bottom edge. */
	.control::after {
		content: '';
		position: absolute;
		left: var(--sp-3);
		right: var(--sp-3);
		bottom: 0;
		height: 1.5px;
		border-radius: 1px;
		background: var(--accent);
		transform: scaleX(0);
		opacity: 0;
		transition:
			transform var(--dur-base) var(--ease),
			opacity var(--dur-base) var(--ease);
	}

	.control:focus-within::after,
	.control.demo-focus::after {
		transform: scaleX(1);
		opacity: 1;
	}

	.control:hover:not(:focus-within) {
		background: color-mix(in srgb, var(--bg-sunken) 88%, var(--text));
	}

	.control:focus-within,
	.control.demo-focus {
		color: var(--text);
	}

	input {
		flex: 1;
		min-width: 0;
		height: 100%;
		padding: 0 var(--sp-4);
		background: transparent;
		border: 0;
		color: var(--text);
		font-size: var(--fs-small);
	}

	.lg input {
		font-size: var(--fs-body);
	}

	input:focus-visible {
		outline: none;
	}

	input::placeholder {
		color: var(--text-faint);
		opacity: 1;
	}

	input:disabled {
		cursor: not-allowed;
	}

	.control:has(input:disabled) {
		opacity: 0.5;
	}

	.has-leading input {
		padding-left: var(--sp-2);
	}

	.has-trailing input {
		padding-right: var(--sp-2);
	}

	.leading {
		display: flex;
		padding-left: var(--sp-3);
	}

	.trailing {
		display: flex;
		align-items: center;
		padding-right: var(--sp-2);
	}

	.hint {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	/* Hide the native search cancel button: we draw our own. */
	input::-webkit-search-cancel-button {
		-webkit-appearance: none;
		appearance: none;
	}
</style>
