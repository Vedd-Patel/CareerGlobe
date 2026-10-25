<script lang="ts">
	// S30 Admin console: crawl health, failing sources, companies, category rules,
	// access codes, removal requests and reports. The API checks is_admin on every call.
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { account } from '$lib/account.svelte';
	import { ago, shortDate } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Banner from '$lib/components/Banner.svelte';

	type Tab = 'crawl' | 'sources' | 'company' | 'rules' | 'codes' | 'removals' | 'reports';
	const TABS: { key: Tab; label: string }[] = [
		{ key: 'crawl', label: 'Crawl' }, { key: 'sources', label: 'Failing' }, { key: 'company', label: 'Add company' },
		{ key: 'rules', label: 'Categories' }, { key: 'codes', label: 'Codes' }, { key: 'removals', label: 'Removals' }, { key: 'reports', label: 'Reports' }
	];
	const SOURCE_STATUSES = ['active', 'broken', 'blocked', 'review', 'disabled'] as const;

	let tab = $state<Tab>('crawl');
	let error = $state('');
	let ok = $state('');

	// Crawl
	let crawl = $state<{ health?: string; failed_code_guesses_last_hour?: number; runs?: Schemas['CrawlRun'][] } | null>(null);
	// Sources
	let sources = $state<Schemas['CareersSource'][]>([]);
	let editing = $state<{ id: number; careers_url: string; ats_type: string; ats_slug: string; selectors: string; status: string } | null>(null);
	// Company
	let co = $state({ name: '', website: '', careers_url: '', city: '', state: '', type: 'product', ats_type: '', ats_slug: '' });
	// Rules
	let rules = $state<Schemas['CategoryRule'][]>([]);
	let ruleCats = $state<string[]>([]);
	let unclassified = $state<{ title?: string; jobs?: number }[]>([]);
	let rule = $state({ keyword: '', category: 'Software Engineering', priority: 5 });
	// Codes
	let codes = $state<Schemas['AccessCode'][]>([]);
	let newCode = $state({ label: '', max_uses: 1, expires: '' });
	let created = $state<{ code?: string; note?: string } | null>(null);
	let codeUsers = $state<{ id: number; items: { name?: string | null; email?: string | null; created_at?: string; last_login_at?: string | null }[] } | null>(null);
	// Removals and reports
	let removals = $state<Schemas['RemovalRequest'][]>([]);
	let removal = $state({ kind: 'user_data', email: '', note: '' });
	let reports = $state<Schemas['Report'][]>([]);

	async function run(fn: () => Promise<unknown>, done = '') {
		error = '';
		ok = '';
		try {
			await fn();
			ok = done;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Request failed.';
		}
	}

	const loaders: Record<Tab, () => Promise<unknown>> = {
		crawl: async () => (crawl = await api.get('/v1/admin/crawl')),
		sources: async () => (sources = (await api.get<{ items?: Schemas['CareersSource'][] }>('/v1/admin/sources/failing')).items ?? []),
		company: async () => {},
		rules: async () => {
			const [r, u] = await Promise.all([
				api.get<{ categories?: string[]; items?: Schemas['CategoryRule'][] }>('/v1/admin/category-rules'),
				api.get<{ items?: { title?: string; jobs?: number }[] }>('/v1/admin/unclassified')
			]);
			rules = r.items ?? [];
			ruleCats = r.categories ?? [];
			unclassified = u.items ?? [];
		},
		codes: async () => (codes = (await api.get<{ items?: Schemas['AccessCode'][] }>('/v1/admin/codes')).items ?? []),
		removals: async () => (removals = (await api.get<{ items?: Schemas['RemovalRequest'][] }>('/v1/admin/removal-requests')).items ?? []),
		reports: async () => (reports = (await api.get<{ items?: Schemas['Report'][] }>('/v1/admin/reports')).items ?? [])
	};

	$effect(() => {
		if (account.me?.is_admin) run(loaders[tab]);
	});

	function edit(s: Schemas['CareersSource']) {
		editing = {
			id: s.id ?? 0, careers_url: s.careers_url ?? '', ats_type: s.ats_type ?? '', ats_slug: s.ats_slug ?? '',
			selectors: s.selectors ? JSON.stringify(s.selectors, null, 2) : '', status: s.status ?? 'review'
		};
	}

	function saveSource() {
		const e = editing;
		if (!e) return;
		run(async () => {
			let selectors: unknown = null;
			if (e.selectors.trim()) {
				try {
					selectors = JSON.parse(e.selectors);
				} catch {
					throw new ApiError(400, 'bad_request', 'Selectors must be JSON.');
				}
			}
			await api.put(`/v1/admin/sources/${e.id}`, {
				careers_url: e.careers_url, ats_type: e.ats_type || null, ats_slug: e.ats_slug || null, selectors, status: e.status
			});
			editing = null;
			await loaders.sources();
		}, 'Saved. The next crawl uses it.');
	}

	function addCompany() {
		run(async () => {
			const body = Object.fromEntries(Object.entries(co).filter(([, v]) => v !== ''));
			await api.post('/v1/admin/companies', body);
			co = { name: '', website: '', careers_url: '', city: '', state: '', type: 'product', ats_type: '', ats_slug: '' };
		}, 'Company added. It is crawled tonight. Add it to seeds/companies.csv too so a rebuild keeps it.');
	}

	function addRule() {
		run(async () => {
			await api.put('/v1/admin/category-rules', { ...rule, keyword: rule.keyword.trim() });
			rule.keyword = '';
			await loaders.rules();
		}, 'Rule saved. Applies at the next nightly classify.');
	}

	function deleteRule(id: number) {
		run(async () => {
			await api.del(`/v1/admin/category-rules/${id}`);
			await loaders.rules();
		});
	}

	function createCode() {
		run(async () => {
			created = await api.post('/v1/admin/codes', {
				label: newCode.label.trim(), max_uses: newCode.max_uses, expires_at: newCode.expires ? new Date(newCode.expires).toISOString() : null
			});
			newCode = { label: '', max_uses: 1, expires: '' };
			await loaders.codes();
		});
	}

	function setRevoked(id: number, revoked: boolean) {
		if (revoked && !confirm('Revoke this code? Everyone who joined with it loses access within a minute.')) return;
		run(async () => {
			await api.put(`/v1/admin/codes/${id}/revoked`, { revoked });
			await loaders.codes();
		});
	}

	function showUsers(id: number) {
		run(async () => {
			const r = await api.get<{ items?: NonNullable<typeof codeUsers>['items'] }>(`/v1/admin/codes/${id}/users`);
			codeUsers = { id, items: r.items ?? [] };
		});
	}

	function logRemoval() {
		run(async () => {
			await api.post('/v1/admin/removal-requests', { ...removal, email: removal.email.trim() });
			removal = { kind: 'user_data', email: '', note: '' };
			await loaders.removals();
		}, 'Logged.');
	}

	function removalDone(id: number) {
		run(async () => {
			await api.post(`/v1/admin/removal-requests/${id}/done`, {});
			await loaders.removals();
		});
	}

	function reportDone(id: number) {
		run(async () => {
			await api.post(`/v1/admin/reports/${id}/done`);
			await loaders.reports();
		});
	}
</script>

<svelte:head><title>Admin · CareerGlobe</title></svelte:head>

<main class="page wide">
	{#if !account.me?.is_admin}
		<Banner kind="err">This page is for admins.</Banner>
	{:else}
		<h1 class="t-title">Admin</h1>
		<div class="segs tabs" role="tablist">
			{#each TABS as t (t.key)}
				<button class="seg {tab === t.key ? 'on' : ''}" role="tab" aria-selected={tab === t.key} onclick={() => { tab = t.key; ok = ''; error = ''; }}>{t.label}</button>
			{/each}
		</div>
		{#if error}<Banner kind="err">{error}</Banner>{/if}
		{#if ok}<Banner kind="ok">{ok}</Banner>{/if}

		{#if tab === 'crawl' && crawl}
			<div class="stats">
				<div class="card stat"><span class="num {crawl.health === 'ok' ? 't-accent' : ''}" style={crawl.health === 'ok' ? '' : 'color: var(--rejected)'}>{crawl.health}</span><span class="t-caption t-3">Nightly crawl</span></div>
				<div class="card stat"><span class="num" style={(crawl.failed_code_guesses_last_hour ?? 0) > 20 ? 'color: var(--rejected)' : ''}>{crawl.failed_code_guesses_last_hour ?? 0}</span><span class="t-caption t-3">Failed code tries, last hour</span></div>
			</div>
			<div class="table-wrap">
				<table>
					<thead><tr><th>Started</th><th>Took</th><th>OK</th><th>Failed</th><th>New jobs</th><th>Closed</th></tr></thead>
					<tbody>
						{#each crawl.runs ?? [] as r (r.id)}
							<tr>
								<td>{shortDate(r.started_at)} {new Date(r.started_at ?? '').toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit' })}</td>
								<td>{r.finished_at ? `${Math.round((+new Date(r.finished_at) - +new Date(r.started_at ?? '')) / 60000)} min` : 'running'}</td>
								<td>{r.companies_ok}</td><td>{r.companies_failed}</td><td>{r.jobs_new}</td><td>{r.jobs_closed}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else if tab === 'sources'}
			{#each sources as s (s.id)}
				<div class="card col-card">
					<div style="display: flex; justify-content: space-between; gap: 8px; flex-wrap: wrap">
						<b>{s.company}</b><span class="pill p-rejected">{s.status} · {s.fail_count} fails</span>
					</div>
					<a class="t-caption t-2" href={s.careers_url} target="_blank" rel="noopener noreferrer">{s.careers_url}</a>
					<p class="t-caption t-3">{s.ats_type ?? 'custom'}{s.ats_slug ? ` / ${s.ats_slug}` : ''} · last OK {ago(s.last_ok_at) || 'never'}</p>
					{#if s.last_error}<code class="err-text">{s.last_error}</code>{/if}
					{#if editing && editing.id === s.id}
						<div class="field"><label for="u{s.id}">Careers URL</label><input id="u{s.id}" class="input" bind:value={editing.careers_url} /></div>
						<div class="grid2">
							<div class="field"><label for="a{s.id}">ATS</label><input id="a{s.id}" class="input" bind:value={editing.ats_type} /></div>
							<div class="field"><label for="sl{s.id}">Slug</label><input id="sl{s.id}" class="input" bind:value={editing.ats_slug} /></div>
						</div>
						<div class="field"><label for="se{s.id}">Selectors (JSON, for the html adapter)</label><textarea id="se{s.id}" class="input mono" rows="4" bind:value={editing.selectors}></textarea></div>
						<div class="field"><label for="st{s.id}">Status</label>
							<select id="st{s.id}" class="input" bind:value={editing.status}>{#each SOURCE_STATUSES as st (st)}<option value={st}>{st}</option>{/each}</select>
						</div>
						<div style="display: flex; gap: 10px"><button class="btn btn-primary" onclick={saveSource}>Save</button><button class="btn btn-text" onclick={() => (editing = null)}>Cancel</button></div>
					{:else}
						<button class="btn btn-secondary" style="align-self: flex-start" onclick={() => edit(s)}><Icon name="pen" size={16} />Fix</button>
					{/if}
				</div>
			{:else}
				<p class="t-body t-2">No failing companies.</p>
			{/each}
		{:else if tab === 'company'}
			<form class="stack" onsubmit={(e) => { e.preventDefault(); addCompany(); }}>
				<div class="field"><label for="cn">Name</label><input id="cn" class="input" required bind:value={co.name} /></div>
				<div class="field"><label for="cw">Website</label><input id="cw" class="input" type="url" bind:value={co.website} /></div>
				<div class="field"><label for="cc">Careers URL (must be the company's own page or ATS board)</label><input id="cc" class="input" type="url" required bind:value={co.careers_url} /></div>
				<div class="grid2">
					<div class="field"><label for="ci">City</label><input id="ci" class="input" bind:value={co.city} /></div>
					<div class="field"><label for="cs">State</label><input id="cs" class="input" bind:value={co.state} /></div>
				</div>
				<div class="grid2">
					<div class="field"><label for="ct">Type</label><select id="ct" class="input" bind:value={co.type}><option>product</option><option>service</option><option>both</option></select></div>
					<div class="field"><label for="ca">ATS (blank = auto)</label><input id="ca" class="input" bind:value={co.ats_type} /></div>
				</div>
				<div class="field"><label for="cl">ATS slug</label><input id="cl" class="input" bind:value={co.ats_slug} /></div>
				<button class="btn btn-primary" type="submit"><Icon name="plus" size={18} />Add company</button>
			</form>
		{:else if tab === 'rules'}
			<form class="grid-rule" onsubmit={(e) => { e.preventDefault(); addRule(); }}>
				<input class="input" placeholder="Title keyword" maxlength="60" required bind:value={rule.keyword} aria-label="Keyword" />
				<select class="input" bind:value={rule.category} aria-label="Category">{#each ruleCats as c (c)}<option value={c}>{c}</option>{/each}</select>
				<input class="input" type="number" min="0" max="100" bind:value={rule.priority} aria-label="Priority" />
				<button class="btn btn-primary" type="submit">Add</button>
			</form>
			<div class="table-wrap">
				<table>
					<thead><tr><th>Keyword</th><th>Category</th><th>Priority</th><th></th></tr></thead>
					<tbody>
						{#each rules as r (r.id)}
							<tr><td>{r.keyword}</td><td>{r.category}</td><td>{r.priority}</td>
								<td><button class="icon-btn" aria-label="Delete rule {r.keyword}" onclick={() => deleteRule(r.id ?? 0)}><Icon name="trash" size={16} /></button></td></tr>
						{/each}
					</tbody>
				</table>
			</div>
			<h2 class="t-heading">Filed as Other</h2>
			{#each unclassified as u (u.title)}
				<div class="list-row"><span style="flex: 1">{u.title}</span><span class="t-caption t-3">{u.jobs} jobs</span>
					<button class="btn btn-text" onclick={() => (rule.keyword = (u.title ?? '').toLowerCase())}>Use</button></div>
			{:else}
				<p class="t-body t-2">Nothing unclassified.</p>
			{/each}
		{:else if tab === 'codes'}
			<form class="grid-rule" onsubmit={(e) => { e.preventDefault(); createCode(); }}>
				<input class="input" placeholder="Label, e.g. Friends batch 1" maxlength="100" required bind:value={newCode.label} aria-label="Label" />
				<input class="input" type="number" min="1" max="1000" bind:value={newCode.max_uses} aria-label="Max uses" />
				<input class="input" type="date" bind:value={newCode.expires} aria-label="Expires" />
				<button class="btn btn-primary" type="submit">Create</button>
			</form>
			{#if created?.code}
				<div class="card col-card" style="border-color: var(--accent)">
					<p class="t-caption t-2">NEW CODE — shown once, copy it now</p>
					<code class="code">{created.code}</code>
					<div style="display: flex; gap: 10px">
						<button class="btn btn-secondary" onclick={() => navigator.clipboard?.writeText(created?.code ?? '')}>Copy</button>
						<button class="btn btn-text" onclick={() => (created = null)}>Done</button>
					</div>
				</div>
			{/if}
			<div class="table-wrap">
				<table>
					<thead><tr><th>Label</th><th>Used</th><th>Expires</th><th>Status</th><th></th></tr></thead>
					<tbody>
						{#each codes as c (c.id)}
							<tr>
								<td>{c.label}</td><td>{c.used_count} / {c.max_uses}</td><td>{c.expires_at ? shortDate(c.expires_at) : '—'}</td>
								<td><span class="pill {c.revoked ? 'p-rejected' : 'p-offer'}">{c.revoked ? 'revoked' : 'active'}</span></td>
								<td style="white-space: nowrap">
									<button class="btn btn-text" onclick={() => showUsers(c.id ?? 0)}><Icon name="users" size={16} />{c.users}</button>
									<button class="btn btn-text" onclick={() => setRevoked(c.id ?? 0, !c.revoked)}>{c.revoked ? 'Restore' : 'Revoke'}</button>
								</td>
							</tr>
							{#if codeUsers && codeUsers.id === c.id}
								<tr><td colspan="5">
									{#each codeUsers.items as u (u.email)}
										<p class="t-caption t-2">{u.name ?? '—'} · {u.email} · joined {shortDate(u.created_at)} · last seen {ago(u.last_login_at) || 'never'}</p>
									{:else}<p class="t-caption t-3">Nobody yet.</p>{/each}
								</td></tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{:else if tab === 'removals'}
			<form class="grid-rule" onsubmit={(e) => { e.preventDefault(); logRemoval(); }}>
				<select class="input" bind:value={removal.kind} aria-label="Kind"><option value="user_data">User data</option><option value="company">Company</option></select>
				<input class="input" type="email" placeholder="From email" required bind:value={removal.email} aria-label="Email" />
				<input class="input" placeholder="Note" bind:value={removal.note} aria-label="Note" />
				<button class="btn btn-primary" type="submit">Log</button>
			</form>
			{#each removals as r (r.id)}
				<div class="list-row">
					<div style="flex: 1">
						<b>{r.kind === 'company' ? 'Remove company' : 'Delete user data'}</b> · {r.email}
						<p class="t-caption t-3">Received {shortDate(r.received_at)}{r.note ? ` · ${r.note}` : ''}</p>
					</div>
					{#if r.done_at}<span class="pill p-offer">done {shortDate(r.done_at)}</span>
					{:else}<button class="btn btn-secondary" onclick={() => removalDone(r.id ?? 0)}>Mark done</button>{/if}
				</div>
			{:else}
				<p class="t-body t-2">No requests.</p>
			{/each}
		{:else if tab === 'reports'}
			{#each reports as r (r.id)}
				<div class="list-row">
					<div style="flex: 1; min-width: 0">
						<b>{r.job_title ?? r.company_name ?? 'General'}</b>{#if r.job_title && r.company_name}<span class="t-3"> · {r.company_name}</span>{/if}
						<p class="t-body t-2">{r.message}</p>
						<p class="t-caption t-3">{ago(r.created_at)}
							{#if r.job_id} · <a href="/jobs/{r.job_id}">job</a>{/if}
							{#if r.company_id} · <a href="/companies/{r.company_id}">company</a>{/if}</p>
					</div>
					{#if r.done_at}<span class="pill p-offer">done</span>
					{:else}<button class="btn btn-secondary" onclick={() => reportDone(r.id ?? 0)}>Done</button>{/if}
				</div>
			{:else}
				<p class="t-body t-2">No reports.</p>
			{/each}
		{/if}
	{/if}
</main>

<style>
	.wide { max-width: 1100px; }
	.tabs { overflow-x: auto; }
	.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; }
	.stat { flex-direction: column; align-items: flex-start; gap: 2px; }
	.col-card { flex-direction: column; align-items: stretch; gap: 8px; }
	.grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
	.grid-rule { display: grid; grid-template-columns: 2fr 2fr 1fr auto; gap: 8px; }
	.table-wrap { overflow-x: auto; }
	table { width: 100%; border-collapse: collapse; font: var(--font-body); }
	th { text-align: left; font: var(--font-caption); color: var(--text-3); padding: 8px; border-bottom: 1px solid var(--line); }
	td { padding: 8px; border-bottom: 1px solid var(--line); vertical-align: middle; }
	.mono, .err-text, .code { font-family: ui-monospace, Menlo, monospace; font-size: 13px; }
	.err-text { color: var(--rejected); white-space: pre-wrap; word-break: break-word; }
	.code { font-size: 22px; letter-spacing: 0.1em; font-weight: 700; }
	@media (max-width: 700px) { .grid-rule { grid-template-columns: 1fr 1fr; } }
</style>
