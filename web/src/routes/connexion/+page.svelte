<script lang="ts">
	// Sign-in (no app shell). The session cookie keeps this device signed in.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, errorMessage } from '$lib/api/client';
	import AuthShell from '$lib/components/auth/AuthShell.svelte';
	import PasswordField from '$lib/components/auth/PasswordField.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Input from '$lib/components/Input.svelte';
	import { safeReturn, session } from '$lib/session.svelte';

	let name = $state('');
	let password = $state('');
	let busy = $state(false);
	let error = $state<{ title: string; text: string; limited?: boolean } | null>(null);
	let nameEl = $state<HTMLInputElement>();
	let passwordEl = $state<HTMLInputElement>();

	const target = $derived(safeReturn(page.url.searchParams.get('retour')));
	const expired = $derived(page.url.searchParams.has('retour'));

	onMount(async () => {
		await session.init();
		if (session.status === 'authenticated') goto(target, { replaceState: true });
		else if (session.status === 'setup') goto('/bienvenue', { replaceState: true });
		else nameEl?.focus();
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (busy) return;
		if (!name.trim() || !password) {
			error = { title: 'Champs manquants', text: 'Indiquez votre nom et votre mot de passe.' };
			(!name.trim() ? nameEl : passwordEl)?.focus();
			return;
		}
		busy = true;
		error = null;
		try {
			await session.login(name.trim(), password);
			goto(target, { replaceState: true });
		} catch (err) {
			password = '';
			if (err instanceof ApiError && err.status === 429) {
				error = { title: 'Trop de tentatives', text: 'Par sécurité, les connexions depuis cet appareil sont suspendues quelques minutes. Réessayez plus tard.', limited: true };
			} else if (err instanceof ApiError && err.status === 401) {
				error = { title: 'Identifiants incorrects', text: 'Le nom ou le mot de passe ne correspond à aucun compte. Vérifiez la saisie (majuscules comprises).' };
				passwordEl?.focus();
			} else {
				error = { title: 'Connexion impossible', text: errorMessage(err) };
			}
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Connexion — Aether</title>
</svelte:head>

<AuthShell>
	<section class="card login-card" aria-labelledby="login-title">
		<h1 id="login-title">Connexion</h1>
		<p class="lead">
			{expired ? 'Connectez-vous pour continuer : vous reviendrez directement à la page demandée.' : 'Heureux de vous revoir. Connectez-vous pour retrouver votre médiathèque.'}
		</p>

		{#if error}
			<div class="alert" role="alert">
				<Icon name={error.limited ? 'clock' : 'alert'} size={18} />
				<div>
					<p class="alert-title">{error.title}</p>
					<p>{error.text}</p>
				</div>
			</div>
		{/if}

		<form class="form" onsubmit={submit} novalidate>
			<Input
				label="Nom"
				bind:value={name}
				bind:element={nameEl}
				size="lg"
				autocomplete="username"
				autocapitalize="off"
				spellcheck="false"
				required
				aria-invalid={!!error && !error.limited}
			/>
			<PasswordField label="Mot de passe" bind:value={password} autocomplete="current-password" required aria-invalid={!!error && !error.limited} />
			<p class="device">
				<Icon name="shield" size={16} />
				<span>Cet appareil restera connecté jusqu’à ce que vous vous déconnectiez.</span>
			</p>
			<Button variant="primary" size="lg" type="submit" disabled={busy || !!error?.limited} aria-busy={busy}>
				{busy ? 'Connexion…' : 'Se connecter'}
			</Button>
		</form>
		<p class="help">Mot de passe oublié ? Un administrateur peut le réinitialiser depuis Réglages › Utilisateurs.</p>
	</section>
</AuthShell>

<style>
	.card {
		display: grid;
		gap: var(--sp-4);
		width: 100%;
		padding: var(--sp-7) var(--sp-6) var(--sp-6);
		border-radius: var(--r-xl);
		background: var(--tint), var(--surface);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
	}

	h1 {
		font-size: 1.75rem;
		line-height: 1.2;
		font-weight: var(--fw-heavy);
		letter-spacing: -0.02em;
	}

	.lead {
		margin-top: calc(-1 * var(--sp-2));
		color: var(--text-muted);
	}

	.form {
		display: grid;
		gap: var(--sp-4);
		margin-top: var(--sp-2);
	}

	.form :global(.btn) {
		width: 100%;
		margin-top: var(--sp-1);
	}

	.device {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	.device :global(svg) {
		color: var(--accent);
		margin-top: 1px;
	}

	.alert {
		display: flex;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--danger) 12%, var(--surface));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 30%, transparent);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.alert :global(svg) {
		flex: none;
		color: var(--danger);
		margin-top: 2px;
	}

	.alert-title {
		color: var(--text);
		font-weight: var(--fw-bold);
	}

	.help {
		padding-top: var(--sp-4);
		box-shadow: 0 -1px 0 var(--line);
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
		text-align: center;
	}

	@media (max-width: 767px) {
		.card {
			padding: var(--sp-6) var(--sp-5) var(--sp-5);
		}
	}
</style>
