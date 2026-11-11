package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const fakeCommit = "0123456789abcdef0123456789abcdef01234567"

// fakeGitHub serves just enough of the GitHub API + raw host for the marketplace.
type fakeGitHub struct {
	*httptest.Server
	pdfTreeSHA atomic.Value // string; flip it to simulate an upstream change
	limited    atomic.Bool
	blocked    atomic.Bool
	apiHits    atomic.Int32
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{}
	f.pdfTreeSHA.Store("aaaa")
	files := map[string]string{
		"skills/pdf/SKILL.md":       "---\nname: pdf\ndescription: >-\n  Work with PDF files\n  and forms.\nlicense: MIT\n---\n# PDF\nUse scripts/run.py\n",
		"skills/pdf/reference.md":   "ref",
		"skills/pdf/scripts/run.py": "print('hi')",
		"skills/bad/SKILL.md":       "---\nname: Bad Name!\ndescription: invalid install name\n---\n",
		"README.md":                 "readme",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/anthropics/skills", func(w http.ResponseWriter, r *http.Request) {
		f.apiHits.Add(1)
		if f.blocked.Load() {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"message":"repository blocked by policy"}`))
			return
		}
		if f.limited.Load() {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	})
	mux.HandleFunc("/repos/anthropics/skills/commits/main", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fakeCommit))
	})
	mux.HandleFunc("/repos/anthropics/skills/git/trees/"+fakeCommit, func(w http.ResponseWriter, r *http.Request) {
		tree := []map[string]any{
			{"path": "skills", "type": "tree", "sha": "t0"},
			{"path": "skills/pdf", "type": "tree", "sha": f.pdfTreeSHA.Load().(string)},
			{"path": "skills/bad", "type": "tree", "sha": "tbad"},
		}
		for p, c := range files {
			tree = append(tree, map[string]any{"path": p, "type": "blob", "sha": "b", "size": len(c)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sha": "root", "tree": tree, "truncated": false})
	})
	mux.HandleFunc("/anthropics/skills/"+fakeCommit+"/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/anthropics/skills/"+fakeCommit+"/")
		if c, ok := files[p]; ok {
			_, _ = w.Write([]byte(c))
			return
		}
		w.WriteHeader(404)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/elsewhere", 302)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func newMarketServer(t *testing.T, f *fakeGitHub) *Server {
	c := &marketClient{apiBase: f.URL, rawBase: f.URL}
	c.http = c.buildHTTP("")
	return &Server{m: &Manager{}, skillDir: t.TempDir(), market: &skillMarket{client: c}}
}

func marketDo(t *testing.T, h http.HandlerFunc, method, target string, body any) (int, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	rec := httptest.NewRecorder()
	h(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestParseSkillFrontmatter(t *testing.T) {
	n, d, l := parseSkillFrontmatter([]byte("---\r\nname: pdf\r\ndescription: >-\r\n  a\r\n  b\r\nlicense: MIT\r\n---\r\nbody"))
	if n != "pdf" || d != "a b" || l != "MIT" {
		t.Fatalf("got %q %q %q", n, d, l)
	}
	if n, _, _ := parseSkillFrontmatter([]byte("no frontmatter")); n != "" {
		t.Fatalf("expected empty, got %q", n)
	}
	// malformed YAML falls back to the line reader for the name
	if n, _, _ := parseSkillFrontmatter([]byte("---\nname: ok\ndescription: [unclosed\n---\n")); n != "ok" {
		t.Fatalf("fallback name = %q", n)
	}
}

func TestMarketSourceID(t *testing.T) {
	if got := marketSourceID("Anthropics/Skills", ""); got != "anthropics-skills" {
		t.Fatal(got)
	}
	if got := marketSourceID("a/b", "feature/x"); got != "a-b-feature-x" {
		t.Fatal(got)
	}
}

func TestMarketSearchPreviewInstall(t *testing.T) {
	f := newFakeGitHub(t)
	s := newMarketServer(t, f)

	// catalogue: the invalid-name skill is hidden, descriptions are parsed
	code, out := marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills", nil)
	if code != 200 {
		t.Fatalf("search = %d %v", code, out)
	}
	items := out["skills"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 skill, got %d: %v", len(items), items)
	}
	it := items[0].(map[string]any)
	if it["name"] != "pdf" || it["description"] != "Work with PDF files and forms." || it["has_scripts"] != true || it["version"] != "aaaa" || it["installed"] != false {
		t.Fatalf("item = %v", it)
	}
	if code, out := marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills?q=nomatch", nil); code != 200 || len(out["skills"].([]any)) != 0 {
		t.Fatalf("query filter failed: %v", out)
	}
	hits := f.apiHits.Load()
	marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills", nil)
	if f.apiHits.Load() != hits {
		t.Error("second search should be served from cache")
	}

	// preview
	code, out = marketDo(t, s.marketPreview, "GET", "/api/skill-market/preview?source=anthropics-skills&path=skills/pdf", nil)
	if code != 200 || !strings.Contains(out["skill_md"].(string), "# PDF") || len(out["files"].([]any)) != 3 {
		t.Fatalf("preview = %d %v", code, out)
	}

	// install
	body := map[string]any{"source": "anthropics-skills", "path": "skills/pdf"}
	code, out = marketDo(t, s.marketInstall, "POST", "/api/skill-market/install", body)
	if code != 201 || out["name"] != "pdf" {
		t.Fatalf("install = %d %v", code, out)
	}
	for _, p := range []string{"SKILL.md", "reference.md", "scripts/run.py", skillOriginFile} {
		if _, err := os.Stat(filepath.Join(s.skillDir, "pdf", p)); err != nil {
			t.Errorf("missing installed file %s", p)
		}
	}
	origin := s.readSkillOrigin("pdf")
	if origin == nil || origin.Commit != fakeCommit || origin.Path != "skills/pdf" || origin.Version != "aaaa" {
		t.Fatalf("origin = %+v", origin)
	}
	// no visibility is granted by installing: the installer never touches the database
	if code, _ = marketDo(t, s.marketInstall, "POST", "/api/skill-market/install", body); code != 409 {
		t.Fatalf("second install = %d, want 409", code)
	}
	body["overwrite"] = true
	if code, out = marketDo(t, s.marketInstall, "POST", "/api/skill-market/install", body); code != 201 {
		t.Fatalf("overwrite = %d %v", code, out)
	}
	if entries, _ := os.ReadDir(s.skillDir); len(entries) != 1 {
		t.Errorf("scratch dirs left behind: %v", entries)
	}

	// installed + up to date, then upstream changes
	_, out = marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills", nil)
	it = out["skills"].([]any)[0].(map[string]any)
	if it["installed"] != true || it["update_available"] != false || it["installed_from"] != "anthropics/skills:skills/pdf" {
		t.Fatalf("installed state = %v", it)
	}
	f.pdfTreeSHA.Store("bbbb")
	_, out = marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills?refresh=true", nil)
	it = out["skills"].([]any)[0].(map[string]any)
	if it["update_available"] != true {
		t.Fatalf("update should be available: %v", it)
	}

	// the skill list shows the skill but never scratch space
	_ = os.MkdirAll(filepath.Join(s.skillDir, ".bak-pdf-123"), 0o755)
	rec := httptest.NewRecorder()
	s.fsListSkills(rec, httptest.NewRequest("GET", "/api/skills", nil))
	if strings.Contains(rec.Body.String(), ".bak-") {
		t.Errorf("dot-directory leaked into the skill list: %s", rec.Body.String())
	}
}

func TestMarketInstallRejects(t *testing.T) {
	f := newFakeGitHub(t)
	s := newMarketServer(t, f)
	for _, tc := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"unknown source", map[string]any{"source": "nope", "path": "skills/pdf"}, 404},
		{"traversal", map[string]any{"source": "anthropics-skills", "path": "../etc"}, 400},
		{"not in catalogue", map[string]any{"source": "anthropics-skills", "path": "skills/ghost"}, 404},
		{"invalid-name skill is not installable", map[string]any{"source": "anthropics-skills", "path": "skills/bad"}, 404},
	} {
		if code, out := marketDo(t, s.marketInstall, "POST", "/api/skill-market/install", tc.body); code != tc.want {
			t.Errorf("%s: got %d %v, want %d", tc.name, code, out, tc.want)
		}
	}
	if entries, _ := os.ReadDir(s.skillDir); len(entries) != 0 {
		t.Errorf("failed installs left files: %v", entries)
	}
}

func TestMarketRateLimitAndRedirect(t *testing.T) {
	f := newFakeGitHub(t)
	s := newMarketServer(t, f)
	f.limited.Store(true)
	code, out := marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills", nil)
	if code != 200 {
		t.Fatal(code)
	}
	errs := out["errors"].([]any)
	if len(errs) != 1 || !strings.Contains(errs[0].(map[string]any)["error"].(string), "GITHUB_TOKEN") {
		t.Fatalf("rate limit should be reported per source with a hint: %v", out)
	}

	// a 403 that is not a rate limit surfaces GitHub's own message and no token hint
	f.limited.Store(false)
	f.blocked.Store(true)
	_, out = marketDo(t, s.marketSearch, "GET", "/api/skill-market/skills?refresh=true", nil)
	msg := out["errors"].([]any)[0].(map[string]any)["error"].(string)
	if !strings.Contains(msg, "blocked by policy") || strings.Contains(msg, "GITHUB_TOKEN") {
		t.Fatalf("plain 403 message = %q", msg)
	}

	// redirects off the allowed hosts are refused
	c := s.market.client
	if _, err := c.get(t.Context(), c.apiBase, "/redirect", 1024, ""); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("redirect to another host must be refused, got %v", err)
	}
}

func TestMarketAddSourceValidation(t *testing.T) {
	f := newFakeGitHub(t)
	s := newMarketServer(t, f)
	// no database in this fixture: the handler answers 503 before touching anything
	code, _ := marketDo(t, s.marketAddSource, "POST", "/api/skill-market/sources", map[string]any{"repo": "x/y"})
	if code != 503 {
		t.Fatalf("want 503 without a database, got %d", code)
	}
	for _, bad := range []string{"", "no-slash", "a/b/c", "../x", "a/b c", "https://evil.example/a/b"} {
		if marketRepoRe.MatchString(bad) {
			t.Errorf("%q must not be a valid repo", bad)
		}
	}
	if !marketRepoRe.MatchString("anthropics/skills") {
		t.Error("valid repo rejected")
	}
}
