<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { session } from '$lib/auth.svelte';
	import { requireMember } from '$lib/flows/guard';

	let { children }: { children: Snippet } = $props();
	const steps = ['resume', 'review', 'roles', 'locations', 'level', 'notifications'];
	const index = $derived(Math.max(0, steps.indexOf(page.url.pathname.split('/')[2] ?? 'resume')));
	let ok = $state(false);
	$effect(() => {
		if (session.ready) requireMember().then((m) => (ok = m));
	});
</script>

{#if ok}
	<main class="onb">
		<div class="progress" role="progressbar" aria-valuemin="1" aria-valuemax="6" aria-valuenow={index + 1}>
			<i style="width: {((index + 1) / steps.length) * 100}%"></i>
		</div>
		<p class="t-caption t-3">Step {index + 1} of {steps.length}</p>
		{@render children()}
	</main>
{/if}

<style>
	.onb { max-width: 560px; margin: 0 auto; padding: 32px 20px 48px; display: flex; flex-direction: column; gap: 16px; min-height: 100vh; }
</style>
