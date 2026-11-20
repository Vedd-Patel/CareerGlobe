<script lang="ts">
	// S19 Matches: ranked jobs with the one-line reason.
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import Icon from '$lib/components/Icon.svelte';
	import JobCard from '$lib/components/JobCard.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import { EMPLOYMENT, EXPERIENCE, SALARY_LPA, jobTypeQuery } from '$lib/globe';

	type Match = Schemas['MatchCard'];
	const PAGE = 30;

	let sort = $state<'best' | 'newest'>('best');
	let remote = $state(false);
	let type = $state<'' | 'product' | 'service'>('');
	let category = $state('');
	let employment = $state<string[]>([]);
	let exp = $state('');
	let minSalaryLpa = $state(0);
	let items = $state<Match[]>([]);
	let loading = $state(true);
	let more = $state(false);
	let error = $state('');
	let categories = $state<string[]>([]);

	async function load(append = false) {
		loading = !append;
		error = '';
		try {
			const res = await api.get<{ items?: Match[] }>('/v1/matches', {
				sort, remote: remote || undefined, type: type || undefined, category: category || undefined,
				...jobTypeQuery({ employment, exp, minSalaryLpa }),
				limit: PAGE, offset: append ? items.length : 0
			});
			const got = res.items ?? [];
			items = append ? [...items, ...got] : got;
			more = got.length === PAGE;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not load matches.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void [sort, remote, type, category, employment.length, exp, minSalaryLpa];
		load();
	});

	$effect(() => {
		api.get<Schemas['Meta']>('/v1/meta').then((m) => (categories = m.categories ?? [])).catch(() => {});
	});

	async function save(jobId: number) {
		const m = items.find((x) => x.job_id === jobId);
		if (!m || m.saved) return;
		await api.post('/v1/applications', { job_id: jobId, status: 'saved' });
		m.saved = true;
	}
</script>

<svelte:head><title>Matches · CareerGlobe</title></svelte:head>

<main class="page">
	<header class="stick">
	<div class="page-head">
		<h1 class="t-title" style="flex: 1">Your matches</h1>
		<button class="icon-btn refresh" aria-label="Refresh" onclick={() => load()}><Icon name="refresh" size={18} /></button>
		<div class="segs">
			<button class="seg {sort === 'best' ? 'on' : ''}" onclick={() => (sort = 'best')}>Best match</button>
			<button class="seg {sort === 'newest' ? 'on' : ''}" onclick={() => (sort = 'newest')}>Newest</button>
		</div>
	</div>
	<div class="chips scroll-row">
		<button class="chip {remote ? 'on' : ''}" aria-pressed={remote} onclick={() => (remote = !remote)}>Remote</button>
		<button class="chip {type === 'product' ? 'on' : ''}" onclick={() => (type = type === 'product' ? '' : 'product')}>Product</button>
		<button class="chip {type === 'service' ? 'on' : ''}" onclick={() => (type = type === 'service' ? '' : 'service')}>Service</button>
		{#each EMPLOYMENT as t (t.id)}
			<button class="chip {employment.includes(t.id) ? 'on' : ''}" aria-pressed={employment.includes(t.id)}
				onclick={() => (employment = employment.includes(t.id) ? employment.filter((x) => x !== t.id) : [...employment, t.id])}>{t.label}</button>
		{/each}
		<select class="chip" bind:value={exp} aria-label="Years of experience">
			<option value="">Any experience</option>
			{#each EXPERIENCE as b (b.id)}<option value={b.id}>{b.label}</option>{/each}
		</select>
		<select class="chip" bind:value={minSalaryLpa} aria-label="Minimum salary">
			<option value={0}>Any salary</option>
			{#each SALARY_LPA as n (n)}<option value={n}>₹{n}+ LPA</option>{/each}
		</select>
		<select class="chip" bind:value={category} aria-label="Category">
			<option value="">All categories</option>
			{#each categories as c (c)}<option value={c}>{c}</option>{/each}
		</select>
	</div>
	</header>

	{#if error}
		<Banner kind="err">{error}</Banner>
	{:else if loading}
		<SkeletonCards count={5} />
	{:else if !items.length}
		<EmptyState icon="sparkles" title="No matches yet">
			<p class="t-body t-2">Matches are worked out each night after the crawl. Add more skills or widen your locations to see more.</p>
			{#snippet actions()}<a class="btn btn-secondary" href="/profile">Edit profile</a>{/snippet}
		</EmptyState>
	{:else}
		<div class="stack">
			{#each items as m (m.job_id)}
				<JobCard job={{ ...m, job_id: m.job_id ?? 0, title: m.title ?? '', reason: m.reasons?.summary }} onSave={save} />
			{/each}
		</div>
		{#if more}<button class="btn btn-secondary" onclick={() => load(true)}>Show more</button>{/if}
	{/if}
</main>
