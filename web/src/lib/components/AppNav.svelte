<script lang="ts">
	import { page } from '$app/state';
	import Icon, { type IconName } from './Icon.svelte';
	let { badges = {} }: { badges?: Record<string, number> } = $props();
	const items: { href: string; label: string; icon: IconName }[] = [
		{ href: '/globe', label: 'Globe', icon: 'globe' },
		{ href: '/matches', label: 'Matches', icon: 'sparkles' },
		{ href: '/tracker', label: 'Tracker', icon: 'kanban' },
		{ href: '/saved', label: 'Saved', icon: 'bookmark' },
		{ href: '/profile', label: 'Profile', icon: 'user' }
	];
	const active = (href: string) => page.url.pathname === href || page.url.pathname.startsWith(href + '/');
</script>

<!-- Bottom tabs on phones, an icon rail on small desktops, a labelled sidebar on wide screens. -->
<nav class="app-nav" aria-label="Main">
	<a class="brand" href="/globe" aria-label="CareerGlobe home"><span class="brand-mark"></span><span class="brand-name">CareerGlobe</span></a>
	{#each items as it (it.href)}
		<a class={active(it.href) ? 'tab on' : 'tab'} href={it.href} aria-current={active(it.href) ? 'page' : undefined}>
			<span style="position: relative; display: inline-flex">
				<Icon name={it.icon} size={24} />
				{#if badges[it.href]}<span class="dot" aria-label="{badges[it.href]} new"></span>{/if}
			</span>
			<span class="label">{it.label}</span>
		</a>
	{/each}
	<div class="foot">
		<a class={active('/settings') ? 'tab on' : 'tab'} href="/settings" aria-current={active('/settings') ? 'page' : undefined}>
			<Icon name="settings" size={22} /><span class="label">Settings</span>
		</a>
		<p class="t-caption t-3 note">Jobs from each company's own careers page, checked nightly.</p>
	</div>
</nav>

<style>
	.app-nav {
		position: fixed; left: 0; right: 0; bottom: 0; z-index: 20; height: 76px; padding: 8px 6px calc(10px + env(safe-area-inset-bottom));
		background: var(--surface); border-top: 1px solid var(--line); display: flex; justify-content: space-around; align-items: center;
	}
	.brand, .foot { display: none; }
	.tab:hover { color: var(--text); }
	.tab.on:hover { color: var(--accent); }
	.dot { position: absolute; top: -2px; right: -4px; width: 9px; height: 9px; border-radius: 50%; background: var(--pin-hiring); border: 2px solid var(--surface); }
	@media (min-width: 900px) {
		.app-nav {
			top: 0; right: auto; width: 88px; height: auto; flex-direction: column; justify-content: flex-start; gap: 6px; padding: 20px 0;
			border-top: 0; border-right: 1px solid var(--line);
		}
		.app-nav :global(.tab) { padding: 10px 0; width: 72px; border-radius: var(--r-md); }
		.app-nav :global(.tab:hover) { background: var(--raised); }
		.app-nav :global(.tab.on) { background: var(--accent-soft); }
		.brand { display: flex; align-items: center; gap: 10px; margin-bottom: 18px; color: var(--text); text-decoration: none; }
		.brand-mark { width: 34px; height: 34px; }
		.brand-name { display: none; font: 800 20px/24px var(--font); letter-spacing: -0.02em; }
		.foot { display: flex; flex-direction: column; align-items: center; margin-top: auto; }
		.note { display: none; }
	}
	@media (min-width: 1200px) {
		.app-nav { width: 244px; align-items: stretch; padding: 24px 16px; gap: 4px; }
		.brand { padding: 0 12px; }
		.brand-name { display: inline; }
		.app-nav :global(.tab) {
			width: auto; flex-direction: row; gap: 14px; padding: 0 14px; height: 48px; border-radius: var(--r-pill);
			font: 650 16px/20px var(--font); color: var(--text);
		}
		.app-nav :global(.tab.on) { color: var(--accent-strong); font-weight: 800; }
		.foot { align-items: stretch; gap: 12px; }
		.note { display: block; padding: 0 14px; }
	}
</style>
