package match

import (
	"testing"
	"time"
)

var now = time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)

func states(city string) string {
	return map[string]string{"Bengaluru": "Karnataka", "Mysuru": "Karnataka", "Pune": "Maharashtra", "Mumbai": "Maharashtra"}[city]
}

func years(v float64) *float64 { return &v }

var alice = User{
	Skills: []string{"Go", "Kafka", "PostgreSQL", "Kubernetes", "AWS"}, Titles: []string{"Senior Backend Engineer"},
	YearsExp: years(7), City: "Bengaluru", Categories: []string{"Software Engineering"},
	Cities: []string{"Bengaluru"}, RemoteOK: true,
}

func TestStrongMatch(t *testing.T) {
	j := Job{Title: "Senior Backend Engineer", Category: "Software Engineering",
		Skills: []string{"Go", "Kafka", "PostgreSQL", "Kubernetes", "GraphQL"}, Cities: []string{"Bengaluru"}, FirstSeen: now}
	score, r := Score(alice, j, states, now)
	if score < 90 {
		t.Fatalf("score = %d, %+v", score, r)
	}
	if r.Summary != "Matches 4 of 5 skills · missing GraphQL" {
		t.Fatalf("summary = %q", r.Summary)
	}
	if r.Seniority.Fit != "exact" || r.Location != "Bengaluru · a city you chose" {
		t.Fatalf("reasons = %+v", r)
	}
}

func TestOrderingOfSignals(t *testing.T) {
	base := Job{Title: "Backend Engineer", Category: "Software Engineering",
		Skills: []string{"Go", "Kafka", "AWS"}, Cities: []string{"Bengaluru"}, FirstSeen: now}
	exact, _ := Score(alice, base, states, now)

	sameState := base
	sameState.Cities = []string{"Mysuru"}
	ss, r := Score(alice, sameState, states, now)
	remote := base
	remote.Cities, remote.WorkMode = []string{"Pune"}, "remote"
	rm, _ := Score(alice, remote, states, now)
	far := base
	far.Cities = []string{"Pune"}
	fr, _ := Score(alice, far, states, now)
	if !(exact > ss && ss > rm && rm > fr) {
		t.Fatalf("location order exact %d > state %d > remote %d > other %d (%s)", exact, ss, rm, fr, r.Location)
	}

	old := base
	old.FirstSeen = now.AddDate(0, 0, -40)
	o, _ := Score(alice, old, states, now)
	if exact-o != 5 {
		t.Fatalf("freshness worth 5 points, got %d", exact-o)
	}

	intern := base
	intern.Title = "Backend Engineering Intern"
	in, r2 := Score(alice, intern, states, now)
	if in >= exact || r2.Seniority.Fit != "far" {
		t.Fatalf("intern role for 7 years: %d vs %d (%+v)", in, exact, r2.Seniority)
	}
}

func TestFewSkillsSharesWeight(t *testing.T) {
	j := Job{Title: "Senior Backend Engineer", Category: "Software Engineering", Skills: []string{"Go"},
		Cities: []string{"Bengaluru"}, FirstSeen: now}
	score, r := Score(alice, j, states, now)
	if !r.FewSkills || r.Summary != "Few skills listed in this posting" || r.Parts["skills"] != 0 {
		t.Fatalf("%+v", r)
	}
	if score != 100 {
		t.Fatalf("perfect role/seniority/location with few skills = %d, want 100", score)
	}
}

func TestNoCategoriesAndNoYears(t *testing.T) {
	u := User{Skills: []string{"Figma"}, Titles: []string{"Product Designer"}, Cities: []string{"Pune"}}
	j := Job{Title: "Product Designer", Category: "Design", Skills: []string{"Figma", "Prototyping", "User Research"},
		Cities: []string{"Pune"}, FirstSeen: now}
	score, r := Score(u, j, states, now)
	if score < 50 || score > 90 || r.Seniority.Fit != "close" {
		t.Fatalf("score = %d, %+v", score, r)
	}
}

func TestSummaryTruncatesMissing(t *testing.T) {
	r := Reasons{SkillsHave: []string{"Go"}, SkillsMissing: []string{"A", "B", "C", "D", "E"}}
	if got := summary(r, 6); got != "Matches 1 of 6 skills · missing A, B, C +2" {
		t.Fatalf("summary = %q", got)
	}
}
