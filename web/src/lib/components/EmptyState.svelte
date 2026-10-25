<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';
	let { icon, title, tone = 'accent', children, actions }: {
		icon: IconName; title: string; tone?: 'accent' | 'applied' | 'saved'; children?: Snippet; actions?: Snippet;
	} = $props();
	const bg = $derived(tone === 'accent' ? 'var(--accent-soft)' : `var(--${tone}-soft)`);
	const fg = $derived(tone === 'accent' ? 'var(--accent)' : `var(--${tone})`);
</script>

<div style="display: flex; flex-direction: column; align-items: center; text-align: center; gap: 14px; padding: 48px 20px; max-width: 420px; margin: 0 auto">
	<div style="width: 88px; height: 88px; border-radius: 28px; background: {bg}; color: {fg}; display: flex; align-items: center; justify-content: center">
		<Icon name={icon} size={40} />
	</div>
	<h2 class="t-title">{title}</h2>
	{#if children}<p class="t-body t-2">{@render children()}</p>{/if}
	{#if actions}<div style="display: flex; flex-direction: column; gap: 8px; align-items: center">{@render actions()}</div>{/if}
</div>
