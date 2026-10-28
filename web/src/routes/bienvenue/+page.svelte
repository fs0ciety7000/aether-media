<script lang="ts">
	// First run (system/info.setupRequired): create the administrator, then a first library.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { errorMessage } from '$lib/api/client';
	import { libraries, scans } from '$lib/api/stores.svelte';
	import type { Library } from '$lib/api/types';
	import AuthShell from '$lib/components/auth/AuthShell.svelte';
	import PasswordField from '$lib/components/auth/PasswordField.svelte';
	import PasswordStrength from '$lib/components/auth/PasswordStrength.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Input from '$lib/components/Input.svelte';
	import AddLibraryDialog from '$lib/components/settings/AddLibraryDialog.svelte';
	import { session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	type Step = 'loading' | 'account' | 'library';
	let step = $state<Step>('loading');

	let name = $state('');
	let password = $state('');
	let confirm = $state('');
	let busy = $state(false);
	let error = $state('');
	let touched = $state(false);
	let addOpen = $state(false);

	const mismatch = $derived(touched && !!confirm && confirm !== password);
	const tooShort = $derived(touched && password.length > 0 && password.length < 8);

	onMount(async () => {
		await session.init();
		if (session.status === 'setup') step = 'account';
		// Already configured: this page has nothing to offer.
		else goto(session.status === 'authenticated' ? '/' : '/connexion', { replaceState: true });
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		touched = true;
		error = '';
		if (!name.trim()) return (error = 'Choisissez un nom pour le compte administrateur.');
		if (password.length < 8) return (error = 'Le mot de passe doit faire au moins 8 caractères.');
		if (password !== confirm) return (error = 'Les deux mots de passe ne correspondent pas.');
		busy = true;
		try {
			await session.setup(name.trim(), password);
			step = 'library';
			libraries.ensure();
			scans.ensure();
		} catch (err) {
			error = errorMessage(err);
		} finally {
			busy = false;
		}
	}

	function created(lib: Library) {
		scans.queued(lib.id);
		libraries.refresh();
		toasts.show(`Bibliothèque « ${lib.name} » créée`, { detail: 'L’analyse a commencé : les titres apparaissent au fil de l’eau.' });
		goto('/', { replaceState: true });
	}

	const STEPS = [
		{ id: 'account', label: 'Compte administrateur' },
		{ id: 'library', label: 'Première bibliothèque' },
		{ id: 'done', label: 'C’est parti' }
	] as const;
	const current = $derived(step === 'library' ? 1 : 0);
</script>

<svelte:head>
	<title>Bienvenue — Aether</title>
</svelte:head>

<AuthShell wide>
	{#if step !== 'loading'}
		<section class="welcome" aria-labelledby="welcome-title">
			<div class="intro">
				<p class="micro eyebrow">Premier lancement</p>
				<h1 id="welcome-title">Bienvenue dans Aether</h1>
				<p class="lead">Deux minutes pour préparer votre médiathèque : un compte administrateur, puis vos dossiers de films, séries ou musique.</p>
				<ol class="steps" aria-label="Étapes">
					{#each STEPS as s, i (s.id)}
						<li class:done={i < current} class:active={i === current} aria-current={i === current ? 'step' : undefined}>
							<span class="dot tabular" aria-hidden="true">{#if i < current}<Icon name="check" size={14} />{:else}{i + 1}{/if}</span>
							<span>{s.label}</span>
							{#if i < current}<span class="visually-hidden">(terminé)</span>{/if}
						</li>
					{/each}
				</ol>
			</div>

			<div class="card">
				{#if step === 'account'}
					<h2>Créer le compte administrateur</h2>
					<p class="muted">Ce compte gère les bibliothèques et les autres utilisateurs. Vous pourrez inviter votre famille ensuite.</p>
					<form class="form" onsubmit={submit} novalidate>
						<Input label="Nom" bind:value={name} size="lg" autocomplete="username" autocapitalize="words" maxlength={64} placeholder="Camille" required />
						<div class="pw">
							<PasswordField label="Mot de passe" bind:value={password} autocomplete="new-password" required aria-invalid={tooShort} aria-describedby="pw-strength" />
							<PasswordStrength password={password} id="pw-strength" />
						</div>
						<PasswordField
							label="Confirmation"
							bind:value={confirm}
							autocomplete="new-password"
							required
							aria-invalid={mismatch}
							hint={mismatch ? 'Les deux mots de passe diffèrent.' : undefined}
							onblur={() => (touched = true)}
						/>
						{#if error}<p class="error" role="alert"><Icon name="alert" size={16} />{error}</p>{/if}
						<Button variant="primary" size="lg" type="submit" disabled={busy} aria-busy={busy} icon="arrow-right">
							{busy ? 'Création…' : 'Créer le compte'}
						</Button>
					</form>
				{:else}
					<span class="ok" aria-hidden="true"><Icon name="check" size={22} /></span>
					<h2>Bonjour {session.user?.name} !</h2>
					<p class="muted">Votre compte est prêt et cet appareil est connecté. Indiquez maintenant où vivent vos fichiers : Aether les analyse et récupère affiches et résumés.</p>
					<ul class="kinds" role="list">
						<li><Icon name="film" size={18} />Films</li>
						<li><Icon name="tv" size={18} />Séries</li>
						<li><Icon name="music" size={18} />Musique</li>
					</ul>
					<div class="actions">
						<Button variant="primary" size="lg" icon="plus" onclick={() => (addOpen = true)}>Ajouter une bibliothèque</Button>
						<Button variant="tertiary" size="lg" href="/">Plus tard</Button>
					</div>
				{/if}
			</div>
		</section>
	{/if}
</AuthShell>

{#if step === 'library'}
	<AddLibraryDialog bind:open={addOpen} oncreated={created} />
{/if}

<style>
	.welcome {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 440px);
		gap: clamp(var(--sp-6), 6vw, 88px);
		align-items: center;
		width: 100%;
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	h1 {
		margin-top: var(--sp-2);
		font-size: clamp(2rem, 1.4rem + 2vw, 2.75rem);
		line-height: 1.1;
		font-weight: var(--fw-heavy);
		letter-spacing: -0.025em;
	}

	.lead {
		margin-top: var(--sp-4);
		max-width: 46ch;
		color: var(--text-muted);
		font-size: 0.9375rem;
	}

	.steps {
		display: grid;
		gap: var(--sp-3);
		margin-top: var(--sp-6);
	}

	.steps li {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	.steps li.active {
		color: var(--text);
		font-weight: var(--fw-bold);
	}

	.dot {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: 50%;
		box-shadow: inset 0 0 0 1px var(--line-strong);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
	}

	.active .dot {
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: none;
	}

	.done .dot {
		background: var(--accent-soft);
		color: var(--accent-ink);
		box-shadow: none;
	}

	.card {
		display: grid;
		gap: var(--sp-3);
		padding: var(--sp-6);
		border-radius: var(--r-xl);
		background: var(--tint), var(--surface);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
	}

	h2 {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
	}

	.muted {
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.form {
		display: grid;
		gap: var(--sp-4);
		margin-top: var(--sp-3);
	}

	.pw {
		display: grid;
		gap: var(--sp-2);
	}

	.form > :global(.btn) {
		width: 100%;
		flex-direction: row-reverse;
	}

	.error {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--danger);
		font-size: var(--fs-small);
	}

	.ok {
		display: grid;
		place-items: center;
		width: 44px;
		height: 44px;
		border-radius: 50%;
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.kinds {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		margin: var(--sp-2) 0;
	}

	.kinds li {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
		height: 34px;
		padding: 0 14px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
	}

	.kinds :global(svg) {
		color: var(--accent);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-3);
		margin-top: var(--sp-2);
	}

	@media (max-width: 899px) {
		.welcome {
			grid-template-columns: minmax(0, 1fr);
			max-width: 480px;
			margin: 0 auto;
		}

		.steps {
			display: flex;
			flex-wrap: wrap;
			gap: var(--sp-2) var(--sp-4);
			margin-top: var(--sp-5);
		}

		.steps li {
			font-size: var(--fs-small);
		}
	}

	@media (max-width: 767px) {
		.card {
			padding: var(--sp-5);
		}

		.actions :global(.btn) {
			flex: 1;
		}
	}
</style>
