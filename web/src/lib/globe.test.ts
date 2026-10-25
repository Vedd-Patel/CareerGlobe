import { describe, expect, it } from 'vitest';
import { activeCount, clusterSize, emptyFilters, pinKind } from './globe';
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
