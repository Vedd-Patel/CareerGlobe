<script lang="ts">
	// S08 Upload resume: consent required, upload disabled until ticked.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { request, ApiError, type Schemas } from '$lib/api/client';
	import Banner from '$lib/components/Banner.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import { draft } from '$lib/draft.svelte';

	const CONSENT = 'I agree to share my resume and profile details with CareerGlobe so it can find more relevant, better-matched jobs for me. I have read the Privacy Policy.';
	let file = $state<File | null>(null);
	let consent = $state(false);
	let phase = $state<'idle' | 'uploading' | 'error'>('idle');
	let error = $state('');
	let dragging = $state(false);

	function pick(f: File | undefined | null) {
		error = '';
		if (!f) return;
		if (f.size > 5 * 1024 * 1024) return (error = 'The file must be a PDF or DOCX up to 5 MB.');
		if (!/\.(pdf|docx)$/i.test(f.name)) return (error = 'Only PDF and DOCX files are accepted.');
		file = f;
	}

	async function upload() {
		if (!file || !consent) return;
		phase = 'uploading';
		const form = new FormData();
		form.set('file', file);
		form.set('consent', 'true');
		try {
			draft.value = await request<Schemas['ResumeDraft']>('POST', '/v1/resume', { form });
			goto(page.url.searchParams.get('from') === 'profile' ? '/onboarding/review?from=profile' : '/onboarding/review');
		} catch (e) {
			phase = 'error';
			error = e instanceof ApiError ? e.message : "Couldn't read this file — try a text PDF.";
		}
	}

	function skip() {
		try {
			localStorage.setItem('cg.onboardingSkipped', '1');
		} catch {
			/* ignore */
		}
		goto('/onboarding/roles');
	}
</script>

<svelte:head><title>Upload your resume · CareerGlobe</title></svelte:head>

<h1 class="t-title">Upload your resume</h1>
<p class="t-body t-2">We read your skills, titles and experience to rank jobs for you. You'll check everything on the next screen.</p>

{#if phase === 'uploading'}
	<div class="dropzone" aria-live="polite"><Icon name="file" size={30} /><p class="t-heading">Reading your resume…</p><p class="t-caption t-3">{file?.name}</p></div>
{:else}
	<label class="dropzone {dragging ? 'drag' : ''}"
		ondragover={(e) => { e.preventDefault(); dragging = true; }}
		ondragleave={() => (dragging = false)}
		ondrop={(e) => { e.preventDefault(); dragging = false; pick(e.dataTransfer?.files[0]); }}>
		<span style="color: var(--accent)"><Icon name="upload" size={30} /></span>
		<span class="t-heading">{file ? file.name : 'Drop your resume here or choose a file'}</span>
		<span class="t-caption t-3">PDF or DOCX · up to 5 MB</span>
		<input type="file" accept=".pdf,.docx,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document" class="sr-only"
			onchange={(e) => pick((e.currentTarget as HTMLInputElement).files?.[0])} />
	</label>
{/if}
{#if error}<Banner kind="err">{error}</Banner>{/if}
<label class="check"><input type="checkbox" bind:checked={consent} />{CONSENT}</label>
<button class="btn btn-primary btn-block" disabled={!file || !consent || phase === 'uploading'} onclick={upload}>Upload</button>
<button class="btn btn-text" onclick={skip}>Skip for now — matching stays off</button>

<style>
	.dropzone { cursor: pointer; }
	.dropzone.drag { border-color: var(--accent); background: var(--accent-soft); }
</style>
