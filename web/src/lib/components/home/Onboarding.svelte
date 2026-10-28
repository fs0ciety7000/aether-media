<script lang="ts">
	// First run: no library yet. A welcoming panel with one clear action.
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';

	interface Props {
		/** Members cannot add libraries: they wait for an administrator. */
		admin?: boolean;
	}

	let { admin = true }: Props = $props();

	const steps = [
		{ icon: 'folder', title: 'Choisissez un dossier', text: 'Films, séries ou musique : indiquez où vivent vos fichiers.' },
		{ icon: 'refresh', title: 'Aether analyse', text: 'Fichiers, NFO et images locales sont lus ; la progression s’affiche en direct.' },
		{ icon: 'play', title: 'Parcourez', text: 'Affiches, fiches détaillées et recherche instantanée, sur tous vos écrans.' }
	] as const;
</script>

<section class="onboarding panel" aria-labelledby="onb-title">
	<div class="intro">
		<p class="micro eyebrow">{admin ? 'Premier lancement' : 'Bienvenue'}</p>
		<h1 id="onb-title">Bienvenue dans Aether</h1>
		{#if admin}
			<p class="lead">Votre médiathèque est encore vide. Ajoutez une première bibliothèque : Aether s’occupe du reste.</p>
			<div class="actions">
				<Button variant="primary" size="lg" icon="plus" href="/reglages/bibliotheques?ajouter=1">Ajouter une bibliothèque</Button>
			</div>
		{:else}
			<p class="lead">Aucune bibliothèque n’est encore partagée avec vous. Demandez à un administrateur de ce serveur de vous y donner accès.</p>
		{/if}
	</div>
	<ol class="steps" role="list">
		{#each steps as step, i (step.title)}
			<li class="step">
				<span class="num tabular" aria-hidden="true">{i + 1}</span>
				<span class="icon" aria-hidden="true"><Icon name={step.icon} size={20} /></span>
				<span class="text">
					<strong>{step.title}</strong>
					<span>{step.text}</span>
				</span>
			</li>
		{/each}
	</ol>
</section>

<style>
	.onboarding {
		display: grid;
		grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr);
		gap: var(--sp-7);
		align-items: center;
		padding: clamp(var(--sp-5), 5vw, var(--sp-8));
		overflow: hidden;
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	h1 {
		margin-top: var(--sp-2);
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		font-weight: var(--fw-heavy);
		letter-spacing: -0.02em;
	}

	.lead {
		margin-top: var(--sp-3);
		max-width: 44ch;
		color: var(--text-muted);
		font-size: 0.9375rem;
	}

	.actions {
		margin-top: var(--sp-5);
	}

	.steps {
		display: grid;
		gap: var(--sp-3);
	}

	.step {
		position: relative;
		display: flex;
		align-items: flex-start;
		gap: var(--sp-4);
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: color-mix(in srgb, var(--bg) 55%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.num {
		position: absolute;
		top: var(--sp-3);
		right: var(--sp-4);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		color: var(--text-faint);
	}

	.icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 42px;
		height: 42px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.text {
		display: grid;
		gap: 2px;
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.text strong {
		color: var(--text);
		font-size: var(--fs-body);
	}

	@media (max-width: 1023px) {
		.onboarding {
			grid-template-columns: minmax(0, 1fr);
			gap: var(--sp-6);
		}
	}

	@media (max-width: 767px) {
		.actions :global(.btn) {
			width: 100%;
		}
	}
</style>
