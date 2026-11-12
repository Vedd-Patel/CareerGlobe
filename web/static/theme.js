// Applies the saved theme before the first paint. Light unless the member picked
// dark or system in Settings (S29). A file, not an inline script, so the
// Content-Security-Policy can forbid inline scripts.
try {
	var t = localStorage.getItem('cg.theme');
	if (t !== 'system') document.documentElement.setAttribute('data-theme', t === 'dark' ? 'dark' : 'light');
} catch (e) {}
