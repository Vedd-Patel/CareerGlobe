<script lang="ts">
	// S09 Review parsed profile: chips, titles, years stepper, city; "Looks good" confirms.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, type Schemas } from '$lib/api/client';
	import { loadMe } from '$lib/account.svelte';
	import { draft } from '$lib/draft.svelte';
	import ChipEditor from '$lib/components/ChipEditor.svelte';
	import Banner from '$lib/components/Banner.svelte';

	let skills = $state<string[]>([]);
	let titles = $state<string[]>([]);
	let years = $state(0);
	let city = $state('');
	let busy = $state(false);
	let error = $state('');

	onMount(async () => {
		const p = draft.value ?? (await api.get<Schemas['Profile']>('/v1/profile'));
		skills = [...(p.skills ?? [])];
		titles = [...(p.titles ?? [])];
		years = p.years_exp ?? 0;
		city = p.current_city ?? '';
	});

	async function confirm() {
		busy = true;
		error = '';
		try {
			await api.put('/v1/profile', { skills, titles, years_exp: years, current_city: city || null, confirm: true });
			await loadMe();
			goto(page.url.searchParams.get('from') === 'profile' ? '/profile' : '/onboarding/roles');
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<h1 class="t-title">Does this look right?</h1>
<p class="t-body t-2">Parsers make mistakes. Fix anything before we match.</p>
<ChipEditor bind:items={skills} label="Skills" placeholder="Add a skill" />
<ChipEditor bind:items={titles} label="Job titles" placeholder="Add a title" max={10} />
<div class="field">
	<span class="t-caption t-2">YEARS OF EXPERIENCE</span>
	<div class="stepper">
		<button type="button" aria-label="Fewer years" onclick={() => (years = Math.max(0, Math.ceil(years) - 1))}>−</button>
		<input
			class="years"
			type="number"
			inputmode="decimal"
			min="0"
			max="60"
			step="0.5"
			aria-label="Years of experience"
			value={years}
			oninput={(e) => {
				const n = e.currentTarget.valueAsNumber;
				if (Number.isFinite(n)) years = Math.min(60, Math.max(0, Math.round(n * 10) / 10));
			}}
		/>
		<span class="t-2">{years === 1 ? 'year' : 'years'}</span>
		<button type="button" aria-label="More years" onclick={() => (years = Math.min(60, Math.floor(years) + 1))}>+</button>
	</div>
</div>
<div class="field"><label for="city">Current city</label><input id="city" class="input" bind:value={city} /></div>
{#if error}<Banner kind="err">{error}</Banner>{/if}
<button class="btn btn-primary btn-block" disabled={busy} onclick={confirm}>Looks good</button>

<style>
	.years {
		width: 4.5rem;
		text-align: center;
		font: inherit;
		font-weight: 600;
		color: inherit;
		background: transparent;
		border: 1px solid var(--border, #d0d5dd);
		border-radius: 8px;
		padding: 0.25rem 0.4rem;
	}
</style>
