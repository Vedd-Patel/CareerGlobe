// Package match scores a job for a user, 0-100, always with reasons
// (spec: Scoring a job). Weights are a starting point: tune them only when the
// test set's "good fits in the top 20" goes up.
package match

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/classify"
)

// Weights (spec): skills 45, role 20, seniority 15, location 15, freshness 5.
const (
	WSkills    = 45.0
	WRole      = 20.0
	WSeniority = 15.0
	WLocation  = 15.0
	WFresh     = 5.0
	// MinSkills: below this many recognised skills in a posting, skills overlap
	// is skipped and its weight is shared by role fit and seniority.
	MinSkills = 3
	// Threshold: only scores at or above this count as a match.
	Threshold = 60
)

// User is the confirmed profile plus preferences.
type User struct {
	Skills     []string
	Titles     []string
	YearsExp   *float64
	City       string
	Categories []string
	Cities     []string
	Anywhere   bool
	RemoteOK   bool
	Seniority  []string
}

// Job is what the scorer needs from a job.
type Job struct {
	Title     string
	Category  string
	Category2 string
	Seniority string
	Skills    []string
	Cities    []string
	WorkMode  string
	FirstSeen time.Time
}

// Reasons are stored as JSON with each match and shown on the job card and
// the match breakdown (S19, S20).
type Reasons struct {
	Summary       string             `json:"summary"`
	SkillsHave    []string           `json:"skills_have"`
	SkillsMissing []string           `json:"skills_missing"`
	FewSkills     bool               `json:"few_skills"`
	Seniority     SeniorityFit       `json:"seniority"`
	Location      string             `json:"location"`
	Parts         map[string]float64 `json:"parts"`
}

// SeniorityFit compares the job's level with the user's.
type SeniorityFit struct {
	Job  string `json:"job"`
	You  string `json:"you"`
	Fit  string `json:"fit"` // "exact", "close", "far"
	Info string `json:"info"`
}

// Score returns 0-100 and the reasons. now is the reference time for freshness.
func Score(u User, j Job, stateOf func(city string) string, now time.Time) (int, Reasons) {
	r := Reasons{Parts: map[string]float64{}}
	wSkills, wRole, wSen := WSkills, WRole, WSeniority

	// Skills overlap: job skills the profile has, over job skills.
	have := map[string]bool{}
	for _, s := range u.Skills {
		have[strings.ToLower(s)] = true
	}
	for _, s := range j.Skills {
		if have[strings.ToLower(s)] {
			r.SkillsHave = append(r.SkillsHave, s)
		} else {
			r.SkillsMissing = append(r.SkillsMissing, s)
		}
	}
	var skills float64
	if len(j.Skills) < MinSkills {
		r.FewSkills = true
		wRole += WSkills * WRole / (WRole + WSeniority)
		wSen += WSkills * WSeniority / (WRole + WSeniority)
		wSkills = 0
	} else {
		skills = float64(len(r.SkillsHave)) / float64(len(j.Skills))
	}

	role := roleFit(u, j)
	sen := seniorityFit(u, j, &r.Seniority)
	loc := locationFit(u, j, stateOf, &r.Location)
	fresh := freshness(j.FirstSeen, now)

	total := wSkills*skills + wRole*role + wSen*sen + WLocation*loc + WFresh*fresh
	r.Parts = map[string]float64{"skills": round1(wSkills * skills), "role": round1(wRole * role),
		"seniority": round1(wSen * sen), "location": round1(WLocation * loc), "freshness": round1(WFresh * fresh)}
	r.Summary = summary(r, len(j.Skills))
	if r.SkillsHave == nil {
		r.SkillsHave = []string{}
	}
	if r.SkillsMissing == nil {
		r.SkillsMissing = []string{}
	}
	return int(math.Round(math.Max(0, math.Min(100, total)))), r
}

// roleFit: half for the job being in a chosen category, half for the title
// sharing words with the user's own titles.
func roleFit(u User, j Job) float64 {
	cat := 0.5 // no categories chosen: neutral
	if len(u.Categories) > 0 {
		cat = 0
		if slices.Contains(u.Categories, j.Category) {
			cat = 1
		} else if j.Category2 != "" && slices.Contains(u.Categories, j.Category2) {
			cat = 0.8
		}
	}
	best := 0.0
	jt := words(j.Title)
	for _, t := range u.Titles {
		best = math.Max(best, overlap(jt, words(t)))
	}
	return 0.5*cat + 0.5*best
}

var stop = map[string]bool{"and": true, "of": true, "the": true, "for": true, "in": true, "at": true, "with": true,
	"to": true, "a": true, "i": true, "ii": true, "iii": true, "1": true, "2": true, "3": true, "sr": true, "jr": true,
	"senior": true, "junior": true, "lead": true, "principal": true, "staff": true, "associate": true}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '#')
	}) {
		if !stop[w] {
			out[w] = true
		}
	}
	return out
}

// overlap: shared words over the job title's words.
func overlap(job, mine map[string]bool) float64 {
	if len(job) == 0 {
		return 0
	}
	n := 0
	for w := range job {
		if mine[w] {
			n++
		}
	}
	return float64(n) / float64(len(job))
}

func seniorityFit(u User, j Job, out *SeniorityFit) float64 {
	jobBand := j.Seniority
	if jobBand == "" {
		jobBand = classify.Seniority(j.Title)
	}
	out.Job = jobBand
	if len(u.Seniority) > 0 && slices.Contains(u.Seniority, jobBand) {
		out.You, out.Fit, out.Info = strings.Join(u.Seniority, "/"), "exact", "Matches the level you chose"
		return 1
	}
	if u.YearsExp == nil {
		out.Fit, out.Info = "close", "Add your years of experience for a better match"
		return 0.6
	}
	you := classify.BandForYears(*u.YearsExp)
	out.You = you
	d := classify.BandIndex(jobBand) - classify.BandIndex(you)
	if d < 0 {
		d = -d
	}
	switch d {
	case 0:
		out.Fit, out.Info = "exact", fmt.Sprintf("%s role, you have %.4g years", jobBand, *u.YearsExp)
		return 1
	case 1:
		out.Fit, out.Info = "close", fmt.Sprintf("%s role, one level from yours (%s)", jobBand, you)
		return 0.6
	case 2:
		out.Fit, out.Info = "far", fmt.Sprintf("%s role, two levels from yours (%s)", jobBand, you)
		return 0.2
	}
	out.Fit, out.Info = "far", fmt.Sprintf("%s role, far from your level (%s)", jobBand, you)
	return 0
}

// locationFit (spec): exact city > same state > remote.
func locationFit(u User, j Job, stateOf func(string) string, out *string) float64 {
	wanted := u.Cities
	if len(wanted) == 0 && u.City != "" {
		wanted = []string{u.City}
	}
	for _, c := range j.Cities {
		if containsFold(wanted, c) {
			*out = c + " · a city you chose"
			return 1
		}
	}
	if stateOf != nil {
		states := map[string]bool{}
		for _, c := range wanted {
			if s := stateOf(c); s != "" {
				states[s] = true
			}
		}
		for _, c := range j.Cities {
			if s := stateOf(c); s != "" && states[s] {
				*out = c + " · same state as your cities"
				return 0.7
			}
		}
	}
	if j.WorkMode == "remote" && (u.RemoteOK || u.Anywhere) {
		*out = "Remote"
		return 0.6
	}
	if u.Anywhere {
		*out = strings.Join(j.Cities, ", ") + " · anywhere in India"
		return 0.5
	}
	*out = strings.Join(j.Cities, ", ")
	return 0
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// freshness: 1 for a job first seen today, falling to 0 at 30 days.
func freshness(first, now time.Time) float64 {
	days := now.Sub(first).Hours() / 24
	return math.Max(0, math.Min(1, 1-days/30))
}

func summary(r Reasons, jobSkills int) string {
	if r.FewSkills {
		return "Few skills listed in this posting"
	}
	s := fmt.Sprintf("Matches %d of %d skills", len(r.SkillsHave), jobSkills)
	if n := len(r.SkillsMissing); n > 0 {
		missing := r.SkillsMissing
		if n > 3 {
			missing = missing[:3]
		}
		s += " · missing " + strings.Join(missing, ", ")
		if n > 3 {
			s += fmt.Sprintf(" +%d", n-3)
		}
	}
	return s
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
