package server

import (
	"encoding/json"
	"errors"
	"html"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
)

const (
	maxPins     = 2000 // the web client clusters pins itself, so a view ships every location (spec said 500)
	clusterZoom = 7.0  // below this zoom, pins are clustered on a grid
)

type jobFilters struct {
	Category    *string
	RemoteOnly  bool
	CompanyType *string
	PostedSince *time.Time
	HiringOnly  bool
	MatchesOnly bool
	Employment  []string // intern, full_time, part_time, contract; nil = any
	ExpMin      *int32   // years of experience a job may ask for: a job passes when its range meets [ExpMin, ExpMax]
	ExpMax      *int32
	MinSalary   *int32 // yearly, in rupees; only jobs that list a salary at least this high pass
}

func (f jobFilters) any() bool {
	return f.Category != nil || f.RemoteOnly || f.PostedSince != nil || f.Employment != nil || f.ExpMin != nil || f.MinSalary != nil
}

var employmentTypes = []string{"intern", "full_time", "part_time", "contract"}

// queryInt32 is nil when the parameter is missing or not a whole number; otherwise clamped.
func queryInt32(r *http.Request, name string, min, max int) *int32 {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return nil
	}
	n := int32(clamp(v, min, max))
	return &n
}

func filtersFrom(r *http.Request) jobFilters {
	q := r.URL.Query()
	f := jobFilters{
		Category:    optString(q.Get("category")),
		RemoteOnly:  queryBool(r, "remote"),
		HiringOnly:  queryBool(r, "hiring"),
		MatchesOnly: queryBool(r, "matches"),
	}
	for _, e := range strings.Split(q.Get("employment"), ",") {
		if e = strings.TrimSpace(e); slices.Contains(employmentTypes, e) && !slices.Contains(f.Employment, e) {
			f.Employment = append(f.Employment, e)
		}
	}
	if lo, hi := queryInt32(r, "exp_min", 0, 60), queryInt32(r, "exp_max", 0, 60); lo != nil {
		if hi == nil || *hi < *lo {
			hi = lo
		}
		f.ExpMin, f.ExpMax = lo, hi
	}
	f.MinSalary = queryInt32(r, "min_salary", 0, 100_000_000)
	if t := q.Get("type"); t == "product" || t == "service" {
		f.CompanyType = &t
	}
	if d, ok := map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}[q.Get("posted")]; ok {
		since := time.Now().Add(-d)
		f.PostedSince = &since
	}
	return f
}

type bbox struct{ west, south, east, north float64 }

func bboxFrom(r *http.Request) (bbox, error) {
	q := r.URL.Query()
	vals := [4]float64{}
	for i, k := range []string{"west", "south", "east", "north"} {
		v, err := strconv.ParseFloat(q.Get(k), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return bbox{}, badRequest("west, south, east and north are required numbers.")
		}
		vals[i] = v
	}
	b := bbox{vals[0], vals[1], vals[2], vals[3]}
	if b.south < -90 || b.north > 90 || b.south > b.north || b.west < -180 || b.east > 180 {
		return bbox{}, badRequest("Map bounds are out of range.")
	}
	if b.west > b.east { // crosses the antimeridian: India never does, so take the whole width
		b.west, b.east = -180, 180
	}
	return b, nil
}

// GET /v1/globe?west&south&east&north&zoom&filters: pins (or clusters when
// zoomed out) for the map (S14).
func (s *Server) globe(w http.ResponseWriter, r *http.Request) error {
	b, err := bboxFrom(r)
	if err != nil {
		return err
	}
	zoom, _ := strconv.ParseFloat(r.URL.Query().Get("zoom"), 64)
	f := filtersFrom(r)
	uid := account(r).UserID
	if zoom < clusterZoom {
		cell := 360 / math.Pow(2, math.Max(zoom, 1)) / 8
		rows, err := s.q.ClustersInView(r.Context(), db.ClustersInViewParams{
			UserID: uid, Category: f.Category, RemoteOnly: f.RemoteOnly, PostedSince: f.PostedSince,
			West: b.west, South: b.south, East: b.east, North: b.north, CompanyType: f.CompanyType,
			HiringOnly: f.HiringOnly, MatchesOnly: f.MatchesOnly, JobFilters: f.any(), Cell: cell, MaxPins: maxPins,
			Employment: f.Employment, ExpMin: f.ExpMin, ExpMax: f.ExpMax, MinSalary: f.MinSalary,
		})
		if err != nil {
			return err
		}
		return ok(w, map[string]any{"mode": "clusters", "items": rows, "updated_at": s.lastCrawl(r)})
	}
	rows, err := s.q.PinsInView(r.Context(), db.PinsInViewParams{
		UserID: uid, Category: f.Category, RemoteOnly: f.RemoteOnly, PostedSince: f.PostedSince,
		West: b.west, South: b.south, East: b.east, North: b.north, CompanyType: f.CompanyType,
		HiringOnly: f.HiringOnly, MatchesOnly: f.MatchesOnly, JobFilters: f.any(), MaxPins: maxPins,
		Employment: f.Employment, ExpMin: f.ExpMin, ExpMax: f.ExpMax, MinSalary: f.MinSalary,
	})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"mode": "pins", "items": rows, "updated_at": s.lastCrawl(r)})
}

func (s *Server) lastCrawl(r *http.Request) *time.Time {
	st, err := s.q.PublicStats(r.Context())
	if err != nil {
		return nil
	}
	return st.LastCrawlAt
}

// GET /v1/companies/in-view: the list beside the globe, sorted by matches (or nearest to lat, lon).
func (s *Server) companiesInView(w http.ResponseWriter, r *http.Request) error {
	b, err := bboxFrom(r)
	if err != nil {
		return err
	}
	f := filtersFrom(r)
	limit := queryInt(r, "limit", 50, 1, 100)
	// lat and lon (the map centre or the member's position) add a distance to each company;
	// sort=nearest orders by it.
	var nearLat, nearLon *float64
	if la, lo := queryFloat(r, "lat"), queryFloat(r, "lon"); la != nil && lo != nil && *la >= -90 && *la <= 90 && *lo >= -180 && *lo <= 180 {
		nearLat, nearLon = la, lo
	}
	rows, err := s.q.CompaniesInView(r.Context(), db.CompaniesInViewParams{
		UserID: account(r).UserID, West: b.west, South: b.south, East: b.east, North: b.north,
		CompanyType: f.CompanyType, MaxRows: int32(limit), Skip: int32(queryInt(r, "offset", 0, 0, 10_000)),
		NearLat: nearLat, NearLon: nearLon, Nearest: nearLat != nil && r.URL.Query().Get("sort") == "nearest",
	})
	if err != nil {
		return err
	}
	if nearLat == nil {
		return ok(w, map[string]any{"items": rows})
	}
	ids := make([]int64, len(rows))
	for i, c := range rows {
		ids[i] = c.ID
	}
	dist, err := s.q.CompanyDistances(r.Context(), db.CompanyDistancesParams{Lat: *nearLat, Lon: *nearLon, CompanyIds: ids})
	if err != nil {
		return err
	}
	km := map[int64]float64{}
	for _, d := range dist {
		km[d.CompanyID] = d.Km
	}
	items := make([]map[string]any, len(rows))
	for i, c := range rows {
		items[i] = map[string]any{"id": c.ID, "name": c.Name, "slug": c.Slug, "type": c.Type, "logo_url": c.LogoUrl,
			"city": c.City, "address": c.Address, "open_jobs": c.OpenJobs, "matches": c.Matches}
		if d, found := km[c.ID]; found {
			items[i]["distance_km"] = d
		}
	}
	return ok(w, map[string]any{"items": items})
}

// GET /v1/companies/{id}: company detail (S18).
func (s *Server) company(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	uid := account(r).UserID
	c, err := s.q.GetCompany(r.Context(), db.GetCompanyParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	locs, err := s.q.CompanyLocations(r.Context(), id)
	if err != nil {
		return err
	}
	jobs, err := s.q.CompanyOpenJobs(r.Context(), db.CompanyOpenJobsParams{UserID: uid, CompanyID: id})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{
		"id": c.ID, "name": c.Name, "slug": c.Slug, "website": c.Website, "logo_url": c.LogoUrl, "type": c.Type,
		"stage": c.Stage, "size_band": c.SizeBand, "industry": c.Industry, "description": c.Description,
		"careers_url": c.CareersUrl, "last_checked_at": c.LastCheckedAt, "saved": c.Saved,
		"locations": locs, "open_jobs": jobs,
	})
}

func (s *Server) saveCompany(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.q.SaveCompany(r.Context(), db.SaveCompanyParams{UserID: account(r).UserID, CompanyID: id}); err != nil {
		if isForeignKey(err) {
			return errNotFound
		}
		return err
	}
	return ok(w, map[string]bool{"saved": true})
}

func (s *Server) unsaveCompany(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.q.UnsaveCompany(r.Context(), db.UnsaveCompanyParams{UserID: account(r).UserID, CompanyID: id}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"saved": false})
}

func (s *Server) savedCompanies(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.SavedCompanies(r.Context(), account(r).UserID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// GET /v1/jobs/{id}: job detail with the match breakdown (S20) or the closed
// state with similar open jobs (S22). Only an excerpt of the description is
// shown: it is the company's content (spec: Legal and compliance).
func (s *Server) job(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	uid := account(r).UserID
	j, err := s.q.GetJob(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	out := map[string]any{
		"id": j.ID, "title": j.Title, "category": j.Category, "category_2": j.Category2, "seniority": j.Seniority,
		"description_excerpt": Excerpt(j.Description, 900), "cities": j.Cities, "work_mode": j.WorkMode,
		"salary_min": j.SalaryMin, "salary_max": j.SalaryMax, "apply_url": j.ApplyUrl, "posted_at": j.PostedAt,
		"first_seen": j.FirstSeen, "last_verified": j.LastSeen, "closed_at": j.ClosedAt, "status": j.Status,
		"company": map[string]any{"id": j.CompanyID, "name": j.CompanyName, "slug": j.CompanySlug,
			"logo_url": j.CompanyLogo, "type": j.CompanyType},
	}
	if m, err := s.q.GetMatch(r.Context(), db.GetMatchParams{UserID: uid, JobID: id}); err == nil {
		out["match"] = map[string]any{"score": m.Score, "reasons": json.RawMessage(m.Reasons), "computed_at": m.ComputedAt}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if app, err := s.q.GetApplicationForJob(r.Context(), db.GetApplicationForJobParams{UserID: uid, JobID: id}); err == nil {
		out["application"] = map[string]any{"id": app.ID, "status": app.Status, "dont_ask": app.DontAsk}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if j.Status == "closed" {
		similar, err := s.q.SimilarOpenJobs(r.Context(), db.SimilarOpenJobsParams{
			UserID: uid, JobID: id, Category: j.Category, CompanyID: j.CompanyID})
		if err != nil {
			return err
		}
		out["similar"] = similar
	}
	return ok(w, out)
}

// GET /v1/search?q=: companies, jobs and cities in one box (S17).
func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		return ok(w, map[string]any{"companies": []any{}, "jobs": []any{}, "cities": []any{}})
	}
	if len(q) > 80 {
		q = q[:80]
	}
	q = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q)
	companies, err := s.q.SearchCompanies(r.Context(), q)
	if err != nil {
		return err
	}
	jobs, err := s.q.SearchJobs(r.Context(), q)
	if err != nil {
		return err
	}
	cities, err := s.q.SearchCities(r.Context(), q)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"companies": companies, "jobs": jobs, "cities": cities})
}

// POST /v1/reports {company_id?, job_id?, message}: "Report wrong info" (S18).
func (s *Server) report(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		CompanyID *int64 `json:"company_id"`
		JobID     *int64 `json:"job_id"`
		Message   string `json:"message"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	msg := strings.TrimSpace(in.Message)
	if msg == "" || len(msg) > 2000 || (in.CompanyID == nil && in.JobID == nil) {
		return badRequest("Say what is wrong (up to 2000 characters) and about which company or job.")
	}
	uid := account(r).UserID
	if err := s.q.CreateReport(r.Context(), db.CreateReportParams{
		UserID: &uid, CompanyID: in.CompanyID, JobID: in.JobID, Message: msg}); err != nil {
		if isForeignKey(err) {
			return errNotFound
		}
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
	return nil
}

var (
	reTags   = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlocks = regexp.MustCompile(`(?i)</?(p|br|li|h[1-6]|ul|ol|div)[^>]*>`)
	reSpaces = regexp.MustCompile(`[ \t]+`)
	reLines  = regexp.MustCompile(`\n{2,}`)
)

// Excerpt is the description as plain text, cut at a sentence or word near max characters.
func Excerpt(desc *string, max int) string {
	if desc == nil || *desc == "" {
		return ""
	}
	t := reBlocks.ReplaceAllString(*desc, "\n")
	t = html.UnescapeString(reTags.ReplaceAllString(t, ""))
	t = reLines.ReplaceAllString(reSpaces.ReplaceAllString(t, " "), "\n")
	t = strings.TrimSpace(strings.ReplaceAll(t, "\n ", "\n"))
	runes := []rune(t)
	if len(runes) <= max {
		return t
	}
	cut := string(runes[:max])
	if i := strings.LastIndexAny(cut, ".!?\n"); i > max/2 {
		return strings.TrimSpace(cut[:i+1]) + " …"
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + " …"
}
