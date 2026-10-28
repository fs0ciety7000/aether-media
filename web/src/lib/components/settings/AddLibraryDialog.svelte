<script lang="ts">
	// POST /libraries: name, kind, one or more absolute folders. Server errors land on their field.
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import type { Library, LibraryKind } from '$lib/api/types';
	import { LIBRARY_KINDS } from '$lib/media';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import IconButton from '../IconButton.svelte';

	interface Props {
		open: boolean;
		oncreated?: (lib: Library) => void;
	}

	let { open = $bindable(false), oncreated }: Props = $props();

	let name = $state('');
	let kind = $state<LibraryKind>('movies');
	let paths = $state<string[]>(['']);
	let saving = $state(false);
	let errors = $state<{ name?: string; kind?: string; paths?: string; form?: string }>({});
	let nameTouched = false;

	$effect(() => {
		if (!open) return;
		name = LIBRARY_KINDS[0].label;
		kind = 'movies';
		paths = [''];
		errors = {};
		nameTouched = false;
	});

	// Suggest a name from the kind until the user types one.
	$effect(() => {
		const label = LIBRARY_KINDS.find((k) => k.id === kind)?.label ?? '';
		if (!nameTouched) name = label;
	});

	function addPath() {
		paths = [...paths, ''];
		requestAnimationFrame(() => document.getElementById(`lib-path-${paths.length - 1}`)?.focus());
	}

	function removePath(i: number) {
		paths = paths.filter((_, j) => j !== i);
		requestAnimationFrame(() => document.getElementById(`lib-path-${Math.max(0, i - 1)}`)?.focus());
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		const clean = paths.map((p) => p.trim()).filter(Boolean);
		if (!name.trim()) errors.name = 'Donnez un nom à la bibliothèque.';
		if (!clean.length) errors.paths = 'Indiquez au moins un dossier.';
		else if (clean.some((p) => !p.startsWith('/') && !/^[A-Za-z]:[\\/]/.test(p))) errors.paths = 'Les chemins doivent être absolus (ex. /srv/media/films).';
		if (Object.keys(errors).length) return focusError();

		saving = true;
		try {
			const lib = await api.createLibrary({ name: name.trim(), kind, paths: clean });
			oncreated?.(lib);
			open = false;
		} catch (err) {
			const msg = errorMessage(err);
			const code = err instanceof ApiError ? err.code : '';
			if (code.includes('name')) errors.name = msg;
			else if (code.includes('kind')) errors.kind = msg;
			else if (code.includes('path')) errors.paths = msg;
			else errors.form = msg;
			focusError();
		} finally {
			saving = false;
		}
	}

	function focusError() {
		requestAnimationFrame(() => document.querySelector<HTMLElement>('#add-library [aria-invalid="true"]')?.focus());
	}
</script>

<Dialog bind:open title="Ajouter une bibliothèque" description="Aether analysera les dossiers dès la création, puis les surveillera." size="lg" initialFocus="#lib-name">
	<form id="add-library" class="form" onsubmit={submit} novalidate>
		<div class="field">
			<label for="lib-name">Nom</label>
			<input
				id="lib-name"
				type="text"
				bind:value={name}
				oninput={() => (nameTouched = true)}
				maxlength="100"
				aria-invalid={!!errors.name}
				aria-describedby={errors.name ? 'lib-name-err' : undefined}
			/>
			{#if errors.name}<p class="err" id="lib-name-err"><Icon name="alert" size={14} />{errors.name}</p>{/if}
		</div>

		<fieldset class="field" aria-describedby={errors.kind ? 'lib-kind-err' : undefined}>
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
			{#if errors.kind}<p class="err" id="lib-kind-err"><Icon name="alert" size={14} />{errors.kind}</p>{/if}
		</fieldset>

		<fieldset class="field" aria-describedby="lib-paths-hint{errors.paths ? ' lib-paths-err' : ''}">
			<legend>Dossiers</legend>
			<p class="hint" id="lib-paths-hint">Chemins absolus, tels que le serveur les voit (dans Docker : le chemin dans le conteneur).</p>
			{#each paths as _, i (i)}
				<div class="path-row">
					<label class="visually-hidden" for="lib-path-{i}">Dossier {i + 1}</label>
					<input
						id="lib-path-{i}"
						type="text"
						class="mono"
						placeholder="/srv/media/films"
						bind:value={paths[i]}
						autocomplete="off"
						spellcheck="false"
						aria-invalid={!!errors.paths}
					/>
					{#if paths.length > 1}
						<IconButton icon="x" variant="ghost" aria-label="Retirer le dossier {i + 1}" onclick={() => removePath(i)} />
					{/if}
				</div>
			{/each}
			{#if errors.paths}<p class="err" id="lib-paths-err" role="alert"><Icon name="alert" size={14} />{errors.paths}</p>{/if}
			<div><Button variant="tertiary" icon="plus" onclick={addPath}>Ajouter un dossier</Button></div>
		</fieldset>

		{#if errors.form}<p class="err" role="alert"><Icon name="alert" size={14} />{errors.form}</p>{/if}
	</form>

	{#snippet footer()}
		<Button variant="tertiary" onclick={() => (open = false)}>Annuler</Button>
		<Button variant="primary" type="submit" form="add-library" icon="plus" disabled={saving}>{saving ? 'Création…' : 'Créer et analyser'}</Button>
	{/snippet}
</Dialog>

<style>
	.form {
		display: grid;
		gap: var(--sp-5);
	}

	.field {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
		margin: 0;
		padding: 0;
		border: 0;
	}

	label,
	legend {
		padding: 0;
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	legend {
		margin-bottom: var(--sp-2);
	}

	.hint {
		margin-top: calc(-1 * var(--sp-1));
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	input[type='text'] {
		width: 100%;
		height: 42px;
		padding: 0 14px;
		border: 0;
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text);
		font-size: var(--fs-body);
	}

	input[type='text']:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 70%, transparent);
	}

	input[aria-invalid='true'] {
		box-shadow: inset 0 0 0 1.5px var(--danger);
	}

	input::placeholder {
		color: var(--text-faint);
	}

	.mono {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.8125rem !important;
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

	.k-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 38px;
		height: 38px;
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.checked .k-icon {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.k-text {
		display: grid;
		gap: 2px;
		font-size: var(--fs-micro);
		line-height: var(--lh-small);
		color: var(--text-muted);
	}

	.k-text strong {
		font-size: var(--fs-body);
		color: var(--text);
	}

	.path-row {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}

	.err {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: var(--fs-small);
		color: var(--danger);
	}

	@media (max-width: 767px) {
		.kinds {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
