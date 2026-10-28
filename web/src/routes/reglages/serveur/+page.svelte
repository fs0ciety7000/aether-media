<script lang="ts">
	// Réglages › Serveur (admins): editable settings (name, metadata, TMDB, transcoding, rescans)
	// with their source, plus the server status (version, paths, FFmpeg, hardware acceleration).
	import { onMount } from 'svelte';
	import { api, errorMessage } from '$lib/api/client';
	import type { HWAccelMode, ServerSettings, SettingsPatch, SystemStatus } from '$lib/api/types';
	import { encoderLabel, HW_MODES, hwLabel, intervalLabel, SCAN_INTERVALS, SETTING_ENV } from '$lib/admin';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Select from '$lib/components/Select.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import Tag from '$lib/components/Tag.svelte';
	import LocaleFields from '$lib/components/admin/LocaleFields.svelte';
	import SettingMeta from '$lib/components/admin/SettingMeta.svelte';
	import TmdbKeyField from '$lib/components/admin/TmdbKeyField.svelte';
	import { formatBytes, formatDate, formatRelative } from '$lib/format';
	import { system } from '$lib/api/stores.svelte';
	import { toasts } from '$lib/toast.svelte';

	let settings = $state<ServerSettings | null>(null);
	let status = $state<SystemStatus | null>(null);
	let error = $state<unknown>(null);
	let detecting = $state(false);

	// Drafts per section.
	let serverName = $state('');
	let language = $state('fr-FR');
	let country = $state('');
	let hwMode = $state<HWAccelMode>('none');
	let hwDevice = $state('');
	let maxTranscodes = $state('2');
	let scanEvery = $state('360');
	let saving = $state<'' | 'identity' | 'metadata' | 'transcode' | 'library'>('');
	let errors = $state<Record<string, string>>({});

	function fill(s: ServerSettings) {
		settings = s;
		serverName = s.serverName.value;
		language = s.metadataLanguage.value;
		country = s.metadataCountry.value ?? '';
		hwMode = s.hwAccel.value.mode;
		hwDevice = s.hwAccel.value.device ?? '';
		maxTranscodes = String(s.maxTranscodes.value);
		scanEvery = String(s.scanIntervalMinutes.value);
	}

	async function load() {
		try {
			fill(await api.settings());
			error = null;
		} catch (err) {
			error = err;
		}
		loadStatus(false);
	}

	async function loadStatus(detect: boolean) {
		detecting = detect;
		try {
			status = await api.systemStatus(detect);
		} catch {
			/* the settings stay usable */
		} finally {
			detecting = false;
		}
	}

	onMount(load);

	async function save(section: typeof saving, patch: SettingsPatch, e?: SubmitEvent) {
		e?.preventDefault();
		if (!Object.keys(patch).length) return;
		saving = section;
		errors = {};
		try {
			fill(await api.patchSettings(patch));
			const restart = section === 'transcode' && settings?.hwAccel.restartRequired;
			toasts.show('Réglages enregistrés', { detail: restart ? 'L’accélération matérielle sera prise en compte au prochain redémarrage d’Aether.' : 'Appliqués immédiatement.' });
			if (section === 'identity' && system.info) system.info.serverName = settings?.serverName.value;
			if (section === 'transcode') loadStatus(false);
		} catch (err) {
			errors = { [section]: errorMessage(err) };
		} finally {
			saving = '';
		}
	}

	const hwInfo = $derived(HW_MODES.find((m) => m.value === hwMode));
	const needsDevice = $derived(hwMode === 'vaapi' || hwMode === 'qsv');
	const scanOptions = $derived.by(() => {
		const opts = SCAN_INTERVALS.map((o) => ({ value: String(o.value), label: o.label }));
		if (settings && !SCAN_INTERVALS.some((o) => o.value === settings!.scanIntervalMinutes.value))
			opts.unshift({ value: String(settings.scanIntervalMinutes.value), label: intervalLabel(settings.scanIntervalMinutes.value) });
		return opts;
	});
	const transcodeOptions = Array.from({ length: 8 }, (_, i) => ({ value: String(i + 1), label: i === 0 ? '1 à la fois' : `${i + 1} simultanés` }));

	function metadataPatch(): SettingsPatch {
		const p: SettingsPatch = {};
		if (!settings) return p;
		if (!settings.metadataLanguage.readOnly && language !== settings.metadataLanguage.value) p.metadataLanguage = language;
		if (!settings.metadataCountry.readOnly && country !== (settings.metadataCountry.value ?? '')) p.metadataCountry = country;
		return p;
	}

	function transcodePatch(): SettingsPatch {
		const p: SettingsPatch = {};
		if (!settings) return p;
		if (!settings.hwAccel.readOnly && (hwMode !== settings.hwAccel.value.mode || (needsDevice && hwDevice !== (settings.hwAccel.value.device ?? ''))))
			p.hwAccel = { mode: hwMode, device: needsDevice ? hwDevice.trim() : undefined };
		if (!settings.maxTranscodes.readOnly && Number(maxTranscodes) !== settings.maxTranscodes.value) p.maxTranscodes = Number(maxTranscodes);
		return p;
	}

	const uptime = $derived(status ? formatRelative(status.startedAt) : undefined);
</script>

<svelte:head>
	<title>Serveur — Réglages — Aether</title>
</svelte:head>

<section class="page settings">
	<header class="head">
		<p class="micro eyebrow">Réglages</p>
		<h1>Serveur</h1>
		<p class="lead">Les réglages définis par le fichier de configuration ou les variables <code>AETHER_*</code> sont prioritaires : ils s’affichent ici en lecture seule.</p>
	</header>

	{#if error && !settings}
		<EmptyState icon="alert" title="Réglages indisponibles" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !settings}
		<div class="grid" aria-busy="true"><Skeleton height="200px" radius="lg" /><Skeleton height="320px" radius="lg" /></div>
	{:else}
		<div class="grid">
			<div class="col">
				<form class="panel section" aria-labelledby="s-identity" onsubmit={(e) => save('identity', settings!.serverName.readOnly ? {} : { serverName }, e)}>
					<h2 id="s-identity"><Icon name="server" size={18} />Identité</h2>
					<div class="f-field">
						<label class="f-label" for="srv-name">Nom du serveur</label>
						<input id="srv-name" class="f-input" bind:value={serverName} maxlength="64" readonly={settings.serverName.readOnly} aria-describedby="srv-name-meta" />
						<SettingMeta id="srv-name-meta" {...settings.serverName} env={SETTING_ENV.serverName} />
					</div>
					{#if errors.identity}<p class="f-err" role="alert"><Icon name="alert" size={14} />{errors.identity}</p>{/if}
					{#if !settings.serverName.readOnly}
						<div class="foot"><Button variant="primary" type="submit" disabled={saving !== '' || serverName.trim() === settings.serverName.value}>{saving === 'identity' ? 'Enregistrement…' : 'Enregistrer'}</Button></div>
					{/if}
				</form>

				<form class="panel section" aria-labelledby="s-meta" onsubmit={(e) => save('metadata', metadataPatch(), e)}>
					<h2 id="s-meta"><Icon name="globe" size={18} />Métadonnées</h2>
					<LocaleFields bind:language bind:country languageField={settings.metadataLanguage} countryField={settings.metadataCountry} />
					{#if errors.metadata}<p class="f-err" role="alert"><Icon name="alert" size={14} />{errors.metadata}</p>{/if}
					<p class="f-hint">Les éléments déjà identifiés gardent leurs textes jusqu’au prochain rafraîchissement (Réglages › Tâches).</p>
					{#if !(settings.metadataLanguage.readOnly && settings.metadataCountry.readOnly)}
						<div class="foot"><Button variant="primary" type="submit" disabled={saving !== '' || !Object.keys(metadataPatch()).length}>{saving === 'metadata' ? 'Enregistrement…' : 'Enregistrer'}</Button></div>
					{/if}
				</form>

				<section class="panel section" aria-labelledby="s-tmdb">
					<h2 id="s-tmdb"><Icon name="key" size={18} />Clé TMDB</h2>
					<TmdbKeyField setting={settings.tmdbApiKey} onsaved={fill} idp="srv-tmdb" />
				</section>
			</div>

			<div class="col">
				<form class="panel section" aria-labelledby="s-trans" onsubmit={(e) => save('transcode', transcodePatch(), e)}>
					<h2 id="s-trans"><Icon name="cpu" size={18} />Transcodage</h2>
					<div class="f-field">
						<Select label="Accélération matérielle" showLabel options={HW_MODES.map((m) => ({ value: m.value, label: m.label }))} bind:value={hwMode} disabled={settings.hwAccel.readOnly} />
						{#if hwInfo}<p class="f-hint">{hwInfo.hint}</p>{/if}
						<SettingMeta {...settings.hwAccel} env={SETTING_ENV.hwAccel} />
					</div>
					{#if needsDevice}
						<div class="f-field">
							<label class="f-label" for="hw-dev">Périphérique de rendu</label>
							<input id="hw-dev" class="f-input f-mono" bind:value={hwDevice} placeholder="/dev/dri/renderD128" readonly={settings.hwAccel.readOnly} />
						</div>
					{/if}
					<div class="f-field">
						<Select label="Transcodages simultanés" showLabel options={transcodeOptions} bind:value={maxTranscodes} disabled={settings.maxTranscodes.readOnly} />
						<p class="f-hint">Au-delà, une nouvelle lecture qui doit être convertie est refusée avec un message clair. Le remux compte aussi.</p>
						<SettingMeta {...settings.maxTranscodes} env={SETTING_ENV.maxTranscodes} />
					</div>
					{#if errors.transcode}<p class="f-err" role="alert"><Icon name="alert" size={14} />{errors.transcode}</p>{/if}
					{#if !(settings.hwAccel.readOnly && settings.maxTranscodes.readOnly)}
						<div class="foot"><Button variant="primary" type="submit" disabled={saving !== '' || !Object.keys(transcodePatch()).length}>{saving === 'transcode' ? 'Enregistrement…' : 'Enregistrer'}</Button></div>
					{/if}
				</form>

				<form
					class="panel section"
					aria-labelledby="s-lib"
					onsubmit={(e) => save('library', settings!.scanIntervalMinutes.readOnly ? {} : { scanIntervalMinutes: Number(scanEvery) }, e)}
				>
					<h2 id="s-lib"><Icon name="refresh" size={18} />Analyse des bibliothèques</h2>
					<div class="f-field">
						<Select label="Analyse périodique" showLabel options={scanOptions} bind:value={scanEvery} disabled={settings.scanIntervalMinutes.readOnly} />
						<p class="f-hint">En plus de la surveillance des dossiers locaux : utile pour les partages réseau et les seedbox, qui ne signalent pas leurs changements.</p>
						<SettingMeta {...settings.scanIntervalMinutes} env={SETTING_ENV.scanIntervalMinutes} />
					</div>
					{#if errors.library}<p class="f-err" role="alert"><Icon name="alert" size={14} />{errors.library}</p>{/if}
					{#if !settings.scanIntervalMinutes.readOnly}
						<div class="foot">
							<Button variant="primary" type="submit" disabled={saving !== '' || Number(scanEvery) === settings.scanIntervalMinutes.value}>{saving === 'library' ? 'Enregistrement…' : 'Enregistrer'}</Button>
						</div>
					{/if}
				</form>
			</div>
		</div>

		<section class="panel section status" aria-labelledby="s-status">
			<div class="status-head">
				<h2 id="s-status"><Icon name="info" size={18} />État du serveur</h2>
				<Button variant="tertiary" icon="refresh" disabled={detecting} onclick={() => loadStatus(true)}>{detecting ? 'Détection…' : 'Détecter le matériel'}</Button>
			</div>
			{#if status}
				<div class="tiles">
					<StatTile label="Version" value={status.version} hint="{status.commit} · {status.os}/{status.arch} · {status.goVersion}" icon="server" />
					<StatTile label="Démarré" value={uptime ?? '—'} hint={formatDate(status.startedAt)} icon="clock" />
					<StatTile label="Transcodages" value="{status.transcodes.active} / {status.transcodes.max}" hint="en cours / maximum" icon="cpu" tone={status.transcodes.active ? 'accent' : 'default'} />
					<StatTile
						label="Accélération"
						value={hwLabel(status.hwAccel.active)}
						hint={status.hwAccel.detected.length ? `Détecté : ${status.hwAccel.detected.map(encoderLabel).join(', ')}` : 'Aucun encodeur matériel détecté'}
						icon="gauge"
						tone={status.hwAccel.active && status.hwAccel.active !== 'none' ? 'accent' : 'default'}
					/>
				</div>
				<dl class="facts">
					<div>
						<dt>FFmpeg</dt>
						<dd>
							{#if status.ffmpeg.ok}<Tag variant="accent" icon="check">{status.ffmpeg.version}</Tag>{:else}<Tag icon="alert">{status.ffmpeg.error ?? 'introuvable'}</Tag>{/if}
							<code>{status.ffmpeg.path}</code>
						</dd>
					</div>
					<div>
						<dt>FFprobe</dt>
						<dd>
							{#if status.ffprobe.ok}<Tag variant="accent" icon="check">{status.ffprobe.version}</Tag>{:else}<Tag icon="alert">{status.ffprobe.error ?? 'introuvable'}</Tag>{/if}
							<code>{status.ffprobe.path}</code>
						</dd>
					</div>
					<div>
						<dt>Données</dt>
						<dd><code>{status.paths.data}</code></dd>
					</div>
					<div>
						<dt>Base de données</dt>
						<dd><code>{status.paths.database}</code>{#if status.paths.databaseSize}<span class="muted">{formatBytes(status.paths.databaseSize)}</span>{/if}</dd>
					</div>
					<div>
						<dt>Cache</dt>
						<dd><code>{status.paths.cache}</code></dd>
					</div>
					<div>
						<dt>Transcodage</dt>
						<dd><code>{status.paths.transcode}</code></dd>
					</div>
				</dl>
			{:else}
				<Skeleton height="120px" radius="lg" />
			{/if}
		</section>
	{/if}
</section>

<style>
	.settings {
		display: grid;
		gap: var(--gap);
		align-content: start;
		max-width: 1180px;
	}

	.head {
		display: grid;
		gap: var(--sp-1);
		margin-bottom: var(--sp-2);
	}

	.eyebrow {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.lead {
		max-width: 76ch;
		color: var(--text-muted);
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.8125em;
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--gap);
		align-items: start;
	}

	.col {
		display: grid;
		gap: var(--gap);
		min-width: 0;
	}

	.section {
		display: grid;
		gap: var(--sp-4);
		padding: var(--sp-5);
		min-width: 0;
	}

	h2 {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
	}

	h2 :global(svg) {
		color: var(--accent);
	}

	.foot {
		display: flex;
		justify-content: flex-end;
	}

	.status-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
	}

	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
		gap: var(--sp-3);
	}

	.facts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
		gap: var(--sp-3) var(--sp-5);
	}

	.facts div {
		display: grid;
		gap: 4px;
		min-width: 0;
	}

	dt {
		color: var(--text-muted);
		font-size: var(--fs-micro);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	dd {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-2);
		min-width: 0;
	}

	.muted {
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	@media (max-width: 1023px) {
		.grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	@media (max-width: 767px) {
		.section {
			padding: var(--sp-4);
		}

		.facts {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
