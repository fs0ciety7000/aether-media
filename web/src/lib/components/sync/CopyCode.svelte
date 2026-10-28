<script lang="ts">
	// A short code to type elsewhere (device pairing): shown large, grouped for reading, copied raw.
	// Without the Clipboard API (plain HTTP on a LAN) the code is selected instead.
	import Icon from '../Icon.svelte';
	import { groupCode } from '$lib/sync';

	interface Props {
		code: string;
		/** Accessible name of the code ("Code à saisir sur trakt.tv"). */
		label?: string;
		/** Greyed out (expired code). */
		stale?: boolean;
	}

	let { code, label = 'Code à saisir', stale = false }: Props = $props();

	let copied = $state(false);
	let el = $state<HTMLElement>();
	let timer: ReturnType<typeof setTimeout> | undefined;

	function select() {
		if (!el) return;
		const range = document.createRange();
		range.selectNodeContents(el);
		const sel = window.getSelection();
		sel?.removeAllRanges();
		sel?.addRange(range);
	}

	async function copy() {
		try {
			await navigator.clipboard.writeText(code);
			copied = true;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = false), 2000);
		} catch {
			select();
		}
	}
</script>

<div class="copy-code" class:stale>
	<span class="code tabular" bind:this={el} aria-label="{label} : {code.split('').join(' ')}">{groupCode(code)}</span>
	<button type="button" class="copy" onclick={copy} disabled={stale} aria-live="polite">
		<Icon name={copied ? 'check' : 'copy'} size={16} />{copied ? 'Copié' : 'Copier'}
	</button>
</div>

<style>
	.copy-code {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-3) var(--sp-3) var(--sp-3) var(--sp-5);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		max-width: 100%;
	}

	.code {
		font-size: clamp(1.75rem, 1.2rem + 2vw, 2.5rem);
		font-weight: var(--fw-heavy);
		letter-spacing: 0.12em;
		line-height: 1.1;
		color: var(--text);
		user-select: all;
		overflow-wrap: anywhere;
	}

	.stale .code {
		color: var(--text-faint);
		text-decoration: line-through;
		text-decoration-thickness: 2px;
	}

	.copy {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		flex: none;
		height: 36px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-full);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.copy:hover:not(:disabled) {
		background: color-mix(in srgb, var(--surface-raised) 86%, var(--text));
	}

	.copy:disabled {
		opacity: 0.45;
	}

	.copy :global(svg) {
		color: var(--accent);
	}
</style>
