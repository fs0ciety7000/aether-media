<script lang="ts">
	// Profile: playback preferences, password, signed-in devices, appearance.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, errorMessage } from '$lib/api/client';
	import type { DeviceSession, SubtitleMode, UserPrefs } from '$lib/api/types';
	import PasswordField from '$lib/components/auth/PasswordField.svelte';
	import PasswordStrength from '$lib/components/auth/PasswordStrength.svelte';
	import Avatar from '$lib/components/Avatar.svelte';
	import Button from '$lib/components/Button.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Select from '$lib/components/Select.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Tag from '$lib/components/Tag.svelte';
	import ThemePicker from '$lib/components/ThemePicker.svelte';
	import { formatDate, formatRelative } from '$lib/format';
	import { hueFrom, PREF_LANGUAGES, SUBTITLE_MODES } from '$lib/media';
	import { ROLE_LABEL, session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	const user = $derived(session.user);

	// ——— Preferences ———
	let audioLanguage = $state('');
	let subtitleLanguage = $state('');
	let subtitleMode = $state<SubtitleMode>('auto');
	let savingPrefs = $state(false);

	function syncPrefs(p: UserPrefs | undefined) {
		audioLanguage = p?.audioLanguage ?? '';
		subtitleLanguage = p?.subtitleLanguage ?? '';
		subtitleMode = p?.subtitleMode ?? 'auto';
	}
	syncPrefs(session.user?.prefs);

	const dirty = $derived(
		audioLanguage !== (user?.prefs.audioLanguage ?? '') ||
			subtitleLanguage !== (user?.prefs.subtitleLanguage ?? '') ||
			subtitleMode !== (user?.prefs.subtitleMode ?? 'auto')
	);

	const langOptions = (none: string) => [{ value: '', label: none }, ...PREF_LANGUAGES];

	async function savePrefs(e: SubmitEvent) {
		e.preventDefault();
		savingPrefs = true;
		try {
			const u = await session.update({
				prefs: { audioLanguage: audioLanguage || undefined, subtitleLanguage: subtitleLanguage || undefined, subtitleMode }
			});
			syncPrefs(u.prefs);
			toasts.show('Préférences de lecture enregistrées', { detail: 'Elles s’appliquent à la prochaine lecture.' });
		} catch (err) {
			toasts.show('Enregistrement impossible', { detail: errorMessage(err) });
		} finally {
			savingPrefs = false;
		}
	}

	// ——— Password ———
	let current = $state('');
	let next = $state('');
	let confirm = $state('');
	let pwBusy = $state(false);
	let pwError = $state('');

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		pwError = '';
		if (!current) return (pwError = 'Indiquez votre mot de passe actuel.');
		if (next.length < 8) return (pwError = 'Le nouveau mot de passe doit faire au moins 8 caractères.');
		if (next !== confirm) return (pwError = 'Les deux nouveaux mots de passe ne correspondent pas.');
		pwBusy = true;
		try {
			await session.update({ currentPassword: current, newPassword: next });
			current = next = confirm = '';
			toasts.show('Mot de passe modifié', { detail: 'Vos autres appareils ont été déconnectés.' });
			loadSessions();
		} catch (err) {
			pwError = errorMessage(err);
		} finally {
			pwBusy = false;
		}
	}

	// ——— Sessions ———
	let sessions = $state<DeviceSession[] | null>(null);
	let sessionsError = $state('');
	let revoking = $state<DeviceSession | null>(null);
	let revokeOpen = $state(false);
	let revokeBusy = $state(false);

	async function loadSessions() {
		try {
			const list = await api.mySessions();
			sessions = [...list].sort((a, b) => Number(b.current) - Number(a.current) || b.lastSeen.localeCompare(a.lastSeen));
			sessionsError = '';
		} catch (err) {
			sessions ??= [];
			sessionsError = errorMessage(err);
		}
	}

	onMount(loadSessions);

	function askRevoke(s: DeviceSession) {
		revoking = s;
		revokeOpen = true;
	}

	async function confirmRevoke() {
		if (!revoking) return;
		const s = revoking;
		revokeBusy = true;
		try {
			await api.revokeSession(s.id);
			revokeOpen = false;
			if (s.current) {
				await session.logout();
				goto('/connexion', { replaceState: true });
				return;
			}
			sessions = (sessions ?? []).filter((x) => x.id !== s.id);
			toasts.show('Appareil déconnecté', { detail: s.device || s.client });
		} catch (err) {
			toasts.show('Révocation impossible', { detail: errorMessage(err) });
		} finally {
			revokeBusy = false;
		}
	}

	function deviceIcon(s: DeviceSession) {
		return /iOS|Android|iPhone|Mobile/i.test(`${s.device} ${s.client}`) ? 'smartphone' : 'monitor';
	}

	async function logout() {
		await session.logout();
		goto('/connexion', { replaceState: true });
	}
</script>

<svelte:head>
	<title>Profil — Aether</title>
</svelte:head>

{#if user}
	<div class="page">
		<header class="identity panel">
			<Avatar name={user.name} hue={hueFrom(user.name)} size={72} />
			<div class="who">
				<h1>{user.name}</h1>
				<p class="meta">
					<Tag variant={user.isAdmin ? 'accent' : 'surface'} icon={user.isAdmin ? 'shield' : 'user'}>{ROLE_LABEL(user)}</Tag>
					{#if user.createdAt}<span>Membre depuis le {formatDate(user.createdAt)}</span>{/if}
				</p>
			</div>
			<Button icon="log-out" onclick={logout}>Se déconnecter</Button>
		</header>

		<div class="cols">
			<section class="panel block" aria-labelledby="p-play">
				<h2 id="p-play">Lecture</h2>
				<p class="desc">Pistes choisies par défaut au lancement d’une vidéo. Un choix fait pendant la lecture reste prioritaire.</p>
				<form class="form" onsubmit={savePrefs}>
					<div class="two">
						<Select label="Langue audio préférée" showLabel bind:value={audioLanguage} options={langOptions('Celle du fichier')} />
						<Select label="Langue des sous-titres" showLabel bind:value={subtitleLanguage} options={langOptions('Comme l’audio')} />
					</div>
					<fieldset class="modes">
						<legend>Sous-titres</legend>
						{#each SUBTITLE_MODES as m (m.value)}
							<label class="mode" class:checked={subtitleMode === m.value}>
								<input type="radio" name="sub-mode" value={m.value} bind:group={subtitleMode} />
								<span class="radio" aria-hidden="true"></span>
								<span class="mode-text">
									<span class="mode-label">{m.label}</span>
									<span class="mode-hint">{m.hint}</span>
								</span>
							</label>
						{/each}
					</fieldset>
					<div class="row-end">
						{#if dirty}<span class="unsaved" aria-live="polite">Modifications non enregistrées</span>{/if}
						<Button variant="primary" type="submit" disabled={!dirty || savingPrefs}>Enregistrer</Button>
					</div>
				</form>
			</section>

			<section class="panel block" aria-labelledby="p-pw">
				<h2 id="p-pw">Mot de passe</h2>
				<p class="desc">Changer de mot de passe déconnecte vos autres appareils.</p>
				<form class="form" onsubmit={changePassword} novalidate>
					<input class="visually-hidden" type="text" autocomplete="username" value={user.name} readonly tabindex="-1" aria-hidden="true" />
					<PasswordField label="Mot de passe actuel" size="md" bind:value={current} autocomplete="current-password" />
					<div class="pw">
						<PasswordField label="Nouveau mot de passe" size="md" bind:value={next} autocomplete="new-password" aria-describedby="new-pw-strength" />
						<PasswordStrength password={next} id="new-pw-strength" />
					</div>
					<PasswordField
						label="Confirmation"
						size="md"
						bind:value={confirm}
						autocomplete="new-password"
						aria-invalid={!!confirm && confirm !== next}
						hint={confirm && confirm !== next ? 'Les deux mots de passe diffèrent.' : undefined}
					/>
					{#if pwError}<p class="error" role="alert"><Icon name="alert" size={16} />{pwError}</p>{/if}
					<div class="row-end">
						<Button type="submit" variant="primary" disabled={pwBusy}>Modifier le mot de passe</Button>
					</div>
				</form>
			</section>
		</div>

		<section class="panel block" aria-labelledby="p-sessions">
			<h2 id="p-sessions">Appareils connectés</h2>
			<p class="desc">Révoquez un appareil que vous n’utilisez plus ou qui ne vous appartient pas.</p>
			{#if sessions === null}
				<div class="sessions"><Skeleton height="60px" radius="md" /><Skeleton height="60px" radius="md" /></div>
			{:else}
				{#if sessionsError}<p class="error" role="alert">{sessionsError}</p>{/if}
				<ul class="sessions" role="list">
					{#each sessions as s (s.id)}
						<li class="session" class:current={s.current}>
							<span class="dev-icon" aria-hidden="true"><Icon name={deviceIcon(s)} size={20} /></span>
							<span class="dev-text">
								<span class="dev-name">
									{s.device || 'Appareil inconnu'}
									{#if s.current}<Tag variant="accent">Cet appareil</Tag>{/if}
								</span>
								<span class="dev-meta">
									{[s.client, s.current ? 'actif maintenant' : `vu ${formatRelative(s.lastSeen) ?? '—'}`, `connecté le ${formatDate(s.createdAt)}`].filter(Boolean).join(' · ')}
								</span>
							</span>
							<Button variant={s.current ? 'tertiary' : 'secondary'} onclick={() => askRevoke(s)} aria-label="Révoquer {s.device || 'cet appareil'}{s.current ? ' (cet appareil)' : ''}">Révoquer</Button>
						</li>
					{:else}
						<li class="muted">Aucune autre session.</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="panel block" aria-labelledby="p-theme">
			<h2 id="p-theme">Apparence</h2>
			<p class="desc">Propre à ce navigateur.</p>
			<ThemePicker />
		</section>
	</div>

	<ConfirmDialog bind:open={revokeOpen} title={revoking?.current ? 'Vous déconnecter ?' : 'Révoquer cet appareil ?'} confirmLabel="Révoquer" danger busy={revokeBusy} onconfirm={confirmRevoke}>
		{#if revoking?.current}
			<p>C’est <strong>cet appareil</strong> : vous serez déconnecté immédiatement.</p>
		{:else}
			<p><strong>{revoking?.device || 'Cet appareil'}</strong> devra se reconnecter avec votre mot de passe.</p>
		{/if}
	</ConfirmDialog>
{/if}

<style>
	.page {
		display: grid;
		gap: var(--gap);
		max-width: 1180px;
		padding: var(--sp-2) var(--gutter) var(--sp-9);
	}

	.identity {
		display: flex;
		align-items: center;
		gap: var(--sp-5);
		padding: var(--sp-5) var(--sp-6);
	}

	.who {
		display: grid;
		gap: var(--sp-2);
		flex: 1;
		min-width: 0;
	}

	.who h1 {
		font-size: 1.75rem;
		line-height: 1.2;
		overflow-wrap: anywhere;
	}

	.meta {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-2) var(--sp-3);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.cols {
		display: grid;
		grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
		gap: var(--gap);
		align-items: start;
	}

	.block {
		padding: var(--sp-5) var(--sp-6) var(--sp-6);
		min-width: 0;
	}

	.desc {
		margin-top: 2px;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.form {
		display: grid;
		gap: var(--sp-4);
		margin-top: var(--sp-5);
	}

	.two {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--sp-4);
	}

	.pw {
		display: grid;
		gap: var(--sp-2);
	}

	.modes {
		display: grid;
		gap: var(--sp-2);
		margin: 0;
		padding: 0;
		border: 0;
		min-width: 0;
	}

	legend {
		margin-bottom: var(--sp-2);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	.mode {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--bg) 40%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease);
	}

	.mode:hover {
		background: var(--surface-raised);
	}

	.mode.checked {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 55%, transparent);
	}

	.mode input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.mode:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	.radio {
		flex: none;
		width: 18px;
		height: 18px;
		margin-top: 1px;
		border-radius: 50%;
		box-shadow: inset 0 0 0 1.5px var(--text-faint);
	}

	.checked .radio {
		box-shadow: inset 0 0 0 5px var(--accent);
	}

	.mode-text {
		display: grid;
		gap: 2px;
	}

	.mode-label {
		font-weight: var(--fw-bold);
	}

	.mode-hint {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	.row-end {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: var(--sp-4);
		flex-wrap: wrap;
	}

	.unsaved {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.error {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--danger);
		font-size: var(--fs-small);
	}

	.sessions {
		display: grid;
		gap: var(--sp-2);
		margin-top: var(--sp-4);
	}

	.session {
		display: flex;
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-3) var(--sp-3) var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--bg) 40%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.session.current {
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 35%, transparent);
	}

	.dev-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 40px;
		height: 40px;
		border-radius: var(--r-sm);
		background: var(--surface-raised);
		color: var(--text-muted);
	}

	.current .dev-icon {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.dev-text {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: 0;
	}

	.dev-name {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-2);
		font-weight: var(--fw-bold);
	}

	.dev-meta {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.muted {
		color: var(--text-muted);
	}

	@media (max-width: 1023px) {
		.cols {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	@media (max-width: 767px) {
		.identity {
			flex-wrap: wrap;
			padding: var(--sp-5);
			gap: var(--sp-4);
		}

		.identity :global(.btn) {
			width: 100%;
		}

		.block {
			padding: var(--sp-5) var(--sp-4);
		}

		.two {
			grid-template-columns: minmax(0, 1fr);
		}

		.session {
			flex-wrap: wrap;
		}

		.session :global(.btn) {
			margin-left: calc(40px + var(--sp-4));
		}

		.row-end :global(.btn) {
			width: 100%;
		}
	}
</style>
