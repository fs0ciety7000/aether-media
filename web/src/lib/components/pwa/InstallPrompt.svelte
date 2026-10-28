<script lang="ts">
	// Discreet install offer: a small card above the content, shown a few seconds after sign-in,
	// once (dismissal is remembered). Chromium: the browser's own prompt. iOS Safari: how to do it.
	import { onMount } from 'svelte';
	import { pwa } from '$lib/pwa.svelte';
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';
	import IconButton from '../IconButton.svelte';

	interface Props {
		/** Show at once (design gallery) instead of after a delay. */
		immediate?: boolean;
		/** Force a variant (design gallery). */
		variant?: 'prompt' | 'ios';
	}

	let { immediate = false, variant }: Props = $props();

	let ready = $state(false);
	onMount(() => {
		if (immediate || variant) return void (ready = true);
		const t = setTimeout(() => (ready = true), 4000);
		return () => clearTimeout(t);
	});

	const kind = $derived(variant ?? (pwa.deferred ? 'prompt' : pwa.ios ? 'ios' : null));
	const visible = $derived(ready && (variant ? true : pwa.canOffer) && kind !== null);
</script>

{#if visible}
	<aside class="install" class:inline={!!variant} aria-labelledby="install-title">
		<span class="ic" aria-hidden="true"><Icon name="install" size={20} /></span>
		<div class="txt">
			<p id="install-title" class="title">Installer Aether</p>
			{#if kind === 'prompt'}
				<p class="hint">Une icône sur cet appareil, en plein écran, sans barre de navigateur.</p>
			{:else}
				<p class="hint">Dans Safari, touchez <strong>Partager</strong> puis <strong>Sur l’écran d’accueil</strong>.</p>
			{/if}
		</div>
		<div class="act">
			{#if kind === 'prompt'}<Button variant="primary" onclick={() => pwa.install()}>Installer</Button>{/if}
			<IconButton icon="x" variant="ghost" size="sm" aria-label="Ne plus proposer l’installation" onclick={() => pwa.dismiss()} />
		</div>
	</aside>
{/if}

<style>
	.install {
		position: fixed;
		z-index: 60;
		right: var(--sp-5);
		bottom: var(--sp-5);
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: min(420px, calc(100vw - 2 * var(--sp-4)));
		padding: var(--sp-3) var(--sp-3) var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--tint), var(--surface-raised);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
		animation: rise var(--dur-slow) var(--ease) both;
	}

	.install.inline {
		position: static;
		animation: none;
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.install {
			animation: none;
		}
	}

	.ic {
		display: grid;
		place-items: center;
		flex: none;
		width: 38px;
		height: 38px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.txt {
		flex: 1;
		min-width: 0;
	}

	.title {
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
	}

	.hint {
		color: var(--text-muted);
		font-size: var(--fs-micro);
		line-height: 1.4;
	}

	.hint strong {
		color: var(--text);
	}

	.act {
		display: flex;
		align-items: center;
		gap: var(--sp-1);
	}

	@media (max-width: 767px) {
		.install:not(.inline) {
			right: var(--sp-4);
			left: var(--sp-4);
			width: auto;
			bottom: calc(var(--tabbar-h) + env(safe-area-inset-bottom, 0px) + var(--sp-3));
		}
	}
</style>
