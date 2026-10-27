<script lang="ts">
	interface Props {
		width?: string;
		height?: string;
		/** CSS aspect-ratio, e.g. "2 / 3" for a poster. Overrides height. */
		ratio?: string;
		radius?: 'sm' | 'md' | 'lg' | 'full';
		/** Text-line shorthand: n lines with a shorter last line. */
		lines?: number;
		class?: string;
	}

	let { width = '100%', height = '1em', ratio, radius = 'md', lines, class: className }: Props = $props();
</script>

{#if lines}
	<div class={['lines', className]} style:width aria-hidden="true">
		{#each Array.from({ length: lines }, (_, i) => i) as i (i)}
			<span
				class="sk r-sm"
				style:width={i === lines - 1 && lines > 1 ? '62%' : '100%'}
				style:animation-delay="{i * 140}ms"
			></span>
		{/each}
	</div>
{:else}
	<span
		class={['sk', `r-${radius}`, className]}
		style:width
		style:height={ratio ? 'auto' : height}
		style:aspect-ratio={ratio}
		aria-hidden="true"
	></span>
{/if}

<style>
	.sk {
		display: block;
		/* Surface tone that reads on both dark and light grounds. */
		background: color-mix(in srgb, var(--surface) 94%, var(--text) 6%);
		animation: breathe 2.8s var(--ease) infinite alternate;
	}

	.lines {
		display: grid;
		gap: 0.6em;
	}

	.lines .sk {
		height: 0.75em;
	}

	.r-sm {
		border-radius: var(--r-sm);
	}
	.r-md {
		border-radius: var(--r-md);
	}
	.r-lg {
		border-radius: var(--r-lg);
	}
	.r-full {
		border-radius: var(--r-full);
	}

	/* Slow breathing — opacity only, no shimmer. */
	@keyframes breathe {
		from {
			opacity: 1;
		}
		to {
			opacity: 0.45;
		}
	}
</style>
