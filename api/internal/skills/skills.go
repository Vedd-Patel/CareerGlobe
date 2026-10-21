// Package skills finds known skills in text, the same way for resumes and job
// descriptions, with one synonym table (spec: Skill synonyms). Admins can add
// synonyms in the skill_synonyms table; Merge folds them in.
package skills

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// canonical skills by group. Spellings in the text are matched case-insensitively
// on word boundaries; symbols like "C++" and "C#" are handled explicitly.
var groups = map[string][]string{
	"software": {
		"Java", "Python", "JavaScript", "TypeScript", "Go", "Rust", "C", "C++", "C#", "Kotlin", "Swift",
		"Objective-C", "Scala", "Ruby", "PHP", "Perl", "R", "MATLAB", "Dart", "Elixir", "Haskell", "Lua",
		"Bash", "PowerShell", "SQL", "PL/SQL", "NoSQL", "GraphQL", "REST", "gRPC", "WebSockets",
		"React", "React Native", "Angular", "Vue.js", "Svelte", "Next.js", "Node.js", "Express.js", "NestJS",
		"Django", "Flask", "FastAPI", "Spring", "Spring Boot", "Hibernate", ".NET", "ASP.NET", "Ruby on Rails",
		"Laravel", "Flutter", "Jetpack Compose", "SwiftUI", "Android", "iOS", "HTML", "CSS", "Sass",
		"Tailwind CSS", "Redux", "jQuery", "Webpack", "Vite", "Microservices", "System Design",
		"Data Structures", "Algorithms", "Object-Oriented Programming", "Design Patterns", "Multithreading",
		"Distributed Systems", "Kafka", "RabbitMQ", "Redis", "Elasticsearch", "PostgreSQL", "MySQL",
		"MongoDB", "Cassandra", "DynamoDB", "Oracle Database", "SQL Server", "SQLite", "Firebase",
		"Unit Testing", "TDD", "Git", "Linux", "Unix", "JVM", "WebRTC", "OAuth", "Salesforce", "SAP",
		"ServiceNow", "Guidewire", "Mainframe", "COBOL", "Blockchain", "Solidity", "Unity", "Unreal Engine",
	},
	"data": {
		"Machine Learning", "Deep Learning", "NLP", "Computer Vision", "LLM", "Generative AI", "RAG",
		"TensorFlow", "PyTorch", "Keras", "scikit-learn", "Pandas", "NumPy", "Spark", "PySpark", "Hadoop",
		"Hive", "Airflow", "dbt", "Snowflake", "BigQuery", "Redshift", "Databricks", "Tableau", "Power BI",
		"Looker", "Excel", "Statistics", "A/B Testing", "Data Modeling", "ETL", "Data Warehousing",
		"Feature Engineering", "MLOps", "Hugging Face", "LangChain", "OpenCV", "Time Series",
	},
	"devops": {
		"AWS", "Azure", "GCP", "Docker", "Kubernetes", "Terraform", "Ansible", "Jenkins", "GitHub Actions",
		"GitLab CI", "CI/CD", "Prometheus", "Grafana", "Nginx", "Helm", "Istio", "Linux Administration",
		"Networking", "TCP/IP", "DNS", "Load Balancing", "Site Reliability Engineering", "Observability",
		"Cybersecurity", "Penetration Testing", "SIEM", "IAM", "Active Directory", "VMware", "Shell Scripting",
	},
	"hardware": { // spec: hardware / VLSI list
		"Verilog", "SystemVerilog", "VHDL", "UVM", "Cadence", "Synopsys", "Mentor Graphics", "RTL Design",
		"Design Verification", "Physical Design", "Static Timing Analysis", "DFT", "FPGA", "ASIC",
		"Analog Design", "Mixed Signal", "Embedded C", "Embedded Linux", "RTOS", "Firmware", "Microcontrollers",
		"ARM", "RISC-V", "PCB Design", "Altium", "OrCAD", "Device Drivers", "Linux Kernel", "CAN", "AUTOSAR",
		"I2C", "SPI", "UART", "PCIe", "DDR", "Power Electronics", "Signal Processing", "SoC", "Synthesis",
		"Formal Verification", "Python Scripting", "TCL", "Perl Scripting", "LabVIEW", "Simulink",
	},
	"design": { // spec: design list
		"Figma", "Sketch", "Adobe XD", "Photoshop", "Illustrator", "InDesign", "After Effects",
		"User Research", "Prototyping", "Wireframing", "Interaction Design", "Visual Design", "Design Systems",
		"Usability Testing", "Information Architecture", "Accessibility", "Motion Design", "UI Design",
		"UX Design", "Product Design", "Typography",
	},
	"business": {
		"Product Management", "Agile", "Scrum", "Jira", "Confluence", "Stakeholder Management",
		"Roadmapping", "Project Management", "Program Management", "Business Analysis", "SEO", "SEM",
		"Google Analytics", "Content Writing", "Copywriting", "Social Media Marketing", "Performance Marketing",
		"Email Marketing", "CRM", "HubSpot", "Sales", "B2B Sales", "Lead Generation", "Negotiation",
		"Account Management", "Customer Success", "Recruiting", "Talent Acquisition", "HR Operations",
		"Payroll", "Accounting", "Financial Modeling", "FP&A", "Taxation", "GST", "Auditing", "Tally",
		"Supply Chain", "Logistics", "Procurement", "Six Sigma", "Lean", "Operations Management",
		"Compliance", "Contract Management", "Communication", "Leadership", "Team Management",
	},
}

// synonyms: spelling in text -> canonical skill.
var synonyms = map[string]string{
	"js": "JavaScript", "javascript": "JavaScript", "golang": "Go", "python3": "Python", "cpp": "C++", "c plus plus": "C++", "csharp": "C#", "c sharp": "C#",
	"reactjs": "React", "react.js": "React", "react js": "React", "angularjs": "Angular", "angular.js": "Angular",
	"vue": "Vue.js", "vuejs": "Vue.js", "nextjs": "Next.js", "node": "Node.js", "nodejs": "Node.js",
	"node js": "Node.js", "express": "Express.js", "expressjs": "Express.js", "springboot": "Spring Boot",
	"dotnet": ".NET", ".net core": ".NET", "rails": "Ruby on Rails", "ror": "Ruby on Rails",
	"k8s": "Kubernetes", "kube": "Kubernetes", "amazon web services": "AWS", "google cloud": "GCP",
	"google cloud platform": "GCP", "microsoft azure": "Azure", "postgres": "PostgreSQL", "psql": "PostgreSQL",
	"mongo": "MongoDB", "ml": "Machine Learning", "natural language processing": "NLP",
	"genai": "Generative AI", "gen ai": "Generative AI", "llms": "LLM",
	"large language models": "LLM", "sklearn": "scikit-learn", "scikit learn": "scikit-learn",
	"powerbi": "Power BI", "ms excel": "Excel", "advanced excel": "Excel",
	"sv": "SystemVerilog", "system verilog": "SystemVerilog", "sta": "Static Timing Analysis",
	"rtl": "RTL Design", "dv": "Design Verification", "embedded systems": "Firmware",
	"ci cd": "CI/CD", "cicd": "CI/CD", "sre": "Site Reliability Engineering", "oop": "Object-Oriented Programming",
	"oops": "Object-Oriented Programming", "dsa": "Data Structures", "rest api": "REST", "restful": "REST",
	"rest apis": "REST", "ux research": "User Research", "ui/ux": "UX Design", "xd": "Adobe XD",
	"product manager": "Product Management", "jira software": "Jira",
	"ga4": "Google Analytics", "tally erp": "Tally", "fp & a": "FP&A", "talent acquisition": "Talent Acquisition",
	"rabbit mq": "RabbitMQ", "elastic search": "Elasticsearch", "github": "Git", "gitlab": "Git",
	"bitbucket": "Git", "shell": "Bash", "unix shell": "Bash", "plsql": "PL/SQL", "mssql": "SQL Server",
	"ms sql": "SQL Server", "pyspark": "PySpark", "apache spark": "Spark", "apache kafka": "Kafka",
	"apache airflow": "Airflow", "huggingface": "Hugging Face", "open cv": "OpenCV",
}

// Spellings that only count in capitals (they are ordinary words or codes otherwise).
var upperOnly = map[string]bool{"js": true, "ml": true, "sv": true, "dv": true, "sta": true, "rtl": true,
	"sre": true, "dsa": true, "oop": true, "oops": true, "ror": true, "SAP": true, "CAN": true, "ARM": true,
	"SPI": true, "DFT": true, "SoC": true, "ASIC": true, "IAM": true, "DNS": true, "SEO": true, "SEM": true,
	"GST": true, "CRM": true, "RAG": true, "LLM": true, "NLP": true, "ETL": true, "TDD": true}

// One-letter or very common words: only inside a list ("Java, Go, Python").
var listOnly = map[string]bool{"Go": true, "R": true, "C": true}

type matcher struct {
	re        *regexp.Regexp
	canonical string
}

// Extractor finds skills in text.
type Extractor struct {
	matchers []matcher
}

var (
	defaultOnce sync.Once
	defaultExt  *Extractor
)

// Default is the extractor with the built-in list.
func Default() *Extractor {
	defaultOnce.Do(func() { defaultExt = New(nil) })
	return defaultExt
}

// All returns every canonical skill, sorted.
func All() []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	sort.Strings(out)
	return out
}

// New builds an extractor with extra synonyms (alias -> canonical), e.g. from the DB.
func New(extra map[string]string) *Extractor {
	spell := map[string]string{} // spelling -> canonical
	for _, g := range groups {
		for _, s := range g {
			spell[s] = s
		}
	}
	for a, c := range synonyms {
		spell[a] = c
	}
	for a, c := range extra {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" && c != "" {
			spell[a] = c
		}
	}
	keys := make([]string, 0, len(spell))
	for k := range spell {
		keys = append(keys, k)
	}
	// Longest first, so "React Native" is tried before "React", "Spring Boot" before "Spring".
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	e := &Extractor{}
	for _, k := range keys {
		var pat string
		switch {
		case listOnly[k]:
			pat = `(^|[,/|•·;:(]\s*)` + regexp.QuoteMeta(k) + `(\s*[,/|•·;)]|$)`
		case upperOnly[k]:
			pat = `(^|[^A-Za-z0-9+#.])` + regexp.QuoteMeta(strings.ToUpper(k)) + `($|[^A-Za-z0-9+#])`
			if k != strings.ToLower(k) { // mixed-case canonical like "SoC": exact spelling
				pat = `(^|[^A-Za-z0-9+#.])` + regexp.QuoteMeta(k) + `($|[^A-Za-z0-9+#])`
			}
		default:
			pat = `(?i)(^|[^A-Za-z0-9+#.])` + regexp.QuoteMeta(k) + `($|[^A-Za-z0-9+#])`
		}
		e.matchers = append(e.matchers, matcher{regexp.MustCompile(pat), spell[k]})
	}
	return e
}

// Extract returns the canonical skills found in text, in first-seen order.
func (e *Extractor) Extract(text string) []string {
	if text == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range e.matchers {
		if m.canonical == "" || seen[m.canonical] {
			continue
		}
		if m.re.MatchString(text) {
			seen[m.canonical] = true
			out = append(out, m.canonical)
		}
	}
	return out
}

// Canonical maps one skill as typed by a user to its canonical name (or
// returns it trimmed when unknown), so user-added chips match job text.
func (e *Extractor) Canonical(s string) string {
	found := e.Extract(s)
	if len(found) == 1 {
		return found[0]
	}
	return strings.TrimSpace(s)
}
