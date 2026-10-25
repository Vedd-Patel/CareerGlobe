// Small display helpers shared by the screens.
const rtf = new Intl.RelativeTimeFormat('en-IN', { numeric: 'auto' });

export function ago(iso: string | null | undefined, now = Date.now()): string {
	if (!iso) return '';
	const s = (new Date(iso).getTime() - now) / 1000;
	const abs = Math.abs(s);
	if (abs < 3600) return rtf.format(Math.round(s / 60), 'minute');
	if (abs < 86400) return rtf.format(Math.round(s / 3600), 'hour');
	if (abs < 86400 * 30) return rtf.format(Math.round(s / 86400), 'day');
	return rtf.format(Math.round(s / (86400 * 30)), 'month');
}

export function shortDate(iso: string | null | undefined): string {
	if (!iso) return '';
	return new Date(iso).toLocaleDateString('en-IN', { day: 'numeric', month: 'short' });
}

export function timeOfDay(iso: string | null | undefined): string {
	if (!iso) return '';
	return new Date(iso).toLocaleTimeString('en-IN', { hour: 'numeric', minute: '2-digit' });
}

/** "Updated today 6:00 AM" / "Updated 22 Oct". */
export function updatedLabel(iso: string | null | undefined, now = new Date()): string {
	if (!iso) return '';
	const d = new Date(iso);
	const sameDay = d.toDateString() === now.toDateString();
	return sameDay ? `Updated today ${timeOfDay(iso)}` : `Updated ${shortDate(iso)}`;
}

export function scoreBand(score: number | null | undefined): 'band-hi' | 'band-mid' | 'band-lo' {
	if (score == null) return 'band-lo';
	return score >= 80 ? 'band-hi' : score >= 60 ? 'band-mid' : 'band-lo';
}

/** "₹18–24 LPA" from annual INR, or "" when the posting states no salary. */
export function salary(min: number | null | undefined, max: number | null | undefined): string {
	const lpa = (v: number) => (v / 100000).toFixed(v % 100000 === 0 ? 0 : 1);
	if (min && max) return `₹${lpa(min)}–${lpa(max)} LPA`;
	if (min) return `From ₹${lpa(min)} LPA`;
	if (max) return `Up to ₹${lpa(max)} LPA`;
	return '';
}

export function workMode(m: string | null | undefined): string {
	return m === 'remote' ? 'Remote' : m === 'hybrid' ? 'Hybrid' : m === 'onsite' ? 'On-site' : '';
}

export function initials(name: string | null | undefined): string {
	return (name ?? '?').replace(/[^A-Za-z0-9 ]/g, '').split(' ').filter(Boolean).slice(0, 2).map((w) => w[0]).join('').toUpperCase() || '?';
}

/** A stable logo colour per company. */
export function logoClass(name: string | null | undefined): string {
	let h = 0;
	for (const c of name ?? '') h = (h * 31 + c.charCodeAt(0)) >>> 0;
	return 'l' + ((h % 8) + 1);
}

export function deviceId(): string {
	try {
		let id = localStorage.getItem('cg.device');
		if (!id) {
			id = crypto.randomUUID();
			localStorage.setItem('cg.device', id);
		}
		return id;
	} catch {
		return 'unknown';
	}
}
