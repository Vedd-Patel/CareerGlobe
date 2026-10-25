<script lang="ts">
	// S10 Role categories: multi-select tiles with an icon each.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import { loadPrefs, savePrefs } from '$lib/prefs.svelte';

	const CATS: [string, string, IconName][] = [
		['Software Engineering', 'Software Engineering', 'code'], ['Hardware & Semiconductor', 'Hardware & Semiconductor (VLSI, embedded)', 'chip'],
		['Design', 'Design (UI/UX)', 'pen'], ['Product', 'Product', 'box'], ['Data & AI', 'Data & AI', 'chart'],
		['QA & Testing', 'QA & Testing', 'bug'], ['DevOps & IT', 'DevOps & IT', 'server'], ['Recruiting/HR', 'Recruiting/HR', 'users'],
		['Sales', 'Sales', 'rupee'], ['Marketing', 'Marketing', 'megaphone'], ['Operations', 'Operations', 'truck'],
		['Finance', 'Finance', 'chart'], ['Customer Support', 'Customer Support', 'headset'], ['Legal', 'Legal', 'scale'], ['Other', 'Other', 'dots']
	];
	let chosen = $state<string[]>([]);
	let busy = $state(false);
	onMount(async () => (chosen = [...(await loadPrefs()).categories ?? []]));
	const toggle = (c: string) => (chosen = chosen.includes(c) ? chosen.filter((x) => x !== c) : [...chosen, c]);
	async function next() {
		busy = true;
		await savePrefs({ categories: chosen as never });
		goto('/onboarding/locations');
	}
</script>

<h1 class="t-title">What roles do you want?</h1>
<div class="grid" role="group" aria-label="Role categories">
	{#each CATS as [value, label, icon] (value)}
		<button type="button" class="tile {chosen.includes(value) ? 'on' : ''}" aria-pressed={chosen.includes(value)} onclick={() => toggle(value)}>
			<span style="color: var(--accent)"><Icon name={icon} /></span><span>{label}</span>
		</button>
	{/each}
</div>
<button class="btn btn-primary btn-block" disabled={busy} onclick={next}>Continue{chosen.length ? ` · ${chosen.length} selected` : ''}</button>

<style>
	.grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
	.tile { text-align: left; cursor: pointer; color: var(--text); font: 700 14px/18px var(--font); }
</style>
