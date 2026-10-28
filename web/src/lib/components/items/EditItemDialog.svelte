<script lang="ts">
	// PATCH /items/{id}: edited fields become locked against metadata refreshes; a lock can be released.
	import { api, errorMessage } from '$lib/api/client';
	import type { ItemDetail, ItemPatch } from '$lib/api/types';
	import { toasts } from '$lib/toast.svelte';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';

	interface Props {
		open: boolean;
		item: ItemDetail;
		onsaved?: (item: ItemDetail) => void;
	}

	let { open = $bindable(false), item, onsaved }: Props = $props();

	type Field = 'title' | 'originalTitle' | 'year' | 'tagline' | 'overview';
	const FIELDS: { id: Field; label: string; multiline?: boolean; numeric?: boolean }[] = [
		{ id: 'title', label: 'Titre' },
		{ id: 'originalTitle', label: 'Titre original' },
		{ id: 'year', label: 'Année', numeric: true },
		{ id: 'tagline', label: 'Accroche' },
		{ id: 'overview', label: 'Synopsis', multiline: true }
	];

	let values = $state<Record<Field, string>>({ title: '', originalTitle: '', year: '', tagline: '', overview: '' });
	let locks = $state<Record<string, boolean>>({});
	let saving = $state(false);
	let error = $state<string | null>(null);
	let fieldError = $state<Field | null>(null);

	const initial = (f: Field) => (f === 'year' ? (item.year ? String(item.year) : '') : ((item[f] as string | undefined) ?? ''));

	// Reset the form every time it opens.
	$effect(() => {
		if (!open) return;
		values = { title: initial('title'), originalTitle: initial('originalTitle'), year: initial('year'), tagline: initial('tagline'), overview: initial('overview') };
		locks = Object.fromEntries(FIELDS.map((f) => [f.id, item.lockedFields.includes(f.id)]));
		error = null;
		fieldError = null;
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		const body: ItemPatch = {};
		for (const f of FIELDS) {
			const v = values[f.id].trim();
			if (v === initial(f.id).trim()) continue;
			if (f.id === 'year') {
				if (!v) continue; // the API cannot clear a year
				const n = Number(v);
				if (!Number.isInteger(n) || n < 1800 || n > 2200) {
					error = 'Année invalide (entre 1800 et 2200).';
					fieldError = 'year';
					return;
				}
				body.year = n;
			} else body[f.id] = v;
		}
		const unlock = FIELDS.filter((f) => item.lockedFields.includes(f.id) && !locks[f.id]).map((f) => f.id);
		if (unlock.length) body.unlock = unlock;
		if (!Object.keys(body).length) {
			open = false;
			return;
		}
		saving = true;
		error = null;
		fieldError = null;
		try {
			const updated = await api.patchItem(item.id, body);
			toasts.show('Modifications enregistrées', { detail: updated.title });
			onsaved?.(updated);
			open = false;
		} catch (err) {
			error = errorMessage(err);
			const code = (err as { code?: string }).code ?? '';
			fieldError = code.includes('title') ? 'title' : code.includes('year') ? 'year' : null;
		} finally {
			saving = false;
		}
	}
</script>

<Dialog bind:open title="Modifier « {item.title} »" description="Les champs modifiés sont verrouillés : les actualisations de métadonnées ne les remplaceront plus." size="lg">
	<form id="edit-item" class="form" onsubmit={save} novalidate>
		{#each FIELDS as f (f.id)}
			{@const locked = locks[f.id]}
			<div class="field" class:wide={f.multiline} class:short={f.numeric}>
				<div class="label-row">
					<label for="edit-{f.id}">{f.label}</label>
					{#if item.lockedFields.includes(f.id)}
						<button
							type="button"
							class="lock"
							class:on={locked}
							aria-pressed={locked}
							aria-label={locked ? `${f.label} verrouillé — déverrouiller` : `${f.label} déverrouillé — reverrouiller`}
							title={locked ? 'Verrouillé : cliquer pour rendre ce champ aux actualisations' : 'Sera mis à jour automatiquement'}
							onclick={() => (locks[f.id] = !locked)}
						>
							<Icon name={locked ? 'lock' : 'unlock'} size={14} />
							<span>{locked ? 'Verrouillé' : 'Automatique'}</span>
						</button>
					{/if}
				</div>
				{#if f.multiline}
					<textarea id="edit-{f.id}" rows="6" bind:value={values[f.id]} aria-invalid={fieldError === f.id}></textarea>
				{:else}
					<input
						id="edit-{f.id}"
						type="text"
						inputmode={f.numeric ? 'numeric' : undefined}
						bind:value={values[f.id]}
						aria-invalid={fieldError === f.id}
						aria-describedby={fieldError === f.id ? 'edit-error' : undefined}
					/>
				{/if}
			</div>
		{/each}
		{#if error}<p class="error" id="edit-error" role="alert"><Icon name="alert" size={16} />{error}</p>{/if}
	</form>

	{#snippet footer()}
		<Button variant="tertiary" onclick={() => (open = false)}>Annuler</Button>
		<Button variant="primary" type="submit" form="edit-item" disabled={saving}>{saving ? 'Enregistrement…' : 'Enregistrer'}</Button>
	{/snippet}
</Dialog>

<style>
	.form {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 120px;
		gap: var(--sp-4);
	}

	.field {
		display: grid;
		gap: var(--sp-2);
		grid-column: span 1;
		align-content: start;
	}

	.field:nth-child(4),
	.wide {
		grid-column: 1 / -1;
	}

	.label-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-2);
		min-height: 24px;
	}

	label {
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	input,
	textarea {
		width: 100%;
		padding: 10px 14px;
		border: 0;
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text);
		font-size: var(--fs-body);
		line-height: var(--lh-body);
		transition: box-shadow var(--dur-fast) var(--ease);
	}

	input {
		height: 40px;
	}

	textarea {
		resize: vertical;
		min-height: 120px;
	}

	input:focus-visible,
	textarea:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 70%, transparent);
	}

	[aria-invalid='true'] {
		box-shadow: inset 0 0 0 1.5px var(--danger);
	}

	.lock {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		height: 24px;
		padding: 0 8px;
		border-radius: var(--r-full);
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.lock.on {
		color: var(--accent-ink);
		background: var(--accent-soft);
		box-shadow: none;
	}

	.error {
		grid-column: 1 / -1;
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--danger);
		font-size: var(--fs-small);
	}

	@media (max-width: 767px) {
		.form {
			grid-template-columns: minmax(0, 1fr);
		}

		.field {
			grid-column: 1 / -1;
		}
	}
</style>
