<script lang="ts">
	// Create / edit an account (admins): name, password (create or reset), role, library access, parental control.
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import { libraries } from '$lib/api/stores.svelte';
	import type { User, UserInput } from '$lib/api/types';
	import { libraryKindInfo } from '$lib/media';
	import { session } from '$lib/session.svelte';
	import PasswordField from '../auth/PasswordField.svelte';
	import PasswordStrength from '../auth/PasswordStrength.svelte';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import Input from '../Input.svelte';
	import Select from '../Select.svelte';
	import Switch from '../Switch.svelte';
	import { RATING_OPTIONS, toAge } from './parental';

	interface Props {
		open: boolean;
		/** Edit this account; create one otherwise. */
		user?: User | null;
		onsaved?: (u: User, created: boolean) => void;
	}

	let { open = $bindable(false), user = null, onsaved }: Props = $props();

	let name = $state('');
	let password = $state('');
	let resetting = $state(false);
	let isAdmin = $state(false);
	let allLibraries = $state(true);
	let libraryIds = $state<number[]>([]);
	let age = $state('');
	let blockUnrated = $state(false);
	let busy = $state(false);
	let error = $state('');

	const editing = $derived(!!user);
	const self = $derived(!!user && user.id === session.user?.id);

	$effect(() => {
		if (!open) return;
		name = user?.name ?? '';
		password = '';
		resetting = false;
		isAdmin = user?.isAdmin ?? false;
		allLibraries = user?.allLibraries ?? true;
		libraryIds = [...(user?.libraryIds ?? [])];
		age = user?.maxRatingAge === undefined || user?.maxRatingAge === null ? '' : String(user.maxRatingAge);
		blockUnrated = user?.blockUnrated ?? false;
		error = '';
		libraries.ensure();
	});

	function toggleLib(id: number, on: boolean) {
		libraryIds = on ? [...new Set([...libraryIds, id])] : libraryIds.filter((x) => x !== id);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!name.trim()) return (error = 'Le nom est requis.');
		if ((!editing || resetting) && password.length < 8) return (error = 'Le mot de passe doit faire au moins 8 caractères.');
		if (!isAdmin && !allLibraries && !libraryIds.length) return (error = 'Choisissez au moins une bibliothèque, ou « Toutes les bibliothèques ».');
		const body: UserInput = {
			name: name.trim(),
			isAdmin,
			allLibraries: isAdmin || allLibraries,
			libraryIds: isAdmin || allLibraries ? [] : libraryIds,
			maxRatingAge: toAge(age),
			blockUnrated
		};
		if (!editing || resetting) body.password = password;
		if (!editing && body.maxRatingAge === null) delete body.maxRatingAge;
		busy = true;
		try {
			const saved = user ? await api.patchUser(user.id, body) : await api.createUser(body);
			onsaved?.(saved, !user);
			open = false;
		} catch (err) {
			error = err instanceof ApiError && err.code === 'last_admin' ? 'C’est le dernier administrateur : il doit le rester.' : errorMessage(err);
		} finally {
			busy = false;
		}
	}
</script>

<Dialog bind:open title={user ? `Modifier « ${user.name} »` : 'Nouvel utilisateur'} description={user ? undefined : 'Un compte par personne : chacun garde sa progression, ses favoris et ses préférences.'} size="md" initialFocus="#user-name">
	<form id="user-form" class="form" onsubmit={submit} novalidate>
		<Input id="user-name" label="Nom" bind:value={name} maxlength={64} autocomplete="off" autocapitalize="words" required />

		{#if editing && !resetting}
			<div class="reset">
				<span><Icon name="key" size={16} />Mot de passe défini</span>
				<Button variant="tertiary" onclick={() => (resetting = true)}>Réinitialiser…</Button>
			</div>
		{:else}
			<div class="pw">
				<PasswordField
					label={editing ? 'Nouveau mot de passe' : 'Mot de passe'}
					size="md"
					bind:value={password}
					autocomplete="new-password"
					aria-describedby="user-pw-strength"
				/>
				<PasswordStrength {password} id="user-pw-strength" />
				{#if editing}<p class="note">La personne sera déconnectée de tous ses appareils.</p>{/if}
			</div>
		{/if}

		<div class="group">
			<Switch label="Administrateur" hint="Gère les bibliothèques, les utilisateurs et les métadonnées." bind:checked={isAdmin} disabled={self && isAdmin} />
			{#if self}<p class="note">Vous ne pouvez pas retirer vos propres droits d’administrateur.</p>{/if}
		</div>

		<fieldset class="group" disabled={isAdmin}>
			<legend>Accès aux bibliothèques</legend>
			{#if isAdmin}
				<p class="note">Les administrateurs voient toutes les bibliothèques.</p>
			{:else}
				<div class="seg" role="radiogroup" aria-label="Accès aux bibliothèques">
					<label class:on={allLibraries}><input type="radio" name="access" checked={allLibraries} onchange={() => (allLibraries = true)} />Toutes les bibliothèques</label>
					<label class:on={!allLibraries}><input type="radio" name="access" checked={!allLibraries} onchange={() => (allLibraries = false)} />Certaines seulement</label>
				</div>
				{#if !allLibraries}
					<ul class="libs" role="list">
						{#each libraries.list as lib (lib.id)}
							<li>
								<label class="lib">
									<input type="checkbox" checked={libraryIds.includes(lib.id)} onchange={(e) => toggleLib(lib.id, e.currentTarget.checked)} />
									<Icon name={libraryKindInfo(lib.kind).icon} size={18} />
									<span>{lib.name}</span>
								</label>
							</li>
						{:else}
							<li class="note">Aucune bibliothèque pour l’instant.</li>
						{/each}
					</ul>
				{/if}
			{/if}
		</fieldset>

		<fieldset class="group">
			<legend>Contrôle parental</legend>
			<Select label="Classification maximale" showLabel bind:value={age} options={RATING_OPTIONS} />
			<Switch label="Masquer les contenus non classés" hint="Les titres sans classification d’âge sont aussi cachés." bind:checked={blockUnrated} />
			<p class="note">Les titres au-dessus de la limite sont masqués partout : accueil, recherche, bibliothèques et lecture.</p>
		</fieldset>

		{#if error}<p class="error" role="alert"><Icon name="alert" size={16} />{error}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="tertiary" onclick={() => (open = false)}>Annuler</Button>
		<Button variant="primary" type="submit" form="user-form" disabled={busy}>{user ? 'Enregistrer' : 'Créer le compte'}</Button>
	{/snippet}
</Dialog>

<style>
	.form {
		display: grid;
		gap: var(--sp-5);
	}

	.pw,
	.group {
		display: grid;
		gap: var(--sp-3);
	}

	fieldset.group {
		margin: 0;
		padding: var(--sp-4) 0 0;
		border: 0;
		border-top: 1px solid var(--line);
		min-width: 0;
	}

	legend {
		float: left;
		width: 100%;
		margin-bottom: var(--sp-3);
		font-weight: var(--fw-bold);
	}

	legend + :global(*) {
		clear: both;
	}

	.reset {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
		padding: var(--sp-2) var(--sp-2) var(--sp-2) var(--sp-4);
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.reset span {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
	}

	.note {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	.seg {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 4px;
		padding: 4px;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.seg label {
		display: grid;
		place-items: center;
		height: 34px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-full);
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		text-align: center;
		cursor: pointer;
	}

	.seg label.on {
		background: var(--surface-active);
		color: var(--text);
	}

	.seg label:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 1px;
	}

	.seg input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.libs {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
		gap: var(--sp-2);
	}

	.lib {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		height: 44px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		cursor: pointer;
	}

	.lib:has(input:checked) {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 40%, transparent);
	}

	.lib :global(svg) {
		color: var(--text-muted);
	}

	.lib input {
		width: 16px;
		height: 16px;
		accent-color: var(--accent);
	}

	.lib span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.error {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--danger);
		font-size: var(--fs-small);
	}

	@media (max-width: 479px) {
		.seg {
			grid-template-columns: minmax(0, 1fr);
			border-radius: var(--r-lg);
		}
	}
</style>
