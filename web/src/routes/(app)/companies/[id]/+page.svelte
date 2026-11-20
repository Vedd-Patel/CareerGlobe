<script lang="ts">
	// S18 Company detail: facts, offices, open jobs with your scores.
	import { page } from '$app/state';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import { isDarkTheme, mapStyle, themedStyle } from '$lib/map/style';
	import { EMPLOYMENT, EXPERIENCE, SALARY_LPA, jobTypeQuery, pinKind } from '$lib/globe';
	import { untrack } from 'svelte';
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { ago } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import JobCard from '$lib/components/JobCard.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	type Company = Schemas['CompanyDetail'];
	let c = $state<Company | null>(null);
	let error = $state('');
	let reporting = $state(false);
	let report = $state('');
	let reported = $state(false);
	let showAll = $state(false);
	// Filters for this company's open jobs, applied by the server.
	let employment = $state<string[]>([]);
	let exp = $state('');
	let minSalaryLpa = $state(0);
	let remote = $state(false);
	let city = $state('');
	let allCities = $state<string[]>([]);
	const filtering = $derived(employment.length > 0 || !!exp || minSalaryLpa > 0 || remote || !!city);
	function clearFilters() {
		employment = [];
		exp = '';
		minSalaryLpa = 0;
		remote = false;
		city = '';
	}

	const id = $derived(Number(page.params.id));
	const jobs = $derived(c?.open_jobs ?? []);
	const shown = $derived(showAll ? jobs : jobs.slice(0, 20));
	// Open roles grouped by category, best-scoring group first (S18).
	const groups = $derived.by(() => {
		const m = new Map<string, typeof jobs>();
		for (const j of shown) {
			const k = j.category ?? 'Other';
			m.set(k, [...(m.get(k) ?? []), j]);
		}
		const best = (g: typeof jobs) => Math.max(...g.map((j) => j.score ?? 0));
		return [...m.entries()].sort((a, b) => best(b[1]) - best(a[1]) || b[1].length - a[1].length);
	});

	// Small static map of the offices.
	function officeMap(el: HTMLDivElement, locs: NonNullable<Company['locations']>) {
		const pts = locs.map((l) => [l.lon ?? 0, l.lat ?? 0] as [number, number]);
		const b = new maplibregl.LngLatBounds(pts[0], pts[0]);
		for (const pt of pts) b.extend(pt);
		const map = new maplibregl.Map({
			container: el, style: mapStyle(isDarkTheme()), interactive: false, attributionControl: false,
			...(pts.length > 1 ? { bounds: b, fitBoundsOptions: { padding: 40, maxZoom: 7 } } : { center: pts[0], zoom: 3.4 })
		});
		if (isDarkTheme()) themedStyle(true).then((st) => map.setStyle(st)); // the lifted dark map
		for (const pt of pts) {
			const m = document.createElement('span');
			m.className = 'pin-hit';
			const dot = document.createElement('span');
			dot.className = `pin ${pinKind({ open_jobs: jobs.length })}`;
			m.append(dot);
			new maplibregl.Marker({ element: m }).setLngLat(pt).addTo(map);
		}
		return { destroy: () => map.remove() };
	}

	$effect(() => {
		void id;
		c = null;
		error = '';
		clearFilters();
		api.get<Company>(`/v1/companies/${id}`).then(
			(r) => {
				c = r;
				allCities = [...new Set((r.open_jobs ?? []).flatMap((j) => j.cities ?? []))].sort();
			},
			(e) => (error = e instanceof ApiError && e.status === 404 ? 'This company is not in CareerGlobe.' : 'Could not load the company.')
		);
	});

	// Changing a filter asks the server for just the jobs that pass it.
	$effect(() => {
		const params = { ...jobTypeQuery({ employment, exp, minSalaryLpa }), remote: remote || undefined, city: city || undefined };
		untrack(() => {
			if (!c) return;
			showAll = false;
			api.get<Company>(`/v1/companies/${id}`, params).then((r) => {
				if (c) {
					c.open_jobs = r.open_jobs;
					c.open_total = r.open_total;
				}
			});
		});
	});

	async function toggleSave() {
		if (!c) return;
		await api[c.saved ? 'del' : 'put'](`/v1/companies/${c.id}/save`);
		c.saved = !c.saved;
	}

	async function sendReport() {
		if (!c || !report.trim()) return;
		await api.post('/v1/reports', { company_id: c.id, message: report.trim() });
		reported = true;
		reporting = false;
	}
</script>

<svelte:head><title>{c?.name ?? 'Company'} · CareerGlobe</title></svelte:head>

<main class="page">
	<button class="btn btn-text" style="align-self: flex-start" onclick={() => history.back()}><Icon name="back" size={18} />Back</button>
	{#if error}
		<Banner kind="err">{error}</Banner>
	{:else if !c}
		<SkeletonCards count={3} />
	{:else}
		<header style="display: flex; gap: 16px; align-items: center">
			<Logo name={c.name ?? ''} url={c.logo_url} size="lg" />
			<div style="flex: 1; min-width: 0">
				<h1 class="t-title">{c.name}</h1>
				<p class="t-body t-2">{[c.industry, c.type, c.size_band, c.stage].filter(Boolean).join(' · ')}</p>
			</div>
			<button class="icon-btn" aria-label={c.saved ? 'Saved' : 'Save company'} aria-pressed={c.saved} onclick={toggleSave}
				style={c.saved ? 'background: var(--accent); color: var(--on-accent); border-color: var(--accent)' : ''}>
				<Icon name="bookmark" fill={c.saved} />
			</button>
		</header>

		{#if c.description}<p class="t-body t-2">{c.description}</p>{/if}

		<div class="chips">
			{#if c.website}<a class="chip" href={c.website} target="_blank" rel="noopener noreferrer">Website<Icon name="external" size={14} /></a>{/if}
			{#if c.careers_url}<a class="chip" href={c.careers_url} target="_blank" rel="noopener noreferrer">Careers page<Icon name="external" size={14} /></a>{/if}
		</div>
		<p class="t-caption t-3">Careers page last checked {ago(c.last_checked_at) || 'not yet'}</p>

		{#if c.locations?.length}
			<section class="stack">
				<h2 class="t-heading">Offices in India</h2>
				<div class="office-map" use:officeMap={c.locations} aria-label="Map of offices"></div>
				<div class="chips">
					{#each c.locations as l (l.id)}
						<span class="chip"><Icon name="pin" size={14} />{l.city ?? l.state}{l.is_hq ? ' · HQ' : ''}{l.geo_approximate ? ' (approx.)' : ''}</span>
					{/each}
				</div>
			</section>
		{/if}

		<section class="stack">
			<h2 class="t-heading">Open jobs <span class="t-3">({filtering ? `${jobs.length} of ${c.open_total ?? jobs.length}` : jobs.length})</span></h2>
			<div class="chips" aria-label="Filter these jobs">
				{#each EMPLOYMENT as t (t.id)}
					<button class="chip {employment.includes(t.id) ? 'on' : ''}" aria-pressed={employment.includes(t.id)}
						onclick={() => (employment = employment.includes(t.id) ? employment.filter((x) => x !== t.id) : [...employment, t.id])}>{t.label}</button>
				{/each}
				<button class="chip {remote ? 'on' : ''}" aria-pressed={remote} onclick={() => (remote = !remote)}>Remote</button>
				<select class="chip" bind:value={city} aria-label="Location">
					<option value="">All locations</option>
					{#each allCities as ct (ct)}<option value={ct}>{ct}</option>{/each}
				</select>
				<select class="chip" bind:value={exp} aria-label="Years of experience">
					<option value="">Any experience</option>
					{#each EXPERIENCE as b (b.id)}<option value={b.id}>{b.label}</option>{/each}
				</select>
				<select class="chip" bind:value={minSalaryLpa} aria-label="Minimum salary">
					<option value={0}>Any salary</option>
					{#each SALARY_LPA as n (n)}<option value={n}>₹{n}+ LPA</option>{/each}
				</select>
				{#if filtering}<button class="chip" onclick={clearFilters}>Clear filters</button>{/if}
			</div>
			{#each groups as [cat, list] (cat)}
				<h3 class="t-caption t-2">{cat.toUpperCase()} · {list.length}</h3>
				{#each list as j (j.id)}
					<JobCard job={{ ...j, job_id: j.id ?? 0, title: j.title ?? '', first_seen: j.posted_at ?? j.first_seen }} />
				{/each}
			{:else}
				<EmptyState icon="box" title={filtering ? 'No jobs match these filters' : 'No open jobs right now'}>
					<p class="t-body t-2">{filtering ? 'Try a different type, location or experience, or clear the filters.' : "Save this company and we'll tell you when it posts something new."}</p>
				</EmptyState>
			{/each}
			{#if jobs.length > shown.length}
				<button class="btn btn-secondary" onclick={() => (showAll = true)}>Show all {jobs.length}</button>
			{/if}
		</section>

		{#if reported}
			<Banner kind="ok">Thanks. We'll check it.</Banner>
		{:else if reporting}
			<section class="stack">
				<textarea class="input" rows="3" maxlength="2000" placeholder="What's wrong? e.g. wrong city, not hiring in India" bind:value={report}></textarea>
				<div style="display: flex; gap: 10px">
					<button class="btn btn-primary" disabled={!report.trim()} onclick={sendReport}>Send</button>
					<button class="btn btn-text" onclick={() => (reporting = false)}>Cancel</button>
				</div>
			</section>
		{:else}
			<button class="btn btn-text" style="align-self: flex-start" onclick={() => (reporting = true)}><Icon name="alert" size={16} />Report wrong info</button>
		{/if}
	{/if}
</main>

<style>
	.office-map { height: 200px; border-radius: var(--r-lg); overflow: hidden; border: 1px solid var(--line); }
</style>
