<script lang="ts">
	// Image holder: paints the dominant colour first, then fades the image (or generated art) in.
	import type { Snippet } from 'svelte';

	interface Props {
		src?: string;
		alt?: string;
		color?: string;
		art?: Snippet;
		ratio: string;
	}

	let { src, alt = '', color, art, ratio }: Props = $props();

	let loaded = $state(false);

	// Generated art has no network delay; still reveal it with the same fade, on the next frame.
	$effect(() => {
		if (src || !art) return;
		const raf = requestAnimationFrame(() => (loaded = true));
		return () => cancelAnimationFrame(raf);
	});
</script>

<div class="artwork" style:aspect-ratio={ratio} style:background-color={color ?? 'var(--surface)'}>
	{#if src}
		<img {src} {alt} loading="lazy" decoding="async" class:loaded onload={() => (loaded = true)} />
	{:else if art}
		<div class="layer" class:loaded>{@render art()}</div>
	{/if}
</div>

<style>
	.artwork {
		position: relative;
		width: 100%;
		overflow: hidden;
	}

	img,
	.layer {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		opacity: 0;
		transition: opacity var(--dur-slow) var(--ease);
	}

	.loaded {
		opacity: 1;
	}
</style>
