<script lang="ts">
	// S26 Saved: companies you follow and jobs saved to the tracker.
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import JobCard from '$lib/components/JobCard.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	let tab = $state<'companies' | 'jobs'>('companies');
	let companies = $state<Schemas['SavedCompany'][] | null>(null);
	let jobs = $state<Schemas['ApplicationCard'][] | null>(null);
	let error = $state('');

	$effect(() => {
		Promise.all([
			api.get<{ items?: Schemas['SavedCompany'][] }>('/v1/saved/companies'),
			api.get<{ items?: Schemas['ApplicationCard'][] }>('/v1/applications')
		]).then(
			([c, a]) => {
				companies = c.items ?? [];
				jobs = (a.items ?? []).filter((x) => x.status === 'saved');
			},
			(e) => (error = e instanceof ApiError ? e.message : 'Could not load saved items.')
		);
	});

	async function unsave(id: number) {
		await api.del(`/v1/companies/${id}/save`);
		companies = (companies ?? []).filter((c) => c.id !== id);
	}
</script>

<svelte:head><title>Saved · CareerGlobe</title></svelte:head>

<main class="page">
	<div class="page-head">
		<h1 class="t-title">Saved</h1>
		<div class="segs">
			<button class="seg {tab === 'companies' ? 'on' : ''}" onclick={() => (tab = 'companies')}>Companies {companies?.length ?? ''}</button>
			<button class="seg {tab === 'jobs' ? 'on' : ''}" onclick={() => (tab = 'jobs')}>Jobs {jobs?.length ?? ''}</button>
		</div>
	</div>

	{#if error}
		<Banner kind="err">{error}</Banner>
	{:else if companies === null || jobs === null}
		<SkeletonCards count={4} />
	{:else if tab === 'companies'}
		{#each companies as c (c.id)}
			<div class="list-row">
				<a class="plain-link" href="/companies/{c.id}" style="display: flex; gap: 14px; align-items: center; flex: 1; min-width: 0">
					<Logo name={c.name ?? ''} url={c.logo_url} />
					<span><b class="t-heading" style="font-size: 16px">{c.name}</b><br /><span class="t-caption t-3">{c.open_jobs} open jobs</span></span>
				</a>
				<button class="icon-btn" aria-label="Unsave {c.name}" onclick={() => unsave(c.id ?? 0)}
					style="background: var(--accent); color: var(--on-accent); border-color: var(--accent)"><Icon name="bookmark" fill /></button>
			</div>
		{:else}
			<EmptyState icon="bookmark" title="No saved companies" tone="saved">
				<p class="t-body t-2">Save a company from the globe and we'll tell you when it posts a job that fits you.</p>
				{#snippet actions()}<a class="btn btn-primary" href="/globe">Open the globe</a>{/snippet}
			</EmptyState>
		{/each}
	{:else}
		<div class="stack">
			{#each jobs as j (j.id)}
				<JobCard job={{ job_id: j.job_id ?? 0, title: j.title ?? '', company_name: j.company_name, cities: j.cities, work_mode: j.work_mode,
					first_seen: j.created_at, saved: true, closed: j.job_status === 'closed' }} />
			{:else}
				<EmptyState icon="bookmark" title="No saved jobs" tone="saved">
					<p class="t-body t-2">Tap the bookmark on any job to keep it here and in your tracker.</p>
					{#snippet actions()}<a class="btn btn-primary" href="/matches">See your matches</a>{/snippet}
				</EmptyState>
			{/each}
		</div>
	{/if}
</main>
