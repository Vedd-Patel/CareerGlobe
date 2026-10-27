<script lang="ts">
	// S14 Globe home, S15 company preview, S16 filters sheet, S17 search.
	import { onMount, onDestroy } from 'svelte';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { mapStyle } from '$lib/map/style';
	import { activeCount, clusterSize, emptyFilters, filtersToQuery, pinKind, type Filters } from '$lib/globe';
	import { updatedLabel } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Banner from '$lib/components/Banner.svelte';

	type Pin = Schemas['Pin'];
	type Cluster = Schemas['Cluster'];
	type Row = NonNullable<Schemas['CompanyListResponse']['items']>[number];
	type SearchResults = {
		companies: Schemas['CompanyRef'][];
		jobs: Schemas['JobRef'][];
		cities: { city?: string | null; companies?: number; lat?: number; lon?: number }[];
	};

	let container: HTMLDivElement;
	let map: maplibregl.Map | null = null;
	let markers: maplibregl.Marker[] = [];
	let filters = $state<Filters>(emptyFilters());
	let inView = $state<Row[]>([]);
	let updated = $state<string | null>(null);
	let selected = $state<Pin | null>(null);
	let sheet = $state<'list' | 'filters' | 'search'>('list');
	let error = $state('');
	let q = $state('');
	let results = $state<SearchResults | null>(null);
	let firstVisit = $state(false);
	let clusterList = $state<Pin[] | null>(null);
	let liveCount = $state<number | null>(null);
	let recent = $state<string[]>([]);
	const CATEGORIES = ['Software Engineering', 'Hardware & Semiconductor', 'Design', 'Product', 'Data & AI', 'QA & Testing',
		'DevOps & IT', 'Sales', 'Marketing', 'Operations', 'Finance', 'Customer Support'];

	let timer: ReturnType<typeof setTimeout> | undefined;
	function refresh() {
		clearTimeout(timer);
		timer = setTimeout(load, 250);
	}

	function viewBounds() {
		const b = map!.getBounds();
		return {
			west: Math.max(-180, b.getWest()), south: Math.max(-90, b.getSouth()),
			east: Math.min(180, b.getEast()), north: Math.min(90, b.getNorth())
		};
	}

	// Pins (zoom 14 = never clustered) in the current view or a small box.
	async function pinsIn(bounds: { west: number; south: number; east: number; north: number }, f: Filters): Promise<Pin[]> {
		const g = await api.get<{ mode: string; items: Pin[] }>('/v1/globe', { ...bounds, zoom: 14, ...filtersToQuery(f) });
		return g.mode === 'pins' ? g.items : [];
	}

	async function load() {
		if (!map) return;
		const bounds = viewBounds();
		try {
			const [g, list] = await Promise.all([
				api.get<{ mode: 'pins' | 'clusters'; items: (Pin | Cluster)[]; updated_at?: string | null }>('/v1/globe', {
					...bounds, zoom: map.getZoom().toFixed(2), ...filtersToQuery(filters)
				}),
				api.get<Schemas['CompanyListResponse']>('/v1/companies/in-view', { ...bounds, type: filters.type, limit: 40 })
			]);
			error = '';
			updated = g.updated_at ?? null;
			inView = list.items ?? [];
			draw(g.mode, g.items);
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not load the map.';
		}
	}

	function draw(mode: 'pins' | 'clusters', items: (Pin | Cluster)[]) {
		if (!map) return;
		markers.forEach((m) => m.remove());
		markers = [];
		if (mode === 'clusters') {
			for (const c of items as Cluster[]) {
				const el = bubble(c.companies ?? 0, c.matches ?? 0, c.open_jobs ?? 0, `${c.companies} companies, ${c.matches} matches. Zoom in.`);
				el.onclick = () => openCluster(c);
				add(el, c.lon, c.lat);
			}
			return;
		}
		// Companies placed at the same spot (often a city centre) share one bubble
		// that opens a short list (S15); a lone company gets a pin.
		const spots = new Map<string, Pin[]>();
		for (const p of items as Pin[]) {
			const k = `${(p.lat ?? 0).toFixed(4)},${(p.lon ?? 0).toFixed(4)}`;
			spots.set(k, [...(spots.get(k) ?? []), p]);
		}
		for (const group of spots.values()) {
			const p = group[0];
			if (group.length > 1) {
				const matches = group.reduce((n, g) => n + (g.matches ?? 0), 0);
				const open = group.reduce((n, g) => n + (g.open_jobs ?? 0), 0);
				const el = bubble(group.length, matches, open, `${group.length} companies here, ${matches} matches`);
				el.onclick = () => {
					clusterList = [...group].sort((a, b) => (b.matches ?? 0) - (a.matches ?? 0) || (b.open_jobs ?? 0) - (a.open_jobs ?? 0));
					selected = null;
					sheet = 'list';
				};
				add(el, p.lon, p.lat);
				continue;
			}
			const el = document.createElement('button');
			el.type = 'button';
			el.className = 'pin-hit';
			const dot = document.createElement('span');
			dot.className = `pin ${pinKind(p)}${selected?.company_id === p.company_id ? ' sel' : ''}`;
			el.append(dot);
			el.setAttribute('aria-label', `${p.name}, ${p.open_jobs} open jobs, ${p.matches} matches`);
			el.onclick = () => select(p);
			add(el, p.lon, p.lat);
		}
	}

	function bubble(n: number, matches: number, open: number, label: string): HTMLButtonElement {
		const el = document.createElement('button');
		el.type = 'button';
		el.className = `cluster ${clusterSize(n)}`;
		el.textContent = String(n);
		el.setAttribute('aria-label', label);
		if (!matches) el.style.background = open > 0 ? 'var(--pin-hiring)' : 'var(--pin-idle)';
		return el;
	}

	function add(el: HTMLElement, lon = 0, lat = 0) {
		markers.push(new maplibregl.Marker({ element: el }).setLngLat([lon, lat]).addTo(map!));
	}

	async function openCluster(c: Cluster) {
		if (!map) return;
		const z = map.getZoom();
		if (z < 9) {
			map.easeTo({ center: [c.lon ?? 0, c.lat ?? 0], zoom: Math.min(z + 2.5, 12), duration: 600 });
			return;
		}
		// Many companies at one address: show them as a short list (S15).
		const d = 0.05;
		const lat = c.lat ?? 0, lon = c.lon ?? 0;
		clusterList = await pinsIn({ west: lon - d, south: lat - d, east: lon + d, north: lat + d }, filters).catch(() => []);
		selected = null;
		sheet = 'list';
	}

	function select(p: Pin) {
		clusterList = null;
		selected = p;
		sheet = 'list';
		refresh();
	}

	let savedIds = $state<number[]>([]);
	async function toggleSave(id: number) {
		const saved = savedIds.includes(id);
		await api[saved ? 'del' : 'put'](`/v1/companies/${id}/save`);
		savedIds = saved ? savedIds.filter((x) => x !== id) : [...savedIds, id];
	}

	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const text = q.trim();
		clearTimeout(searchTimer);
		if (text.length < 2) {
			results = null;
			return;
		}
		searchTimer = setTimeout(async () => {
			try {
				results = await api.get<SearchResults>('/v1/search', { q: text });
				if (results.companies.length + results.jobs.length + results.cities.length) remember(text);
			} catch {
				results = null;
			}
		}, 200);
	});

	const RECENT = 'cg.recentSearches';
	function remember(text: string) {
		recent = [text, ...recent.filter((r) => r.toLowerCase() !== text.toLowerCase())].slice(0, 6);
		try {
			localStorage.setItem(RECENT, JSON.stringify(recent));
		} catch {
			/* ignore */
		}
	}

	function flyToCity(lat?: number, lon?: number) {
		if (lat == null || lon == null) return;
		sheet = 'list';
		q = '';
		map?.flyTo({ center: [lon, lat], zoom: 10, duration: 900 });
	}

	let countTimer: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		if (sheet !== 'filters' || !map) return;
		const f = $state.snapshot(filters) as Filters;
		clearTimeout(countTimer);
		countTimer = setTimeout(async () => {
			liveCount = (await pinsIn(viewBounds(), f).catch(() => [])).reduce(
				(ids, p) => ids.add(p.company_id ?? 0), new Set<number>()
			).size;
		}, 250);
	});

	function applyFilters() {
		sheet = 'list';
		refresh();
	}

	onMount(() => {
		try {
			recent = JSON.parse(localStorage.getItem(RECENT) ?? '[]');
		} catch {
			recent = [];
		}
		try {
			firstVisit = localStorage.getItem('cg.globeHint') !== '1';
		} catch {
			firstVisit = false;
		}
		const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
		map = new maplibregl.Map({ container, style: mapStyle(), center: [79, 22], zoom: 3.2, attributionControl: { compact: true } });
		map.on('style.load', () => map?.setProjection({ type: 'globe' }));
		map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'bottom-right');
		map.on('load', load);
		map.on('moveend', refresh);
		if (!reduced) {
			// Slow idle spin (design: 1 turn / 120 s) until the first touch.
			let spinning = true;
			const spin = () => {
				if (!spinning || !map || map.getZoom() > 4) return;
				const c = map.getCenter();
				map.easeTo({ center: [c.lng + 3, c.lat], duration: 1000, easing: (t) => t });
			};
			map.on('moveend', () => spinning && spin());
			map.on('mousedown', () => (spinning = false));
			map.on('touchstart', () => (spinning = false));
			map.on('wheel', () => (spinning = false));
			map.once('load', spin);
		}
	});

	onDestroy(() => {
		clearTimeout(timer);
		clearTimeout(searchTimer);
		map?.remove();
	});

	function dismissHint() {
		firstVisit = false;
		try {
			localStorage.setItem('cg.globeHint', '1');
		} catch {
			/* ignore */
		}
	}

	function toggle(key: 'matches' | 'remote' | 'hiring') {
		filters[key] = !filters[key];
		refresh();
	}
</script>

<svelte:head><title>Globe · CareerGlobe</title></svelte:head>

<div class="globe-page">
	<div class="map-wrap space">
		<div class="map" bind:this={container}></div>
		<div class="top">
			<div class="row">
				<button class="search glass" onclick={() => (sheet = 'search')}><Icon name="search" size={18} />Companies, jobs, cities</button>
				<button class="icon-btn glass" aria-label="Filters, {activeCount(filters)} on" onclick={() => (sheet = 'filters')}><Icon name="filter" /></button>
			</div>
			<div class="chips nowrap">
				<button class="chip {filters.matches ? 'on' : 'glass'}" aria-pressed={filters.matches} onclick={() => toggle('matches')}>Matches only</button>
				<button class="chip {filters.remote ? 'on' : 'glass'}" aria-pressed={filters.remote} onclick={() => toggle('remote')}>Remote</button>
				<button class="chip {filters.category === 'Software Engineering' ? 'on' : 'glass'}"
					onclick={() => { filters.category = filters.category === 'Software Engineering' ? '' : 'Software Engineering'; refresh(); }}>Software</button>
				<button class="chip {filters.type === 'product' ? 'on' : 'glass'}"
					onclick={() => { filters.type = filters.type === 'product' ? '' : 'product'; refresh(); }}>Product</button>
				<button class="chip {filters.hiring ? 'on' : 'glass'}" aria-pressed={filters.hiring} onclick={() => toggle('hiring')}>Hiring now</button>
			</div>
			{#if updated}<span class="pill glass updated">{updatedLabel(updated)}</span>{/if}
		</div>
		{#if error}<div class="err"><Banner kind="err">{error}</Banner></div>{/if}
		{#if firstVisit}
			<div class="hint glass" role="dialog" aria-label="How to use the globe">
				<p class="t-heading">Pinch to zoom, drag to spin</p>
				<p class="t-body" style="color: #a6b0c9">Glowing teal pins have jobs that match you. Tap a pin to see the company.</p>
				<button class="btn btn-primary" onclick={dismissHint}>Got it</button>
			</div>
		{/if}
	</div>

	<aside class="panel">
		{#if selected}
			<section class="preview" aria-label="Company preview">
				<div style="display: flex; gap: 14px; align-items: center">
					<Logo name={selected.name ?? ''} url={selected.logo_url} size="lg" />
					<div style="flex: 1; min-width: 0">
						<h2 class="t-title">{selected.name}</h2>
						<div class="meta">{#if selected.type}<span class="pill p-saved">{selected.type}</span>{/if}<span>{selected.city ?? ''}</span></div>
					</div>
					<button class="icon-btn" aria-label="Close preview" onclick={() => { selected = null; refresh(); }}><Icon name="x" /></button>
				</div>
				<div style="display: flex; gap: 28px">
					<div><p class="num" style="font-size: 24px">{selected.open_jobs}</p><p class="t-caption t-3">open jobs</p></div>
					<div><p class="num t-accent" style="font-size: 24px">{selected.matches}</p><p class="t-caption t-3">your matches</p></div>
				</div>
				<div style="display: flex; gap: 10px">
					<button class="btn btn-secondary" style="flex: 1" aria-pressed={savedIds.includes(selected.company_id ?? 0)}
						onclick={() => selected && toggleSave(selected.company_id ?? 0)}>
						<Icon name="bookmark" size={18} fill={savedIds.includes(selected.company_id ?? 0)} />{savedIds.includes(selected.company_id ?? 0) ? 'Saved' : 'Save'}
					</button>
					<a class="btn btn-primary" style="flex: 2" href="/companies/{selected.company_id}">View company</a>
				</div>
			</section>
		{/if}

		{#if clusterList && sheet === 'list'}
			<section class="preview" aria-label="Companies here">
				<div style="display: flex; justify-content: space-between; align-items: center">
					<h2 class="t-heading">{clusterList.length} companies here</h2>
					<button class="icon-btn" aria-label="Close list" onclick={() => (clusterList = null)}><Icon name="x" /></button>
				</div>
				{#each clusterList as p (p.location_id)}
					<button class="list-row plain" onclick={() => select(p)}>
						<Logo name={p.name ?? ''} url={p.logo_url} size="sm" />
						<span style="flex: 1; text-align: left"><b>{p.name}</b><br /><span class="t-caption t-3">{p.open_jobs} open</span></span>
						<span class="num t-accent">{p.matches}</span>
					</button>
				{/each}
			</section>
		{/if}

		{#if sheet === 'filters'}
			<section class="sheet-body" aria-label="Filters">
				<div style="display: flex; justify-content: space-between; align-items: center">
					<h2 class="t-title">Filters</h2>
					<button class="btn btn-text" onclick={() => (filters = emptyFilters())}>Reset</button>
				</div>
				<p class="t-caption t-2">ROLE CATEGORY</p>
				<div class="chips">
					{#each CATEGORIES as c (c)}
						<button class="chip {filters.category === c ? 'on' : ''}" aria-pressed={filters.category === c}
							onclick={() => (filters.category = filters.category === c ? '' : c)}>{c}</button>
					{/each}
				</div>
				<p class="t-caption t-2">WORK MODE</p>
				<div class="segs">
					<button class="seg {filters.remote ? '' : 'on'}" onclick={() => (filters.remote = false)}>Any</button>
					<button class="seg {filters.remote ? 'on' : ''}" onclick={() => (filters.remote = true)}>Remote</button>
				</div>
				<p class="t-caption t-2">COMPANY</p>
				<div class="segs">
					<button class="seg {filters.type === '' ? 'on' : ''}" onclick={() => (filters.type = '')}>Any</button>
					<button class="seg {filters.type === 'product' ? 'on' : ''}" onclick={() => (filters.type = 'product')}>Product</button>
					<button class="seg {filters.type === 'service' ? 'on' : ''}" onclick={() => (filters.type = 'service')}>Service</button>
				</div>
				<p class="t-caption t-2">POSTED WITHIN</p>
				<div class="segs">
					<button class="seg {filters.posted === '' ? 'on' : ''}" onclick={() => (filters.posted = '')}>Any</button>
					<button class="seg {filters.posted === '24h' ? 'on' : ''}" onclick={() => (filters.posted = '24h')}>24h</button>
					<button class="seg {filters.posted === '7d' ? 'on' : ''}" onclick={() => (filters.posted = '7d')}>7 days</button>
					<button class="seg {filters.posted === '30d' ? 'on' : ''}" onclick={() => (filters.posted = '30d')}>30 days</button>
				</div>
				<label class="card" style="align-items: center; justify-content: space-between; box-shadow: none; padding: 12px 16px">
					<span class="t-heading" style="font-size: 15px">Matches only</span>
					<input type="checkbox" bind:checked={filters.matches} style="width: 22px; height: 22px; accent-color: var(--accent)" />
				</label>
				<button class="btn btn-primary btn-block" onclick={applyFilters}>{liveCount == null ? 'Show companies' : `Show ${liveCount} ${liveCount === 1 ? 'company' : 'companies'}`}</button>
			</section>
		{:else if sheet === 'search'}
			<section class="sheet-body" aria-label="Search">
				<div style="display: flex; gap: 8px">
					<!-- svelte-ignore a11y_autofocus -->
					<input class="input" placeholder="Companies, jobs, cities" bind:value={q} autofocus aria-label="Search" />
					<button class="btn btn-text" onclick={() => { sheet = 'list'; q = ''; }}>Cancel</button>
				</div>
				{#if !results && recent.length}
					<p class="t-caption t-2">RECENT</p>
					<div class="chips">{#each recent as r (r)}<button class="chip" onclick={() => (q = r)}>{r}</button>{/each}</div>
				{/if}
				{#if results}
					{#if results.cities.length}<p class="t-caption t-2">CITIES</p>{/if}
					{#each results.cities as c (c.city)}
						<button class="list-row plain" onclick={() => flyToCity(c.lat, c.lon)}>
							<Icon name="pin" /><span style="flex: 1; text-align: left"><b>{c.city}</b><br /><span class="t-caption t-3">{c.companies} companies</span></span>
						</button>
					{/each}
					{#if results.companies.length}<p class="t-caption t-2">COMPANIES</p>{/if}
					{#each results.companies as c (c.id)}
						<a class="list-row plain" href="/companies/{c.id}"><Logo name={c.name ?? ''} url={c.logo_url} size="sm" /><span style="flex: 1"><b>{c.name}</b><br /><span class="t-caption t-3">{c.type ?? ''}</span></span></a>
					{/each}
					{#if results.jobs.length}<p class="t-caption t-2">JOBS</p>{/if}
					{#each results.jobs as j (j.id)}
						<a class="list-row plain" href="/jobs/{j.id}"><Logo name={j.company_name ?? ''} url={j.logo_url} size="sm" /><span style="flex: 1"><b>{j.title}</b><br /><span class="t-caption t-3">{j.company_name} · {(j.cities ?? []).join(', ')}</span></span></a>
					{/each}
					{#if !results.cities.length && !results.companies.length && !results.jobs.length}<p class="t-body t-2">No results for “{q}”.</p>{/if}
				{/if}
			</section>
		{:else}
			<section class="sheet-body" aria-label="Companies in view">
				<div style="display: flex; justify-content: space-between; align-items: baseline"><h2 class="t-heading">Companies in view</h2><span class="t-caption t-3">by matches</span></div>
				{#each inView as c (c.id)}
					<a class="list-row plain" href="/companies/{c.id}">
						<Logo name={c.name ?? ''} url={c.logo_url} size="sm" />
						<span style="flex: 1; min-width: 0"><b class="t-heading" style="font-size: 15px">{c.name}</b><br /><span class="t-caption t-3">{[c.type, c.city, `${c.open_jobs} open`].filter(Boolean).join(' · ')}</span></span>
						<span class="num t-accent">{c.matches}</span>
					</a>
				{:else}
					<p class="t-body t-2">No companies here. Zoom out or move the globe.</p>
				{/each}
			</section>
		{/if}
	</aside>
</div>

<style>
	.globe-page { display: flex; flex-direction: column; height: calc(100vh - 76px); }
	.map-wrap { position: relative; flex: 1; min-height: 52vh; }
	.map { position: absolute; inset: 0; }
	.top { position: absolute; top: 16px; left: 16px; right: 16px; display: flex; flex-direction: column; gap: 10px; z-index: 2; pointer-events: none; }
	.top > * { pointer-events: auto; }
	.row { display: flex; gap: 10px; max-width: 560px; }
	.search { color: #a6b0c9; border: 1px solid rgba(255, 255, 255, 0.1); cursor: pointer; }
	.icon-btn.glass { color: #e9edf7; cursor: pointer; }
	.nowrap { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; }
	.chip { cursor: pointer; }
	.chip.glass { border-color: rgba(255, 255, 255, 0.1); }
	.updated { align-self: flex-start; color: #a6b0c9; }
	.err { position: absolute; left: 16px; right: 16px; bottom: 16px; z-index: 3; }
	.hint { position: absolute; left: 50%; top: 40%; transform: translate(-50%, -50%); width: min(340px, 90%); border-radius: 20px; padding: 20px; display: flex; flex-direction: column; gap: 12px; text-align: center; z-index: 4; }
	.panel { background: var(--surface); border-top: 1px solid var(--line); max-height: 45vh; overflow-y: auto; }
	.preview, .sheet-body { padding: 20px; display: flex; flex-direction: column; gap: 14px; }
	.preview { border-bottom: 1px solid var(--line); }
	.plain { color: inherit; text-decoration: none; background: none; border-left: 0; border-right: 0; border-top: 0; width: 100%; font: inherit; cursor: pointer; }
	.seg { border: 0; cursor: pointer; font: 700 13px var(--font); }
	:global(.maplibregl-ctrl-attrib) { font-size: 10px; }
	:global(.maplibregl-marker.cluster) { cursor: pointer; padding: 0; }
	@media (min-width: 900px) {
		.globe-page { flex-direction: row; height: 100vh; }
		.panel { width: 440px; max-height: none; border-top: 0; border-left: 1px solid var(--line); }
	}
</style>
