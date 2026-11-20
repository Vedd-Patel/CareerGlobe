<script lang="ts">
	import { initials, logoClass } from '$lib/format';
	let { name, url = null, size = 'md' }: { name: string; url?: string | null; size?: 'xs' | 'sm' | 'md' | 'lg' } = $props();
	// A logo that fails to load (moved, blocked, offline) falls back to the lettered badge.
	let broken = $state<string | null>(null);
</script>

{#if url && broken !== url}
	<img class="logo {size === 'md' ? '' : size}" src={url} alt="" loading="lazy" referrerpolicy="no-referrer"
		onerror={() => (broken = url)} style="object-fit: contain; background: #fff; padding: {size === 'xs' ? 2 : 4}px" />
{:else}
	<span class="logo {size === 'md' ? '' : size} {logoClass(name)}" aria-hidden="true">{initials(name)}</span>
{/if}
