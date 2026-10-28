<script lang="ts">
	// Accounts (admins): role, library access, parental control, last activity.
	import { onMount } from 'svelte';
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import { libraries } from '$lib/api/stores.svelte';
	import type { User } from '$lib/api/types';
	import Avatar from '$lib/components/Avatar.svelte';
	import Button from '$lib/components/Button.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Tag from '$lib/components/Tag.svelte';
	import UserDialog from '$lib/components/users/UserDialog.svelte';
	import { parentalLabel } from '$lib/components/users/parental';
	import { formatRelative, plural } from '$lib/format';
	import { hueFrom } from '$lib/media';
	import { ROLE_LABEL, session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	let users = $state<User[] | null>(null);
	let error = $state<unknown>(null);
	let editing = $state<User | null>(null);
	let dialogOpen = $state(false);
	let toDelete = $state<User | null>(null);
	let deleteOpen = $state(false);
	let deleting = $state(false);
	let deleteError = $state('');
	let now = $state(Date.now());

	async function load() {
		error = null;
		try {
			users = (await api.users()).sort((a, b) => Number(b.isAdmin) - Number(a.isAdmin) || a.name.localeCompare(b.name, 'fr'));
		} catch (err) {
			error = err;
		}
	}

	onMount(() => {
		load();
		libraries.ensure();
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});

	function access(u: User): string {
		if (u.isAdmin || u.allLibraries) return 'Toutes les bibliothèques';
		const names = u.libraryIds.map((id) => libraries.get(id)?.name).filter(Boolean) as string[];
		if (!names.length) return u.libraryIds.length ? plural(u.libraryIds.length, 'bibliothèque', 'bibliothèques') : 'Aucune bibliothèque';
		return names.length > 2 ? `${names.slice(0, 2).join(', ')} +${names.length - 2}` : names.join(', ');
	}

	function create() {
		editing = null;
		dialogOpen = true;
	}

	function edit(u: User) {
		editing = u;
		dialogOpen = true;
	}

	function saved(u: User, created: boolean) {
		if (!users) return;
		users = created ? [...users, u] : users.map((x) => (x.id === u.id ? u : x));
		if (u.id === session.user?.id) session.signedIn(u);
		toasts.show(created ? `Compte « ${u.name} » créé` : `Compte « ${u.name} » mis à jour`, {
			detail: created ? 'Communiquez-lui son nom et son mot de passe.' : undefined
		});
	}

	function askDelete(u: User) {
		toDelete = u;
		deleteError = '';
		deleteOpen = true;
	}

	async function confirmDelete() {
		if (!toDelete) return;
		const u = toDelete;
		deleting = true;
		deleteError = '';
		try {
			await api.deleteUser(u.id);
			deleteOpen = false;
			users = (users ?? []).filter((x) => x.id !== u.id);
			toasts.show(`Compte « ${u.name} » supprimé`);
		} catch (err) {
			deleteError =
				err instanceof ApiError && err.code === 'last_admin'
					? 'C’est le dernier administrateur du serveur : nommez d’abord un autre administrateur.'
					: errorMessage(err);
		} finally {
			deleting = false;
		}
	}

	const adminCount = $derived((users ?? []).filter((u) => u.isAdmin).length);
</script>

<svelte:head>
	<title>Utilisateurs — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<div>
			<p class="micro eyebrow">Réglages</p>
			<h1>Utilisateurs</h1>
			<p class="lead">Chaque compte a sa progression, ses favoris et ses préférences. Limitez l’accès à certaines bibliothèques ou par âge.</p>
		</div>
		<Button variant="primary" icon="plus" onclick={create}>Ajouter</Button>
	</header>

	{#if error && !users}
		<EmptyState icon="alert" title="Utilisateurs indisponibles" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !users}
		<div class="list">{#each [0, 1, 2] as i (i)}<Skeleton height="84px" radius="lg" />{/each}</div>
	{:else}
		<ul class="list" role="list">
			{#each users as u (u.id)}
				{@const me = u.id === session.user?.id}
				<li class="user panel">
					<Avatar name={u.name} hue={hueFrom(u.name)} size={44} />
					<div class="main">
						<p class="name">
							<span>{u.name}</span>
							{#if me}<span class="you">(vous)</span>{/if}
							<Tag variant={u.isAdmin ? 'accent' : 'surface'} icon={u.isAdmin ? 'shield' : undefined}>{ROLE_LABEL(u)}</Tag>
						</p>
						<dl class="facts">
							<div><dt>Accès</dt><dd>{access(u)}</dd></div>
							<div><dt>Contrôle parental</dt><dd>{parentalLabel(u)}</dd></div>
							<div><dt>Dernière activité</dt><dd>{me ? 'maintenant' : u.lastSeen ? formatRelative(u.lastSeen, now) : 'jamais connecté'}</dd></div>
						</dl>
					</div>
					<div class="actions">
						<Button icon="edit" onclick={() => edit(u)} aria-label="Modifier {u.name}">Modifier</Button>
						<Button
							variant="tertiary"
							icon="trash"
							onclick={() => askDelete(u)}
							aria-label="Supprimer {u.name}"
							title={u.isAdmin && adminCount <= 1 ? 'Dernier administrateur' : 'Supprimer'}
						/>
					</div>
				</li>
			{/each}
		</ul>
		{#if users.length === 1}
			<p class="tip"><Icon name="info" size={16} />Ajoutez un compte pour chaque membre du foyer : les enfants peuvent avoir un contrôle parental.</p>
		{/if}
	{/if}
</section>

<UserDialog bind:open={dialogOpen} user={editing} onsaved={saved} />

<ConfirmDialog bind:open={deleteOpen} title="Supprimer ce compte ?" confirmLabel="Supprimer" danger busy={deleting} onconfirm={confirmDelete}>
	<p>Le compte <strong>{toDelete?.name}</strong>, sa progression, ses favoris et ses collections personnelles seront supprimés. Ses appareils seront déconnectés.</p>
	{#if deleteError}<p class="del-error" role="alert">{deleteError}</p>{/if}
</ConfirmDialog>

<style>
	.settings {
		display: grid;
		gap: var(--sp-5);
		align-content: start;
		max-width: 1100px;
	}

	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		flex-wrap: wrap;
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.lead {
		margin-top: var(--sp-1);
		max-width: 64ch;
		color: var(--text-muted);
	}

	.list {
		display: grid;
		gap: var(--sp-3);
	}

	.user {
		display: flex;
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-4) var(--sp-5);
	}

	.main {
		display: grid;
		gap: var(--sp-2);
		flex: 1;
		min-width: 0;
	}

	.name {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-2);
		font-weight: var(--fw-bold);
		font-size: 1rem;
	}

	.you {
		color: var(--text-muted);
		font-weight: var(--fw-medium);
		font-size: var(--fs-small);
	}

	.facts {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--sp-2) var(--sp-5);
	}

	.facts div {
		display: grid;
		min-width: 0;
	}

	dt {
		font-size: var(--fs-micro);
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	dd {
		margin: 0;
		font-size: var(--fs-small);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: var(--sp-1);
		flex: none;
	}

	.tip {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.del-error {
		margin-top: var(--sp-3);
		color: var(--danger);
	}

	@media (max-width: 899px) {
		.facts {
			grid-template-columns: minmax(0, 1fr);
			gap: 4px;
		}

		.facts div {
			grid-template-columns: 130px minmax(0, 1fr);
			gap: var(--sp-2);
		}
	}

	@media (max-width: 767px) {
		.user {
			flex-wrap: wrap;
			align-items: flex-start;
			padding: var(--sp-4);
		}

		.main {
			flex-basis: calc(100% - 60px);
		}

		.actions {
			width: 100%;
			justify-content: flex-end;
		}

		.facts div {
			grid-template-columns: minmax(0, 1fr);
			gap: 0;
		}
	}
</style>
