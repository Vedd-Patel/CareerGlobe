import { describe, expect, it } from 'vitest';
import { activeCount, clusterSize, emptyFilters, filtersToQuery, jobTypeQuery, pinKind } from './globe';
import { salary, scoreBand, updatedLabel, initials, logoClass } from './format';

describe('globe helpers', () => {
	it('sizes clusters and colours pins', () => {
		expect([clusterSize(3), clusterSize(42), clusterSize(310)]).toEqual(['c-s', 'c-m', 'c-l']);
		expect(pinKind({ matches: 2, open_jobs: 9 })).toBe('match');
		expect(pinKind({ matches: 0, open_jobs: 9 })).toBe('hiring');
		expect(pinKind({})).toBe('idle');
	});
	it('counts active filters', () => {
		expect(activeCount(emptyFilters())).toBe(0);
		expect(activeCount({ ...emptyFilters(), remote: true, category: 'Design' })).toBe(2);
		expect(activeCount({ ...emptyFilters(), employment: ['intern'], exp: '1-3', minSalaryLpa: 10 })).toBe(3);
	});
	it('turns type, experience and salary into query parameters', () => {
		expect(jobTypeQuery(emptyFilters())).toEqual({ employment: '', exp_min: '', exp_max: '', min_salary: '' });
		expect(jobTypeQuery({ employment: ['intern', 'contract'], exp: 'fresher', minSalaryLpa: 12 })).toEqual({
			employment: 'intern,contract', exp_min: '0', exp_max: '1', min_salary: '1200000'
		});
		expect(jobTypeQuery({ employment: [], exp: '8+', minSalaryLpa: 0 }).exp_max).toBe('60');
		expect(filtersToQuery({ ...emptyFilters(), exp: '3-5' })).toMatchObject({ exp_min: '3', exp_max: '5', remote: false });
	});
});

describe('format', () => {
	it('bands scores like the design system', () => {
		expect([scoreBand(92), scoreBand(68), scoreBand(41), scoreBand(null)]).toEqual(['band-hi', 'band-mid', 'band-lo', 'band-lo']);
	});
	it('shows salary only when stated', () => {
		expect(salary(1800000, 2400000)).toBe('₹18–24 LPA');
		expect(salary(null, null)).toBe('');
		expect(salary(2550000, null)).toBe('From ₹25.5 LPA');
	});
	it('labels freshness', () => {
		const now = new Date('2026-10-26T10:00:00+05:30');
		expect(updatedLabel('2026-10-26T06:00:00+05:30', now)).toMatch(/^Updated today/);
		expect(updatedLabel('2026-10-22T06:00:00+05:30', now)).toMatch(/^Updated 22 Oct/);
	});
	it('makes stable logo initials and colours', () => {
		expect(initials('Tata Motors')).toBe('TM');
		expect(logoClass('Groww')).toBe(logoClass('Groww'));
	});
});

describe('dark map colours', () => {
	it('lifts near-black into a visible warm grey and keeps hue for tinted colours', async () => {
		const { parseColor, liftColor, liftStyle } = await import('./map/style');
		const lifted = parseColor(liftColor('#000000'))!;
		expect(lifted[2]).toBeGreaterThan(0.15); // no longer black
		expect(lifted[0]).toBeGreaterThan(20); // warm, brownish
		expect(lifted[0]).toBeLessThan(45);
		const water = parseColor(liftColor('hsl(210, 60%, 12%)'))!;
		expect(Math.round(water[0])).toBe(210); // water stays blue
		expect(water[2]).toBeGreaterThan(0.2);
		expect(liftColor('not a colour')).toBe('not a colour');
		const style = liftStyle({
			version: 8, sources: {},
			layers: [{ id: 'bg', type: 'background', paint: { 'background-color': '#0a0a0a' } },
				{ id: 'label', type: 'symbol', paint: { 'text-color': '#ffffff' } }]
		} as never);
		expect((style.layers[0] as unknown as { paint: Record<string, string> }).paint['background-color']).toMatch(/^hsla\(/);
		expect((style.layers[1] as unknown as { paint: Record<string, string> }).paint['text-color']).toBe('#ffffff'); // labels untouched
	});
});
