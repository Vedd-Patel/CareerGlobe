<script lang="ts">
	// S11 Preferred locations: city search with chips, remote toggle, anywhere in India.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, type Schemas } from '$lib/api/client';
	import { loadPrefs, savePrefs } from '$lib/prefs.svelte';
	import Icon from '$lib/components/Icon.svelte';

	const POPULAR = ['Bengaluru', 'Pune', 'Hyderabad', 'Mumbai', 'Delhi', 'Gurugram', 'Noida', 'Chennai'];
	let cities = $state<string[]>([]);
	let all = $state<string[]>([]);
	let q = $state('');
	let remote = $state(true);
	let anywhere = $state(false);
	onMount(async () => {
		const p = await loadPrefs();
		cities = [...(p.cities ?? [])];
		remote = p.remote_ok ?? true;
		anywhere = p.anywhere ?? false;
		const meta = await api.get<Schemas['Meta']>('/v1/meta');
		all = (meta.cities ?? []).map((c) => c.city ?? '').filter(Boolean);
	});
	const suggestions = $derived(q.length < 2 ? [] : [...new Set([...POPULAR, ...all])].filter((c) => c.toLowerCase().startsWith(q.toLowerCase()) && !cities.includes(c)).slice(0, 6));
	const toggle = (c: string) => (cities = cities.includes(c) ? cities.filter((x) => x !== c) : [...cities, c]);
	async function next() {
		await savePrefs({ cities, remote_ok: remote, anywhere });
		goto('/onboarding/level');
	}
</script>

<h1 class="t-title">Where do you want to work?</h1>
<div class="search"><Icon name="search" size={18} /><input aria-label="Search a city" placeholder="Search a city" bind:value={q} style="border: 0; outline: 0; background: none; flex: 1; font: inherit; color: var(--text)" /></div>
{#if suggestions.length}
	<div class="chips">{#each suggestions as s (s)}<button class="chip" onclick={() => { toggle(s); q = ''; }}>+ {s}</button>{/each}</div>
{/if}
<div class="chips">
	{#each [...new Set([...cities, ...POPULAR])] as c (c)}
		<button class="chip {cities.includes(c) ? 'on' : ''}" aria-pressed={cities.includes(c)} onclick={() => toggle(c)}>{c}{cities.includes(c) ? ' ✕' : ''}</button>
	{/each}
</div>
<label class="card" style="align-items: center; justify-content: space-between; box-shadow: none; cursor: pointer">
	<span><span class="t-heading" style="font-size: 16px; display: block">Open to remote</span><span class="t-caption t-3">Include remote jobs anywhere in India</span></span>
	<input type="checkbox" bind:checked={remote} style="width: 22px; height: 22px; accent-color: var(--accent)" />
</label>
<label class="card" style="align-items: center; justify-content: space-between; box-shadow: none; cursor: pointer">
	<span><span class="t-heading" style="font-size: 16px; display: block">Anywhere in India</span><span class="t-caption t-3">Show every city, best fits first</span></span>
	<input type="checkbox" bind:checked={anywhere} style="width: 22px; height: 22px; accent-color: var(--accent)" />
</label>
<button class="btn btn-primary btn-block" onclick={next}>Continue</button>
