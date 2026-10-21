package resume

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/skills"
)

// Draft is what the parser read from a resume. The user always confirms and
// edits it before it is used (S09).
type Draft struct {
	Skills      []string `json:"skills"`
	Titles      []string `json:"titles"`
	YearsExp    *float64 `json:"years_exp"`
	CurrentCity *string  `json:"current_city"`
	Education   []string `json:"education"`
}

var months = map[string]time.Month{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "sept": 9,
	"oct": 10, "nov": 11, "dec": 12,
}

const (
	monthWord = `(jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.?`
	dateTok   = `(?:` + monthWord + `[\s,'’-]*(\d{4}|'\d{2})|(\d{1,2})\s*[/.-]\s*(\d{4})|(\d{4}))`
)

var (
	reRange = regexp.MustCompile(`(?i)` + dateTok + `\s*(?:-|–|—|to|till|until)\s*(?:` + dateTok +
		`|(present|current|now|today|till date|date|ongoing))`)
	reYearsStated = regexp.MustCompile(`(?i)(\d{1,2}(?:\.\d)?)\s*\+?\s*(?:years|yrs)(?:\s+of)?\s+(?:total\s+|overall\s+|professional\s+|work\s+|industry\s+)*experience`)
	reEducation   = regexp.MustCompile(`(?i)\b(b\.?\s?tech|m\.?\s?tech|b\.?e\.?(?:\s|$)|m\.?e\.?(?:\s|$)|b\.?sc|m\.?sc|bca|mca|mba|pgdm|ph\.?d|b\.?com|m\.?com|b\.?a\.?(?:\s|$)|m\.?a\.?(?:\s|$)|diploma|bachelor[’']?s?|master[’']?s?|b\.?des|m\.?des)\b[^\n]{0,80}`)
	reTitleLine   = regexp.MustCompile(`(?i)\b(engineer|developer|designer|manager|analyst|scientist|architect|consultant|lead|intern|specialist|administrator|tester|sdet|researcher|associate|executive|officer|director|head|recruiter|accountant)\b`)
	reEduSection  = regexp.MustCompile(`(?i)\b(education|academic|qualification)s?\b`)
	reExpSection  = regexp.MustCompile(`(?i)\b(experience|employment|work history|professional background)\b`)
)

// Parse reads a draft profile from resume text. now is the reference date for "Present".
func Parse(text string, cities []string, now time.Time) Draft {
	d := Draft{Skills: skills.Default().Extract(text)}
	d.YearsExp = years(text, now)
	d.Titles = titles(text)
	d.CurrentCity = city(text, cities)
	d.Education = education(text)
	if d.Skills == nil {
		d.Skills = []string{}
	}
	return d
}

type span struct{ from, to time.Time }

// years sums the work date ranges (merged, so overlapping jobs count once),
// skipping ranges inside the education section; an explicit "5+ years of
// experience" wins when it is larger.
func years(text string, now time.Time) *float64 {
	var spans []span
	for _, line := range workLines(text) {
		for _, m := range reRange.FindAllStringSubmatch(line, -1) {
			from, ok1 := parseDate(m[1:6], false, now)
			to, ok2 := parseDate(m[6:11], true, now)
			if m[11] != "" {
				to, ok2 = now, true
			}
			if !ok1 || !ok2 || !to.After(from) || from.Year() < 1970 || to.After(now.AddDate(0, 1, 0)) {
				continue
			}
			spans = append(spans, span{from, to})
		}
	}
	total := merged(spans)
	if m := reYearsStated.FindStringSubmatch(text); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v <= 50 && v > total {
			total = v
		}
	}
	if total == 0 && len(spans) == 0 {
		return nil
	}
	v := math.Round(total*10) / 10
	return &v
}

// workLines drops the education section, whose dates are not work experience.
func workLines(text string) []string {
	var out []string
	inEdu := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 40 {
			if reEduSection.MatchString(trimmed) {
				inEdu = true
				continue
			}
			if reExpSection.MatchString(trimmed) {
				inEdu = false
			}
		}
		if !inEdu {
			out = append(out, line)
		}
	}
	return out
}

func parseDate(g []string, end bool, now time.Time) (time.Time, bool) {
	// g: [month word+year: month, year] [mm/yyyy: mm, yyyy] [yyyy]
	switch {
	case g[0] != "" && g[1] != "":
		mon := months[strings.ToLower(g[0][:3])]
		if strings.HasPrefix(strings.ToLower(g[0]), "sept") {
			mon = 9
		}
		y := year(g[1], now)
		if mon == 0 || y == 0 {
			return time.Time{}, false
		}
		return time.Date(y, mon, 1, 0, 0, 0, 0, time.UTC), true
	case g[2] != "" && g[3] != "":
		mon, _ := strconv.Atoi(g[2])
		y := year(g[3], now)
		if mon < 1 || mon > 12 || y == 0 {
			return time.Time{}, false
		}
		return time.Date(y, time.Month(mon), 1, 0, 0, 0, 0, time.UTC), true
	case g[4] != "":
		y := year(g[4], now)
		if y == 0 {
			return time.Time{}, false
		}
		if end {
			return time.Date(y, 12, 1, 0, 0, 0, 0, time.UTC), true
		}
		return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}

func year(s string, now time.Time) int {
	s = strings.TrimLeft(s, "'’")
	y, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	if len(s) == 2 {
		y += 2000
		if y > now.Year() {
			y -= 100
		}
	}
	if y < 1950 || y > now.Year()+1 {
		return 0
	}
	return y
}

func merged(spans []span) float64 {
	if len(spans) == 0 {
		return 0
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].from.Before(spans[j].from) })
	total := 0.0
	cur := spans[0]
	for _, s := range spans[1:] {
		if !s.from.After(cur.to) {
			if s.to.After(cur.to) {
				cur.to = s.to
			}
			continue
		}
		total += cur.to.Sub(cur.from).Hours()
		cur = s
	}
	total += cur.to.Sub(cur.from).Hours()
	return total / 24 / 365.25
}

// titles: short lines that look like a job title (up to 5, first seen first).
func titles(text string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "•-*|·:"))
		for _, part := range strings.FieldsFunc(line, func(r rune) bool { return r == '|' || r == '–' || r == '—' }) {
			part = strings.TrimSpace(part)
			words := len(strings.Fields(part))
			if words < 1 || words > 6 || len(part) > 60 || !reTitleLine.MatchString(part) || strings.ContainsAny(part, "@:/") {
				continue
			}
			key := strings.ToLower(part)
			if !seen[key] {
				seen[key] = true
				out = append(out, part)
			}
			if len(out) == 5 {
				return out
			}
		}
	}
	return out
}

// city: the first known Indian city in the top of the resume (the contact block).
func city(text string, cities []string) *string {
	head := text
	if len(head) > 1500 {
		head = head[:1500]
	}
	lower := strings.ToLower(head)
	best, at := "", len(lower)+1
	for _, c := range cities {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(strings.ToLower(c)) + `\b`)
		if loc := re.FindStringIndex(lower); loc != nil && loc[0] < at {
			best, at = c, loc[0]
		}
	}
	if best == "" {
		return nil
	}
	return &best
}

func education(text string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, m := range reEducation.FindAllString(text, 6) {
		m = strings.TrimSpace(strings.Join(strings.Fields(m), " "))
		if !seen[strings.ToLower(m)] {
			seen[strings.ToLower(m)] = true
			out = append(out, m)
		}
	}
	return out
}
