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

<nav class="app-nav" aria-label="Main">
	<a class="brand" href="/globe" aria-label="CareerGlobe home"><span></span></a>
	{#each items as it (it.href)}
		<a class={active(it.href) ? 'tab on' : 'tab'} href={it.href} aria-current={active(it.href) ? 'page' : undefined}>
			<span style="position: relative; display: inline-flex">
				<Icon name={it.icon} size={24} />
				{#if badges[it.href]}<span class="dot" aria-label="{badges[it.href]} new"></span>{/if}
			</span>
			{it.label}
		</a>
	{/each}
</nav>

<style>
	.app-nav {
		position: fixed; left: 0; right: 0; bottom: 0; z-index: 20; height: 76px; padding: 8px 6px calc(10px + env(safe-area-inset-bottom));
		background: var(--surface); border-top: 1px solid var(--line); display: flex; justify-content: space-around; align-items: center;
	}
	.brand { display: none; }
	.dot { position: absolute; top: -2px; right: -4px; width: 9px; height: 9px; border-radius: 50%; background: var(--accent); border: 2px solid var(--surface); }
	@media (min-width: 900px) {
		.app-nav {
			top: 0; right: auto; width: 88px; height: auto; flex-direction: column; justify-content: flex-start; gap: 8px; padding: 20px 0;
			border-top: 0; border-right: 1px solid var(--line);
		}
		.app-nav :global(.tab) { padding: 10px 0; width: 72px; border-radius: 14px; }
		.app-nav :global(.tab.on) { background: var(--accent-soft); }
		.brand { display: block; margin-bottom: 16px; }
		.brand span { display: block; width: 34px; height: 34px; border-radius: 50%; background: radial-gradient(circle at 35% 35%, #3dd6c6, #0e2150); }
	}
</style>
