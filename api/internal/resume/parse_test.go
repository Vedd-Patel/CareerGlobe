package resume

import (
	"slices"
	"testing"
	"time"
)

var cities = []string{"Bengaluru", "Bangalore", "Pune", "Hyderabad", "Mumbai", "Delhi", "Chennai"}

const sample = `Priya Sharma
priya@example.com | +91 98xxxxxx01 | Pune, Maharashtra
Senior Software Engineer

SKILLS
Java, Spring Boot, Kafka, PostgreSQL, k8s, AWS, React

EXPERIENCE
Senior Software Engineer | Acme Payments          Jan 2021 - Present
Built payment APIs in Java and Spring Boot.
Software Engineer | Beta Labs                      07/2017 – 12/2020
Kafka pipelines; on-call for k8s clusters.
Intern | Gamma                                     May 2016 - Jul 2016

EDUCATION
B.Tech in Computer Science, College of Engineering Pune   2013 - 2017
`

func TestParse(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	d := Parse(sample, cities, now)
	for _, s := range []string{"Java", "Spring Boot", "Kafka", "PostgreSQL", "Kubernetes", "AWS", "React"} {
		if !slices.Contains(d.Skills, s) {
			t.Errorf("missing skill %q in %v", s, d.Skills)
		}
	}
	// Jul 2017 - Oct 2026 merged, plus the 2-month internship; the 2013-2017 degree is not work.
	if d.YearsExp == nil || *d.YearsExp < 9.3 || *d.YearsExp > 9.6 {
		t.Errorf("years = %v", d.YearsExp)
	}
	if d.CurrentCity == nil || *d.CurrentCity != "Pune" {
		t.Errorf("city = %v", d.CurrentCity)
	}
	if !slices.Contains(d.Titles, "Senior Software Engineer") || !slices.Contains(d.Titles, "Software Engineer") {
		t.Errorf("titles = %v", d.Titles)
	}
	if len(d.Education) == 0 || d.Education[0][:6] != "B.Tech" {
		t.Errorf("education = %v", d.Education)
	}
}

func TestStatedYearsWinWhenLarger(t *testing.T) {
	d := Parse("Backend developer with 12+ years of experience in Go and Python. Worked 2022 - 2024.", cities,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if d.YearsExp == nil || *d.YearsExp != 12 {
		t.Fatalf("years = %v", d.YearsExp)
	}
}

func TestFreshResumeHasNoYears(t *testing.T) {
	d := Parse("Final year student. Skills: Python, SQL. Projects in machine learning.", cities, time.Now())
	if d.YearsExp != nil {
		t.Fatalf("years = %v", *d.YearsExp)
	}
}
