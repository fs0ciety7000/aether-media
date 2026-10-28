<script lang="ts">
	// POST /libraries { name, kind, sources }. Three steps in one dialog:
	// library (name, kind, list of sources) → source type (3 tiles) → source form (with connection test).
	import { tick } from 'svelte';
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import type { Library, LibraryKind, SourceKind } from '$lib/api/types';
	import { LIBRARY_KINDS } from '$lib/media';
	import { draftLabel, draftToInput, emptyDraft, isTested, sourceKindInfo, validateDraft, type DraftErrors, type SourceDraft } from '$lib/sources';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import IconButton from '../IconButton.svelte';
	import SourceForm from '../sources/SourceForm.svelte';
	import SourceKindTiles from '../sources/SourceKindTiles.svelte';

	interface Props {
		open: boolean;
		oncreated?: (lib: Library) => void;
	}

	let { open = $bindable(false), oncreated }: Props = $props();

	type Step = 'library' | 'kind' | 'source';

	let step = $state<Step>('library');
	let name = $state('');
	let kind = $state<LibraryKind>('movies');
	let sources = $state<SourceDraft[]>([]);
	let saving = $state(false);
	let errors = $state<{ name?: string; kind?: string; sources?: string; form?: string }>({});
	let nameTouched = false;

	/** Source being configured, and its index in `sources` (null = new). */
	let current = $state<SourceDraft | null>(null);
	let currentIndex = $state<number | null>(null);
	let currentErrors = $state<DraftErrors>({});
	let sourceHint = $state<string | null>(null);
	let sourceForm = $state<SourceForm>();

	$effect(() => {
		if (!open) return;
		step = 'library';
		name = LIBRARY_KINDS[0].label;
		kind = 'movies';
		sources = [];
		errors = {};
		current = null;
		currentIndex = null;
		nameTouched = false;
	});

	// Suggest a name from the kind until the user types one.
	$effect(() => {
		const label = LIBRARY_KINDS.find((k) => k.id === kind)?.label ?? '';
		if (!nameTouched) name = label;
	});

	/** Remote sources must pass a test; local folders are validated by the server. */
	const ready = (d: SourceDraft) => d.kind === 'local' || isTested(d);
	const currentReady = $derived(!!current && ready(current) && (current.kind !== 'agent' || !!current.agent.share));
	const allReady = $derived(sources.length > 0 && sources.every(ready));

	async function go(next: Step, focus?: string) {
		step = next;
		await tick();
		const target = focus ? document.querySelector<HTMLElement>(focus) : null;
		(target ?? document.getElementById('add-library-step'))?.focus();
	}

	function startAdd() {
		currentIndex = null;
		current = null;
		sourceHint = null;
		errors = { ...errors, sources: undefined };
		go('kind', '#add-library .tile');
	}

	function pickKind(k: SourceKind) {
		current = emptyDraft(k);
		currentErrors = {};
		sourceHint = null;
		go('source', '#add-library [data-source-form] input');
	}

	function editSource(i: number) {
		currentIndex = i;
		current = $state.snapshot(sources[i]) as SourceDraft;
		currentErrors = {};
		sourceHint = null;
		go('source', '#add-library [data-source-form] input');
	}

	function removeSource(i: number) {
		sources = sources.filter((_, j) => j !== i);
		tick().then(() => document.querySelector<HTMLElement>('#add-library .src-row button, #add-library-add')?.focus());
	}

	function back() {
		if (step === 'source' && currentIndex === null) return go('kind', `#add-library .tile[data-kind="${current?.kind}"]`);
		const i = currentIndex;
		current = null;
		currentIndex = null;
		go('library', i !== null ? `#add-library [data-src-index="${i}"]` : '#add-library-add');
	}

	async function confirmSource() {
		if (!current) return;
		if (!ready(current)) {
			// Enter in a field of an untested source runs the test instead.
			sourceForm?.test();
			sourceHint = 'Testez la connexion avant d’ajouter cette source.';
			return;
		}
		currentErrors = validateDraft(current);
		if (Object.values(currentErrors).some(Boolean)) {
			await tick();
			document.querySelector<HTMLElement>('#add-library [aria-invalid="true"]')?.focus();
			return;
		}
		const d = $state.snapshot(current) as SourceDraft;
		let index: number;
		if (currentIndex === null) {
			sources = [...sources, d];
			index = sources.length - 1;
		} else {
			sources[currentIndex] = d;
			index = currentIndex;
		}
		current = null;
		currentIndex = null;
		go('library', `#add-library [data-src-index="${index}"]`);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (step !== 'library') return confirmSource();
		errors = {};
		if (!name.trim()) errors.name = 'Donnez un nom à la bibliothèque.';
		if (!sources.length) errors.sources = 'Ajoutez au moins une source.';
		else if (!allReady) errors.sources = 'Chaque source distante doit passer le test de connexion.';
		if (Object.keys(errors).length) return focusError();

		saving = true;
		try {
			const lib = await api.createLibrary({ name: name.trim(), kind, sources: sources.map(draftToInput) });
			oncreated?.(lib);
			open = false;
		} catch (err) {
			const msg = errorMessage(err);
			const code = err instanceof ApiError ? err.code : '';
			if (code.includes('name')) errors.name = msg;
			else if (code.includes('kind')) errors.kind = msg;
			else if (code.includes('path') || code.includes('source') || (err instanceof ApiError && err.status === 409)) errors.sources = msg;
			else errors.form = msg;
			focusError();
		} finally {
			saving = false;
		}
	}

	function focusError() {
		requestAnimationFrame(() => document.querySelector<HTMLElement>('#add-library [aria-invalid="true"], #add-library-add')?.focus());
	}

	const description = $derived(
		step === 'library'
			? 'Une bibliothèque regroupe une ou plusieurs sources : dossiers du serveur, machines distantes, seedbox.'
			: step === 'kind'
				? 'Où se trouvent les fichiers de cette source ?'
				: 'Aether vérifie la connexion avant d’enregistrer la source.'
	);
</script>

<Dialog bind:open title="Ajouter une bibliothèque" {description} size="lg" initialFocus="#lib-name">
	<form id="add-library" class="form" onsubmit={submit} novalidate>
		{#if step === 'library'}
			<div class="f-field">
				<label class="f-label" for="lib-name">Nom</label>
				<input
					id="lib-name"
					class="f-input"
					type="text"
					bind:value={name}
					oninput={() => (nameTouched = true)}
					maxlength="100"
					aria-invalid={!!errors.name}
					aria-describedby={errors.name ? 'lib-name-err' : undefined}
				/>
				{#if errors.name}<p class="f-err" id="lib-name-err"><Icon name="alert" size={14} />{errors.name}</p>{/if}
			</div>

			<fieldset class="f-field" aria-describedby={errors.kind ? 'lib-kind-err' : undefined}>
				<legend>Type de contenu</legend>
				<div class="kinds">
					{#each LIBRARY_KINDS as k (k.id)}
						<label class="kind" class:checked={kind === k.id}>
							<input type="radio" name="kind" value={k.id} bind:group={kind} />
							<span class="k-icon" aria-hidden="true"><Icon name={k.icon} size={20} /></span>
							<span class="k-text">
								<strong>{k.label}</strong>
								<span>{k.hint}</span>
							</span>
						</label>
					{/each}
				</div>
				{#if errors.kind}<p class="f-err" id="lib-kind-err"><Icon name="alert" size={14} />{errors.kind}</p>{/if}
			</fieldset>

			<section class="f-field" aria-labelledby="lib-sources-title">
				<h3 class="f-label" id="lib-sources-title">Sources</h3>
				{#if sources.length}
					<ul class="src-list" role="list">
						{#each sources as d, i (i)}
							{@const info = sourceKindInfo(d.kind)}
							{@const lbl = draftLabel(d)}
							<li class="src-row">
								<span class="s-icon" aria-hidden="true"><Icon name={info.icon} size={18} /></span>
								<span class="s-text">
									<strong>{lbl.title}</strong>
									<span>{lbl.detail}</span>
								</span>
								{#if d.kind !== 'local'}
									<span class="s-ok micro"><Icon name="check" size={13} />Testée</span>
								{/if}
								<span class="s-actions">
									<IconButton icon="edit" variant="ghost" size="sm" data-src-index={i} aria-label="Modifier la source {lbl.title}" onclick={() => editSource(i)} />
									<IconButton icon="x" variant="ghost" size="sm" aria-label="Retirer la source {lbl.title}" onclick={() => removeSource(i)} />
								</span>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="empty" id="lib-sources-empty">Aucune source pour l’instant : un dossier du serveur, une machine avec l’agent Aether ou une seedbox SFTP.</p>
				{/if}
				{#if errors.sources}<p class="f-err" role="alert"><Icon name="alert" size={14} />{errors.sources}</p>{/if}
				<div>
					<Button
						id="add-library-add"
						variant={sources.length ? 'tertiary' : 'secondary'}
						icon="plus"
						aria-invalid={!!errors.sources}
						onclick={startAdd}>Ajouter une source</Button
					>
				</div>
			</section>

			{#if errors.form}<p class="f-err" role="alert"><Icon name="alert" size={14} />{errors.form}</p>{/if}
		{:else}
			<div class="step-head">
				<IconButton icon="arrow-left" variant="surface" size="sm" aria-label={step === 'kind' ? 'Retour aux sources' : 'Retour'} onclick={back} />
				<h3 id="add-library-step" tabindex="-1">
					{#if step === 'kind'}
						Ajouter une source
					{:else if current}
						{currentIndex === null ? '' : 'Modifier · '}{sourceKindInfo(current.kind).label}
					{/if}
				</h3>
			</div>

			{#if step === 'kind'}
				<SourceKindTiles onpick={pickKind} />
			{:else if current}
				<SourceForm bind:this={sourceForm} bind:draft={current} bind:errors={currentErrors} idp="new-src" />
				{#if sourceHint && !currentReady}<p class="f-err" role="alert"><Icon name="alert" size={14} />{sourceHint}</p>{/if}
			{/if}
		{/if}
	</form>

	{#snippet footer()}
		{#if step === 'library'}
			<Button variant="tertiary" onclick={() => (open = false)}>Annuler</Button>
			<Button
				variant="primary"
				type="submit"
				form="add-library"
				icon="plus"
				disabled={saving || !allReady}
				title={!sources.length ? 'Ajoutez au moins une source' : undefined}>{saving ? 'Création…' : 'Créer et analyser'}</Button
			>
		{:else if step === 'kind'}
			<Button variant="tertiary" onclick={back}>Retour</Button>
		{:else}
			<Button variant="tertiary" onclick={back}>Retour</Button>
			<Button
				variant="primary"
				type="submit"
				form="add-library"
				icon="check"
				disabled={!currentReady}
				title={currentReady ? undefined : current?.kind === 'agent' && isTested(current) ? 'Choisissez un partage' : 'Testez la connexion d’abord'}
				>{currentIndex === null ? 'Ajouter la source' : 'Mettre à jour'}</Button
			>
		{/if}
	{/snippet}
</Dialog>

<style>
	.form {
		display: grid;
		gap: var(--sp-5);
	}

	.f-field > legend {
		margin-bottom: var(--sp-2);
	}

	h3.f-label {
		font-size: var(--fs-small);
	}

	.kinds {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--sp-2);
	}

	.kind {
		position: relative;
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease);
	}

	.kind:hover {
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.kind.checked {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 60%, transparent);
	}

	.kind input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.kind:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	.k-icon,
	.s-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 38px;
		height: 38px;
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.checked .k-icon,
	.s-icon {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.k-text,
	.s-text {
		display: grid;
		gap: 2px;
		min-width: 0;
		font-size: var(--fs-micro);
		line-height: var(--lh-small);
		color: var(--text-muted);
	}

	.k-text strong,
	.s-text strong {
		font-size: var(--fs-body);
		color: var(--text);
	}

	.s-text > * {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.src-list {
		display: grid;
		gap: var(--sp-2);
	}

	.src-row {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-2) var(--sp-2) var(--sp-2) var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.s-text {
		flex: 1;
	}

	.s-ok {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		flex: none;
		color: var(--success);
	}

	.s-actions {
		display: flex;
		flex: none;
	}

	.empty {
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.step-head {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		margin-bottom: calc(-1 * var(--sp-1));
	}

	.step-head h3 {
		font-size: var(--fs-h3);
	}

	.step-head h3:focus {
		outline: none;
	}

	@media (max-width: 767px) {
		.kinds {
			grid-template-columns: minmax(0, 1fr);
		}

		.s-ok {
			display: none;
		}
	}
</style>
