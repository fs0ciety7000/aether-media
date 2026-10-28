<script lang="ts">
	// Signed-in user in the top bar: avatar, name and role; menu with Profil, Ma liste, Synchronisation, Réglages (admins), Se déconnecter.
	import { goto } from '$app/navigation';
	import { hueFrom } from '$lib/media';
	import { ROLE_LABEL, session } from '$lib/session.svelte';
	import Avatar from '../Avatar.svelte';
	import Menu, { type MenuItem } from '../Menu.svelte';
	import UserChip from '../UserChip.svelte';

	interface Props {
		/** Avatar only (narrow screens). */
		compact?: boolean;
	}

	let { compact = false }: Props = $props();

	const user = $derived(session.user);
	const role = $derived(ROLE_LABEL(user));
	const hue = $derived(hueFrom(user?.name ?? ''));

	async function logout() {
		await session.logout();
		goto('/connexion', { replaceState: true });
	}

	const items = $derived<MenuItem[]>([
		{ label: 'Profil', icon: 'user', onselect: () => goto('/profil') },
		{ label: 'Ma liste', icon: 'bookmark', onselect: () => goto('/ma-liste') },
		{ label: 'Synchronisation', icon: 'refresh', onselect: () => goto('/profil/synchronisation') },
		...(session.isAdmin ? [{ label: 'Réglages', icon: 'settings' as const, onselect: () => goto('/reglages/bibliotheques') }] : []),
		{ label: 'Se déconnecter', icon: 'log-out', onselect: logout, separated: true }
	]);
</script>

{#if user}
	<div class="user-menu" class:compact>
		<Menu label="Compte de {user.name}" labelFromContent={!compact} {items} variant="bare" align="end">
			{#snippet trigger()}
				<UserChip name={user.name} detail={compact ? undefined : role} {hue} {compact} href={null} />
			{/snippet}
			{#snippet header()}
				<span class="who">
					<Avatar name={user.name} {hue} size={36} />
					<span class="text">
						<span class="name">{user.name}</span>
						<span class="role">{role}</span>
					</span>
				</span>
			{/snippet}
		</Menu>
	</div>
{/if}

<style>
	.user-menu {
		min-width: 0;
	}

	.user-menu :global(.trigger.bare) {
		max-width: 100%;
		text-align: left;
	}

	.user-menu :global(.trigger.bare .user:hover) {
		background: none;
	}

	.who {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		min-width: 0;
	}

	.text {
		display: grid;
		min-width: 0;
		line-height: 1.3;
	}

	.name {
		font-weight: var(--fw-bold);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.role {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	/* The menu already names the user on wide screens: the header is for the compact trigger. */
	.user-menu:not(.compact) :global(.list .header) {
		display: none;
	}
</style>
