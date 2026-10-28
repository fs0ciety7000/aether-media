<script lang="ts" module>
	/** 0 (empty) to 4: length first, then variety. Hint only — the server requires ≥ 8 characters. */
	export function passwordScore(pw: string): number {
		if (!pw) return 0;
		if (pw.length < 8) return 1;
		let variety = 0;
		if (/[a-z]/.test(pw)) variety++;
		if (/[A-Z]/.test(pw)) variety++;
		if (/\d/.test(pw)) variety++;
		if (/[^A-Za-z0-9]/.test(pw)) variety++;
		let score = 2;
		if (pw.length >= 12 || variety >= 3) score = 3;
		if ((pw.length >= 14 && variety >= 2) || (pw.length >= 12 && variety >= 3)) score = 4;
		if (/^(.)\1+$/.test(pw) || /^(?:0123|1234|azer|qwer|motdepasse|password)/i.test(pw)) score = Math.min(score, 2);
		return score;
	}

	const LABELS = ['', 'Trop court', 'Correct', 'Solide', 'Excellent'];
	const HINTS = [
		'8 caractères au minimum.',
		'Encore un effort : 8 caractères au minimum.',
		'Allongez-le, ou mêlez chiffres et symboles.',
		'Bon mot de passe.',
		'Très bon mot de passe.'
	];
</script>

<script lang="ts">
	interface Props {
		password: string;
		id?: string;
	}

	let { password, id }: Props = $props();
	const score = $derived(passwordScore(password));
</script>

<div class="strength" class:weak={score === 1} {id} aria-live="polite">
	<div class="bars" aria-hidden="true">
		{#each [1, 2, 3, 4] as i (i)}<span class:on={score >= i}></span>{/each}
	</div>
	<p class="text">
		{#if score}<strong>{LABELS[score]}</strong>{' · '}{/if}{HINTS[score]}
	</p>
</div>

<style>
	.strength {
		display: grid;
		gap: 6px;
	}

	.bars {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 4px;
	}

	.bars span {
		height: 4px;
		border-radius: var(--r-full);
		background: var(--line-strong);
		transition: background-color var(--dur-base) var(--ease);
	}

	.bars span.on {
		background: var(--accent);
	}

	.weak .bars span.on {
		background: var(--danger);
	}

	.text {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	.text strong {
		color: var(--text);
	}
</style>
