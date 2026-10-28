<script lang="ts">
	// Metadata language + country (age ratings, release dates). The country follows the language
	// ("" = deduced) until the user picks one. Values set by the configuration are shown read-only.
	import type { SettingField } from '$lib/api/types';
	import { COUNTRIES, countryLabel, METADATA_LANGUAGES, regionOf, SETTING_ENV } from '$lib/admin';
	import Select from '../Select.svelte';
	import SettingMeta from './SettingMeta.svelte';

	interface Props {
		language: string;
		country: string;
		languageField?: SettingField<string>;
		countryField?: SettingField<string>;
	}

	let { language = $bindable('fr-FR'), country = $bindable(''), languageField, countryField }: Props = $props();

	const withCurrent = (list: { value: string; label: string }[], v: string) =>
		v && !list.some((o) => o.value === v) ? [{ value: v, label: v }, ...list] : list;

	const languages = $derived(withCurrent(METADATA_LANGUAGES, language));
	const deduced = $derived(regionOf(language));
	const countries = $derived([
		{ value: '', label: deduced ? `Selon la langue (${countryLabel(deduced)})` : 'Selon la langue' },
		...withCurrent(COUNTRIES, country)
	]);
</script>

<div class="locale">
	<div class="f-field">
		<Select label="Langue des métadonnées" showLabel options={languages} bind:value={language} disabled={languageField?.readOnly} />
		<p class="f-hint">Titres, résumés et affiches dans cette langue quand TMDB les propose, sinon en anglais.</p>
		{#if languageField}<SettingMeta source={languageField.source} readOnly={languageField.readOnly} applies={languageField.applies} env={SETTING_ENV.metadataLanguage} />{/if}
	</div>
	<div class="f-field">
		<Select label="Pays" showLabel options={countries} bind:value={country} disabled={countryField?.readOnly} />
		<p class="f-hint">Pour les classifications d’âge (contrôle parental) et les dates de sortie en salle.</p>
		{#if countryField}<SettingMeta source={countryField.source} readOnly={countryField.readOnly} applies={countryField.applies} env={SETTING_ENV.metadataCountry} />{/if}
	</div>
</div>

<style>
	.locale {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: var(--sp-4);
	}
</style>
