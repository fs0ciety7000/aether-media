<script lang="ts">
	// Aether monogram: an "A" folded from two facets (like a ribbon of film turning), crossbar inset.
	interface Props {
		size?: number;
		/** Show the "Aether" wordmark next to the mark. */
		wordmark?: boolean;
		class?: string;
	}

	let { size = 30, wordmark = true, class: className }: Props = $props();
</script>

<span class={['logo', className]}>
	<svg class="mark" width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
		<path class="back" d="M13 4h6l10 24h-6Z" />
		<path class="front" d="M13 4h6L9 28H3Z" />
		<path class="bar" d="M12.54 19.5h6.92l1.25 3H11.29Z" />
	</svg>
	{#if wordmark}<span class="word">Aether</span>{/if}
</span>

<style>
	.logo {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		color: var(--text);
	}

	.mark {
		flex: none;
	}

	.front {
		fill: var(--accent);
	}

	.back,
	.bar {
		fill: color-mix(in srgb, var(--accent) 58%, var(--bg));
	}

	.word {
		font-size: 1.375rem;
		font-weight: var(--fw-heavy);
		letter-spacing: -0.01em;
		line-height: 1;
		white-space: nowrap;
	}
</style>
