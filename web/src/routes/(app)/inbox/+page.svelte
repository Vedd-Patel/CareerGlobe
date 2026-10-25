<script lang="ts">
	// S27 Notifications inbox, newest first.
	import { goto } from '$app/navigation';
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { account } from '$lib/account.svelte';
	import { ago } from '$lib/format';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	type N = Schemas['Notification'];
	const ICONS: Record<string, IconName> = { matches: 'sparkles', saved_company: 'bookmark', reminder: 'calendar', system: 'bell' };

	let items = $state<N[] | null>(null);
	let error = $state('');
	const unread = $derived((items ?? []).filter((n) => !n.read_at).length);

	$effect(() => {
		api.get<{ items?: N[] }>('/v1/notifications').then(
			(r) => (items = r.items ?? []),
			(e) => (error = e instanceof ApiError ? e.message : 'Could not load notifications.')
		);
	});

	function syncBadge() {
		if (account.me) account.me.unread_notifications = unread;
	}

	async function open(n: N) {
		if (!n.read_at) {
			n.read_at = new Date().toISOString();
			syncBadge();
			api.post('/v1/notifications/read', { id: n.id }).catch(() => {});
		}
		if (n.link) goto(n.link);
	}

	async function readAll() {
		await api.post('/v1/notifications/read', {});
		const now = new Date().toISOString();
		for (const n of items ?? []) n.read_at ??= now;
		syncBadge();
	}
</script>

<svelte:head><title>Inbox · CareerGlobe</title></svelte:head>

<main class="page">
	<div class="page-head">
		<h1 class="t-title">Inbox</h1>
		{#if unread}<button class="btn btn-text" onclick={readAll}><Icon name="check" size={16} />Mark all read</button>{/if}
	</div>
	{#if error}
		<Banner kind="err">{error}</Banner>
	{:else if items === null}
		<SkeletonCards count={4} />
	{:else}
		{#each items as n (n.id)}
			<button class="list-row row {n.read_at ? '' : 'unread'}" onclick={() => open(n)}>
				<span class="ic"><Icon name={ICONS[n.kind ?? 'system'] ?? 'bell'} size={20} /></span>
				<span style="flex: 1; min-width: 0; text-align: left">
					<b class="t-heading" style="font-size: 15px">{n.title}</b>
					<span class="t-caption t-2" style="display: block">{n.body}</span>
				</span>
				<span class="t-caption t-3">{ago(n.created_at)}</span>
			</button>
		{:else}
			<EmptyState icon="inbox" title="No notifications yet">
				<p class="t-body t-2">New matches, saved-company jobs and tracker reminders show up here.</p>
			</EmptyState>
		{/each}
	{/if}
</main>

<style>
	.row { width: 100%; background: none; border-left: 0; border-right: 0; border-top: 0; font: inherit; color: inherit; cursor: pointer; }
	.ic { width: 40px; height: 40px; border-radius: 12px; background: var(--raised); display: flex; align-items: center; justify-content: center; flex: none; color: var(--text-2); }
	.unread .ic { background: var(--accent-soft); color: var(--accent); }
	.unread b::after { content: ''; display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: var(--accent); margin-left: 8px; vertical-align: middle; }
</style>
