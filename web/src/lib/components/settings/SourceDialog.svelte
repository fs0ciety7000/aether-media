<script lang="ts">
	// Add a source to an existing library (POST /libraries/{id}/sources: type tiles, then the form)
	// or edit one (PATCH /libraries/{id}/sources/{sid}: secrets left empty stay unchanged).
	import { tick, untrack } from 'svelte';
	import { api, errorMessage } from '$lib/api/client';
	import type { Library, Source, SourceKind } from '$lib/api/types';
	import {
		connectionChanged,
		draftFromSource,
		draftToInput,
		draftToPatch,
		emptyDraft,
		isTested,
		sourceKindInfo,
		validateDraft,
		type DraftErrors,
		type SourceDraft
	} from '$lib/sources';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import IconButton from '../IconButton.svelte';
	import SourceForm from '../sources/SourceForm.svelte';
	import SourceKindTiles from '../sources/SourceKindTiles.svelte';

	interface Props {
		open: boolean;
		library: Library | null;
		/** Set to edit; omitted to add a new source. */
		source?: Source | null;
		onsaved?: (src: Source, created: boolean) => void;
	}

	let { open = $bindable(false), library, source = null, onsaved }: Props = $props();

	let draft = $state<SourceDraft | null>(null);
	let errors = $state<DraftErrors>({});
	let formError = $state<string | null>(null);
	let saving = $state(false);
	let hint = $state<string | null>(null);
	let form = $state<SourceForm>();

	const editing = $derived(!!source);

	$effect(() => {
		if (!open) return;
		const src = source;
		draft = untrack(() => (src ? draftFromSource($state.snapshot(src) as Source) : null));
		errors = {};
		formError = null;
		hint = null;
	});

	/** Remote: a new source must pass a test; an edit only when the connection changed. */
	const mustTest = $derived(!!draft && draft.kind !== 'local' && (!source || connectionChanged(draft, source)));
	const canSave = $derived(!!draft && !saving && (!mustTest || isTested(draft)) && (draft.kind !== 'agent' || !!draft.agent.share));
	const dirty = $derived(!!draft && !!source && Object.keys(draftToPatch(draft, source)).length > 0);

	async function pick(kind: SourceKind) {
		draft = emptyDraft(kind);
		errors = {};
		await tick();
		document.querySelector<HTMLElement>('#source-dialog [data-source-form] input')?.focus();
	}

	async function back() {
		const k = draft?.kind;
		draft = null;
		await tick();
		document.querySelector<HTMLElement>(`#source-dialog .tile[data-kind="${k}"]`)?.focus();
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!draft || !library) return;
		formError = null;
		if (mustTest && !isTested(draft)) {
			form?.test();
			hint = 'Testez la connexion avant d’enregistrer.';
			return;
		}
		errors = validateDraft(draft, { editing });
		if (Object.values(errors).some(Boolean)) {
			await tick();
			document.querySelector<HTMLElement>('#source-dialog [aria-invalid="true"]')?.focus();
			return;
		}
		saving = true;
		try {
			if (source) {
				const patch = draftToPatch(draft, source);
				const saved = Object.keys(patch).length ? await api.patchSource(library.id, source.id, patch) : source;
				onsaved?.(saved, false);
			} else {
				onsaved?.(await api.addSource(library.id, draftToInput(draft)), true);
			}
			open = false;
		} catch (err) {
			formError = errorMessage(err);
		} finally {
			saving = false;
		}
	}

	const title = $derived(source ? `Modifier « ${source.name} »` : 'Ajouter une source');
	const description = $derived(
		source
			? 'Les secrets ne sont jamais réaffichés : laissez un champ secret vide pour le conserver.'
			: library
				? `Nouvelle source pour « ${library.name} ». Aether l’analyse dès l’ajout.`
				: undefined
	);
</script>

<Dialog bind:open {title} {description} size="lg">
	<form id="source-dialog" class="form" onsubmit={submit} novalidate>
		{#if !draft}
			<SourceKindTiles onpick={pick} />
		{:else}
			{#if !editing}
				<div class="step-head">
					<IconButton icon="arrow-left" variant="surface" size="sm" aria-label="Changer de type de source" onclick={back} />
					<h3>{sourceKindInfo(draft.kind).label}</h3>
				</div>
			{:else}
				<p class="kind-line"><Icon name={sourceKindInfo(draft.kind).icon} size={16} />{sourceKindInfo(draft.kind).label}</p>
			{/if}
			<SourceForm bind:this={form} bind:draft bind:errors original={source ?? undefined} idp="dlg-src" />
			{#if hint && mustTest && !isTested(draft)}<p class="f-err" role="alert"><Icon name="alert" size={14} />{hint}</p>{/if}
		{/if}
		{#if formError}<p class="f-err" role="alert"><Icon name="alert" size={14} />{formError}</p>{/if}
	</form>

	{#snippet footer()}
		<Button variant="tertiary" onclick={() => (open = false)}>Annuler</Button>
		{#if draft}
			<Button
				variant="primary"
				type="submit"
				form="source-dialog"
				icon={editing ? 'check' : 'plus'}
				disabled={!canSave || (editing && !dirty)}
				title={canSave ? undefined : 'Testez la connexion d’abord'}
			>
				{saving ? 'Enregistrement…' : editing ? 'Enregistrer' : 'Ajouter et analyser'}
			</Button>
		{/if}
	{/snippet}
</Dialog>

<style>
	.form {
		display: grid;
		gap: var(--sp-4);
	}

	.step-head {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
	}

	.step-head h3 {
		font-size: var(--fs-h3);
	}

	.kind-line {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--accent-ink);
	}
</style>
