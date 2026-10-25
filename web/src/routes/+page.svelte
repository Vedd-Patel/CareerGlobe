<script lang="ts">
	// S01 Landing (web only): public, aggregates only.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { request } from '$lib/api/client';
	import { session } from '$lib/auth.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import { timeOfDay } from '$lib/format';

	let stats = $state<{ companies: number; open_jobs: number; last_crawl_at: string | null } | null>(null);
	onMount(async () => {
		try {
			stats = await request('GET', '/v1/public/stats', { auth: false });
		} catch {
			stats = null;
		}
	});
	$effect(() => {
		if (session.ready && session.user) goto('/globe');
	});
</script>

<svelte:head><title>CareerGlobe · Every company in India on one globe</title></svelte:head>

<div data-theme-scope="dark" class="landing">
	<section class="space hero">
		<header>
			<a class="brand" href="/"><span class="dot"></span><span class="t-heading">CareerGlobe</span></a>
			<a class="btn btn-secondary ghost" href="/login">Log in</a>
		</header>
		<div class="hero-body">
			<div class="copy">
				<span class="pill" style="background: rgba(61,214,198,.14); color: #3dd6c6; align-self: flex-start">Private · invite-only</span>
				<h1>Every company in India, on one globe.</h1>
				<p class="lede">Jobs read every night from each company's own careers page. Upload your resume, see the roles that fit you and why, and track every application.</p>
				<div class="cta">
					<a class="btn btn-primary big" href="/access">I have an access code</a>
					<span class="t-caption" style="color: #8e98b3">No public sign-up. Ask a member for a code.</span>
				</div>
				<div class="counters" aria-live="polite">
					<div><p class="num">{stats ? stats.companies.toLocaleString('en-IN') : '—'}</p><p class="t-caption">companies</p></div>
					<div><p class="num">{stats ? stats.open_jobs.toLocaleString('en-IN') : '—'}</p><p class="t-caption">open jobs</p></div>
					<div><p class="num accent">{stats?.last_crawl_at ? timeOfDay(stats.last_crawl_at) : '—'}</p><p class="t-caption">updated this morning</p></div>
				</div>
			</div>
			<div class="art" aria-hidden="true">
				<div class="globe spin" style="width: min(520px, 90%); aspect-ratio: 1; left: 50%; top: 10px; transform: translateX(-50%)"></div>
				<div class="landmass" style="width: 150px; height: 190px; left: calc(50% - 30px); top: 180px; transform: rotate(18deg)"></div>
				<span class="cluster c-l" style="left: calc(50% + 30px); top: 300px">{stats ? Math.min(stats.companies, 999) : ''}</span>
				<span class="pin match" style="left: calc(50% + 70px); top: 360px"></span>
				<span class="pin hiring" style="left: calc(50% + 2px); top: 330px"></span>
				<span class="pin match" style="left: calc(50% + 50px); top: 250px"></span>
				<span class="pin idle" style="left: calc(50% - 40px); top: 300px"></span>
			</div>
		</div>
	</section>
	<section class="features">
		{#each [
			{ icon: 'globe', title: 'A globe of real employers', text: 'Spin to any city and see who is hiring there. Only jobs from each company’s own careers page, checked every night — no aggregators.' },
			{ icon: 'sparkles', title: 'Matches you can read', text: 'Every score comes with its reason: “Matches 7 of 9 skills · missing Kubernetes”. VLSI, design, sales, finance — every field.' },
			{ icon: 'kanban', title: 'Track every application', text: 'Apply on the company’s site, then move it along Saved → Applied → Interview → Offer, with reminders for the next step.' }
		] as f (f.title)}
			<div class="card" style="flex-direction: column; gap: 14px; padding: 28px">
				<div class="ficon"><Icon name={f.icon as 'globe'} size={26} /></div>
				<h2 class="t-title">{f.title}</h2>
				<p class="t-body t-2">{f.text}</p>
			</div>
		{/each}
	</section>
	<footer class="t-caption t-3">
		<span>Jobs are shown as found on each company's careers page and credited to the company.</span>
		<nav><a href="/privacy">Privacy</a><a href="/terms">Terms</a><a href="mailto:privacy@careerglobe.example">Contact</a><a href="mailto:privacy@careerglobe.example?subject=Remove%20my%20company">Remove my company</a></nav>
	</footer>
</div>

<style>
	.landing { background: var(--bg); min-height: 100vh; }
	.hero { min-height: 860px; display: flex; flex-direction: column; color: #e9edf7; }
	header { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 28px clamp(16px, 5vw, 72px); }
	.brand { display: flex; gap: 10px; align-items: center; color: inherit; text-decoration: none; }
	.brand .dot { width: 30px; height: 30px; border-radius: 50%; background: radial-gradient(circle at 35% 35%, #3dd6c6, #0e2150); }
	.ghost { background: transparent; color: #e9edf7; border-color: rgba(255, 255, 255, 0.2); }
	.hero-body { flex: 1; display: flex; flex-wrap: wrap; align-items: center; gap: 48px; padding: 24px clamp(16px, 5vw, 72px) 72px; max-width: 1360px; width: 100%; margin: 0 auto; }
	.copy { flex: 1 1 420px; display: flex; flex-direction: column; gap: 24px; min-width: 0; }
	h1 { font: 800 clamp(38px, 5vw, 64px)/1.05 var(--font); letter-spacing: -0.03em; }
	.lede { font: 500 18px/28px var(--font); color: #a6b0c9; max-width: 560px; }
	.cta { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
	.big { min-height: 56px; padding: 0 28px; font-size: 16px; }
	.counters { display: flex; flex-wrap: wrap; gap: 36px; margin-top: 12px; color: #8e98b3; }
	.counters .num { font-size: 34px; color: #e9edf7; }
	.counters .accent { color: #3dd6c6; }
	.art { flex: 1 1 380px; position: relative; height: 540px; min-width: 0; }
	.spin { animation: drift 120s linear infinite; }
	@keyframes drift { from { filter: hue-rotate(0deg); } to { filter: hue-rotate(10deg); } }
	.features { max-width: 1240px; margin: 0 auto; padding: 88px clamp(16px, 5vw, 72px); display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 24px; }
	.ficon { width: 52px; height: 52px; border-radius: 16px; background: var(--accent-soft); color: var(--accent); display: flex; align-items: center; justify-content: center; }
	footer { border-top: 1px solid var(--line); padding: 32px clamp(16px, 5vw, 72px); display: flex; flex-wrap: wrap; gap: 24px; justify-content: space-between; }
	footer nav { display: flex; flex-wrap: wrap; gap: 24px; }
	@media (max-width: 700px) { .art { height: 360px; } .hero { min-height: 0; } }
</style>
