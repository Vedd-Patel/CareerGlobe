// Package classify assigns role categories and seniority to job titles
// (spec: Role categories, Seniority bands). Admins can add keyword rules in
// category_rules; these defaults are used for anything they do not cover.
package classify

import (
	"regexp"
	"sort"
	"strings"
)

// Categories in the order the apps show them (S10).
var Categories = []string{
	"Software Engineering", "Hardware & Semiconductor", "Design", "Product", "Data & AI",
	"QA & Testing", "DevOps & IT", "Recruiting/HR", "Sales", "Marketing", "Operations",
	"Finance", "Customer Support", "Legal", "Other",
}

// SeniorityBands (spec): Intern 0, Fresher 0-1, Junior 1-3, Mid 3-6, Senior 6-10, Lead+ 10+.
var SeniorityBands = []string{"Intern", "Fresher", "Junior", "Mid", "Senior", "Lead+"}

// Rule maps a keyword (matched on word boundaries, case-insensitive) to a category.
// Lower priority wins when several rules match.
type Rule struct {
	Keyword  string
	Category string
	Priority int
}

// DefaultRules is the spec's keyword table, with a few common Indian title words.
var DefaultRules = []Rule{
	// Specific phrases first so "Product Designer" is Design, not Product.
	{"product designer", "Design", 10}, {"ux researcher", "Design", 10}, {"ui/ux", "Design", 10},
	{"design verification", "Hardware & Semiconductor", 10}, {"physical design", "Hardware & Semiconductor", 10},
	{"mixed signal", "Hardware & Semiconductor", 10}, {"product manager", "Product", 10},
	{"product owner", "Product", 10}, {"program manager", "Product", 20}, {"data scientist", "Data & AI", 10},
	{"data analyst", "Data & AI", 10}, {"data engineer", "Data & AI", 10}, {"machine learning", "Data & AI", 10},
	{"ai engineer", "Data & AI", 10}, {"business analyst", "Data & AI", 40},
	{"automation tester", "QA & Testing", 10}, {"talent acquisition", "Recruiting/HR", 10},
	{"account executive", "Sales", 10}, {"business development", "Sales", 10}, {"pre-sales", "Sales", 10},
	{"presales", "Sales", 10}, {"customer success", "Customer Support", 10}, {"service desk", "Customer Support", 10},
	{"supply chain", "Operations", 10}, {"project coordinator", "Operations", 10},
	{"system administrator", "DevOps & IT", 10}, {"site reliability", "DevOps & IT", 10},
	{"full stack", "Software Engineering", 10}, {"fp&a", "Finance", 10},

	{"vlsi", "Hardware & Semiconductor", 20}, {"rtl", "Hardware & Semiconductor", 20},
	{"asic", "Hardware & Semiconductor", 20}, {"fpga", "Hardware & Semiconductor", 20},
	{"dft", "Hardware & Semiconductor", 20}, {"analog", "Hardware & Semiconductor", 20},
	{"embedded", "Hardware & Semiconductor", 20}, {"firmware", "Hardware & Semiconductor", 20},
	{"pcb", "Hardware & Semiconductor", 20}, {"hardware", "Hardware & Semiconductor", 20},
	{"electronics", "Hardware & Semiconductor", 20}, {"silicon", "Hardware & Semiconductor", 20},
	{"cad engineer", "Hardware & Semiconductor", 20},

	{"software", "Software Engineering", 30}, {"sde", "Software Engineering", 30},
	{"backend", "Software Engineering", 30}, {"back end", "Software Engineering", 30},
	{"frontend", "Software Engineering", 30}, {"front end", "Software Engineering", 30},
	{"android", "Software Engineering", 30}, {"ios", "Software Engineering", 30},
	{"mobile", "Software Engineering", 30}, {"developer", "Software Engineering", 35},
	{"programmer", "Software Engineering", 35}, {"engineer", "Software Engineering", 90},

	{"ui", "Design", 30}, {"ux", "Design", 30}, {"visual designer", "Design", 30},
	{"interaction", "Design", 30}, {"graphic", "Design", 30}, {"designer", "Design", 35},

	{"ml", "Data & AI", 30}, {"ai", "Data & AI", 30}, {"analytics", "Data & AI", 40},

	{"qa", "QA & Testing", 30}, {"sdet", "QA & Testing", 30}, {"test", "QA & Testing", 30},
	{"tester", "QA & Testing", 30}, {"testing", "QA & Testing", 30}, {"quality assurance", "QA & Testing", 30},

	{"devops", "DevOps & IT", 30}, {"sre", "DevOps & IT", 30}, {"cloud", "DevOps & IT", 40},
	{"platform", "DevOps & IT", 45}, {"network", "DevOps & IT", 40}, {"security", "DevOps & IT", 40},
	{"infrastructure", "DevOps & IT", 40}, {"it support", "DevOps & IT", 30},

	{"recruiter", "Recruiting/HR", 30}, {"hr", "Recruiting/HR", 30}, {"human resources", "Recruiting/HR", 30},
	{"people", "Recruiting/HR", 50}, {"talent", "Recruiting/HR", 50},

	{"sales", "Sales", 30}, {"territory", "Sales", 40}, {"relationship manager", "Sales", 40},

	{"marketing", "Marketing", 30}, {"growth", "Marketing", 40}, {"seo", "Marketing", 30},
	{"content", "Marketing", 40}, {"brand", "Marketing", 40}, {"social media", "Marketing", 30},

	{"operations", "Operations", 50}, {"admin", "Operations", 50}, {"logistics", "Operations", 40},
	{"warehouse", "Operations", 40}, {"procurement", "Operations", 40},

	{"finance", "Finance", 30}, {"accountant", "Finance", 30}, {"accounts", "Finance", 40},
	{"audit", "Finance", 30}, {"tax", "Finance", 30}, {"treasury", "Finance", 30}, {"controller", "Finance", 40},

	{"support", "Customer Support", 45}, {"customer service", "Customer Support", 30},
	{"customer care", "Customer Support", 30},

	{"legal", "Legal", 30}, {"counsel", "Legal", 30}, {"compliance", "Legal", 35}, {"company secretary", "Legal", 30},
}

// Classifier holds compiled rules.
type Classifier struct {
	rules []compiled
}

type compiled struct {
	re *regexp.Regexp
	Rule
	order int
}

// New compiles rules; extra (admin) rules are tried alongside the defaults.
func New(extra []Rule) *Classifier {
	all := append(append([]Rule{}, extra...), DefaultRules...)
	c := &Classifier{}
	for i, r := range all {
		kw := strings.ToLower(strings.TrimSpace(r.Keyword))
		if kw == "" {
			continue
		}
		re := regexp.MustCompile(`(^|[^a-z0-9])` + regexp.QuoteMeta(kw) + `($|[^a-z0-9])`)
		c.rules = append(c.rules, compiled{re: re, Rule: r, order: i})
	}
	sort.SliceStable(c.rules, func(i, j int) bool { return c.rules[i].Priority < c.rules[j].Priority })
	return c
}

// Category returns the primary and (if different) secondary category for a
// title. Unknown titles are "Other" (they go to the admin review queue).
func (c *Classifier) Category(title string) (string, string) {
	t := " " + strings.ToLower(title) + " "
	var first, second string
	for _, r := range c.rules {
		if !r.re.MatchString(t) {
			continue
		}
		if first == "" {
			first = r.Category
		} else if r.Category != first && second == "" && r.Priority < 90 {
			second = r.Category
			break
		}
	}
	if first == "" {
		return "Other", ""
	}
	return first, second
}

var (
	reIntern  = regexp.MustCompile(`\b(intern|internship|trainee|apprentice)\b`)
	reFresher = regexp.MustCompile(`\b(fresher|graduate|entry level|associate engineer|campus)\b`)
	reJunior  = regexp.MustCompile(`\b(junior|jr\.?|associate|sde ?(i|1)|engineer ?(i|1)|level ?1|l1)\b`)
	reSenior  = regexp.MustCompile(`\b(senior|sr\.?|sde ?(iii|3)|level ?3|l3|staff)\b`)
	reLead    = regexp.MustCompile(`\b(lead|principal|head|director|vp|vice president|architect|manager|chief|distinguished|avp)\b`)
	reMid     = regexp.MustCompile(`\b(sde ?(ii|2)|engineer ?(ii|2)|level ?2|l2)\b`)
)

// Seniority reads the level from a title; titles with no level read as Mid.
func Seniority(title string) string {
	t := strings.ToLower(title)
	switch {
	case reIntern.MatchString(t):
		return "Intern"
	case reFresher.MatchString(t):
		return "Fresher"
	case reLead.MatchString(t) && !strings.Contains(t, "product manager") && !strings.Contains(t, "account manager"):
		return "Lead+"
	case reSenior.MatchString(t):
		return "Senior"
	case reMid.MatchString(t):
		return "Mid"
	case reJunior.MatchString(t):
		return "Junior"
	}
	return "Mid"
}

// BandForYears maps years of experience to a seniority band.
func BandForYears(years float64) string {
	switch {
	case years <= 0:
		return "Fresher"
	case years < 1:
		return "Fresher"
	case years < 3:
		return "Junior"
	case years < 6:
		return "Mid"
	case years < 10:
		return "Senior"
	}
	return "Lead+"
}

// BandIndex is the position of a band in SeniorityBands (-1 if unknown).
func BandIndex(band string) int {
	for i, b := range SeniorityBands {
		if b == band {
			return i
		}
	}
	return -1
}
