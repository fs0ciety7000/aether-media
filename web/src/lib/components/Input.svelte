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
		grid-template-columns: minmax(0, 1fr);
		gap: var(--sp-2);
		min-width: 0;
	}

	.label {
		font-size: var(--fs-small);
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	.control {
		--h: 40px;
		position: relative;
		display: flex;
		align-items: center;
		height: var(--h);
		background: var(--surface);
		border-radius: var(--r-md);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		transition:
			background-color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.control.lg {
		--h: 46px;
	}

	.control:hover:not(:focus-within) {
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	/* Focus: matte accent ring drawn inside the field. */
	.control:focus-within,
	.control.demo-focus {
		color: var(--text);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 70%, transparent);
	}

	input {
		flex: 1;
		min-width: 0;
		height: 100%;
		padding: 0 var(--sp-4);
		background: transparent;
		border: 0;
		color: var(--text);
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
		padding-left: 14px;
	}

	.lg .leading {
		padding-left: var(--sp-4);
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
