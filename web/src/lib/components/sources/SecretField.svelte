<script lang="ts">
	// Write-only secret (token, password, private key): masked by default with a show/hide toggle.
	// The API never returns secrets, so in edit mode an empty field means "unchanged".
	import Icon from '../Icon.svelte';

	interface Props {
		id: string;
		label: string;
		value: string;
		multiline?: boolean;
		placeholder?: string;
		hint?: string;
		error?: string;
		optional?: boolean;
		mono?: boolean;
		autocomplete?: 'off' | 'new-password' | 'current-password';
	}

	let { id, label, value = $bindable(''), multiline = false, placeholder, hint, error, optional = false, mono = true, autocomplete = 'off' }: Props = $props();

	let shown = $state(false);
	const describedby = $derived([hint ? `${id}-hint` : '', error ? `${id}-err` : ''].filter(Boolean).join(' ') || undefined);
</script>

<div class="f-field">
	<div class="head">
		<label class="f-label" for={id}>{label}{#if optional}<span class="f-optional">{' (facultatif)'}</span>{/if}</label>
		<button type="button" class="toggle" aria-controls={id} aria-pressed={shown} onclick={() => (shown = !shown)}>
			<Icon name={shown ? 'eye-off' : 'eye'} size={15} />{shown ? 'Masquer' : 'Afficher'}<span class="visually-hidden"> {label.toLowerCase()}</span>
		</button>
	</div>
	{#if multiline}
		<textarea
			{id}
			class={['f-input', { 'f-mono': mono, masked: !shown }]}
			bind:value
			{placeholder}
			rows="6"
			spellcheck="false"
			autocomplete="off"
			autocapitalize="off"
			aria-invalid={!!error}
			aria-describedby={describedby}
		></textarea>
	{:else}
		<input
			{id}
			class={['f-input', { 'f-mono': mono && shown }]}
			type={shown ? 'text' : 'password'}
			bind:value
			{placeholder}
			{autocomplete}
			spellcheck="false"
			autocapitalize="off"
			aria-invalid={!!error}
			aria-describedby={describedby}
		/>
	{/if}
	{#if hint}<p class="f-hint" id="{id}-hint">{hint}</p>{/if}
	{#if error}<p class="f-err" id="{id}-err"><Icon name="alert" size={14} />{error}</p>{/if}
</div>

<style>
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
	}

	.toggle {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		min-height: 24px;
		padding: 0 6px;
		margin-right: -6px;
		border-radius: var(--r-xs);
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	.toggle:hover {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 7%, transparent);
	}

	/* Textareas cannot be type=password: mask the glyphs where supported. */
	textarea.masked {
		-webkit-text-security: disc;
	}
</style>
