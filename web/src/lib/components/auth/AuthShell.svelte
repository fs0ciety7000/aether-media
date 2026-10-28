<script lang="ts">
	// Full-page frame for the pages without the app shell (connexion, first run):
	// a calm night ground with soft teal light and drifting "poster" silhouettes, the card centred.
	import type { Snippet } from 'svelte';
	import Logo from '../brand/Logo.svelte';

	interface Props {
		children: Snippet;
		/** Wider layout (first-run assistant). */
		wide?: boolean;
	}

	let { children, wide = false }: Props = $props();
	const tiles = Array.from({ length: 14 }, (_, i) => i);
</script>

<div class="auth" class:wide>
	<div class="ground" aria-hidden="true">
		<div class="wall">
			{#each tiles as i (i)}<span style:--i={i}></span>{/each}
		</div>
	</div>
	<header class="top">
		<Logo size={30} />
	</header>
	<main id="contenu" class="center" tabindex="-1">
		{@render children()}
	</main>
	<footer class="foot">Aether · votre médiathèque, chez vous</footer>
</div>

<style>
	.auth {
		position: relative;
		isolation: isolate;
		display: grid;
		grid-template-rows: auto 1fr auto;
		min-height: 100vh;
		min-height: 100dvh;
		padding: var(--sp-5) var(--gutter);
		background: var(--bg);
		overflow: hidden;
	}

	.auth :global(main:focus) {
		outline: none;
	}

	.ground {
		position: absolute;
		inset: 0;
		z-index: -1;
		background:
			radial-gradient(60% 55% at 18% 12%, color-mix(in srgb, var(--accent) 16%, transparent), transparent 70%),
			radial-gradient(50% 50% at 90% 95%, color-mix(in srgb, var(--accent) 9%, transparent), transparent 70%),
			var(--bg);
	}

	/* A tilted wall of blank poster frames: suggests a library without showing any content. */
	.wall {
		position: absolute;
		top: -10%;
		right: -12%;
		display: grid;
		grid-template-columns: repeat(7, 120px);
		gap: 18px;
		transform: rotate(-12deg);
		opacity: 0.55;
		mask-image: radial-gradient(70% 70% at 70% 40%, #000 20%, transparent 75%);
	}

	.wall span {
		aspect-ratio: 2 / 3;
		border-radius: var(--r-lg);
		background:
			linear-gradient(160deg, color-mix(in srgb, var(--accent) calc(4% + var(--i) * 0.6%), var(--surface)), var(--bg-sunken));
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.wall span:nth-child(odd) {
		transform: translateY(50%);
	}

	.top {
		display: flex;
		justify-content: center;
		padding: var(--sp-3) 0 var(--sp-6);
	}

	.center {
		display: grid;
		place-items: center;
		align-content: center;
		width: 100%;
		max-width: 440px;
		margin: 0 auto;
	}

	.wide .center {
		max-width: 980px;
	}

	.foot {
		padding-top: var(--sp-6);
		text-align: center;
		font-size: var(--fs-micro);
		color: var(--text-faint);
	}

	@media (max-width: 767px) {
		.auth {
			padding: var(--sp-4) var(--sp-4);
		}

		.top {
			padding-bottom: var(--sp-5);
		}

		.wall {
			grid-template-columns: repeat(5, 90px);
			right: -40%;
			opacity: 0.4;
		}
	}
</style>
