<script lang="ts">
	// S12 Experience level and pay.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { loadPrefs, savePrefs } from '$lib/prefs.svelte';

	const BANDS = ['Intern', 'Fresher', 'Junior', 'Mid', 'Senior', 'Lead+'];
	let chosen = $state<string[]>([]);
	let lpa = $state('');
	onMount(async () => {
		const p = await loadPrefs();
		chosen = [...(p.seniority ?? [])];
		lpa = p.min_salary ? String(p.min_salary / 100000) : '';
	});
	const toggle = (b: string) => (chosen = chosen.includes(b) ? chosen.filter((x) => x !== b) : [...chosen, b]);
	async function next() {
		const n = parseFloat(lpa);
		await savePrefs({ seniority: chosen as never, min_salary: Number.isFinite(n) && n > 0 ? Math.round(n * 100000) : null });
		goto('/onboarding/notifications');
	}
</script>

<h1 class="t-title">Your level</h1>
<p class="t-body t-2">Pick the levels you're open to.</p>
<div class="chips" role="group" aria-label="Seniority">
	{#each BANDS as b (b)}<button class="chip {chosen.includes(b) ? 'on' : ''}" aria-pressed={chosen.includes(b)} onclick={() => toggle(b)}>{b}</button>{/each}
</div>
<div class="field">
	<label for="lpa">Minimum salary · optional (₹ LPA per year)</label>
	<input id="lpa" class="input" inputmode="decimal" bind:value={lpa} style="max-width: 160px" />
</div>
<p class="t-caption t-3">Only filters jobs that state a salary. Jobs without one still show.</p>
<button class="btn btn-primary btn-block" onclick={next}>Continue</button>
