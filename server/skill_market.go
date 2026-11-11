package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Skill marketplace.
//
// A "source" is a public GitHub repository that holds skills as <dir>/SKILL.md trees (the layout used by the
// official anthropics/skills repository and by most community collections). The server lists them with two
// GitHub API calls per source, reads each SKILL.md's frontmatter from raw.githubusercontent.com, and installs a
// skill by fetching only that skill's files at a pinned commit and feeding them through the same extraction path
// as a manual zip upload (installSkillFromZip). Nothing is installed or updated without an explicit request, and an
// installed skill is NOT visible to any agent until the user enables it per agent (see agent_skill_visibility).

const (
	settingSkillMarketSources = "skill_market_sources"
	settingSkillMarketProxy   = "skill_market_proxy"

	marketIndexTTL       = 15 * time.Minute
	marketMaxSkills      = 400 // per source
	marketMaxFilesPerSk  = 600 // files in a single skill
	marketMaxTreeDepth   = 6   // SKILL.md deeper than this is ignored
	marketMaxMetaBytes   = 64 << 10
	marketFetchWorkers   = 8
	marketDefaultTimeout = 30 * time.Second
)

var (
	marketRepoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}/[A-Za-z0-9._-]{1,100}$`)
	marketRefRe  = regexp.MustCompile(`^[A-Za-z0-9._/-]{0,100}$`)
	marketIDRe   = regexp.MustCompile(`[^a-z0-9]+`)
)

// marketSource is one configured skill repository.
type marketSource struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`            // owner/name on github.com
	Ref     string `json:"ref,omitempty"`   // branch/tag/commit; empty = the repository's default branch
	Label   string `json:"label,omitempty"` // display name
	Builtin bool   `json:"builtin,omitempty"`
}

var defaultMarketSources = []marketSource{
	{ID: "anthropics-skills", Repo: "anthropics/skills", Label: "Anthropic (official)", Builtin: true},
}

func marketSourceID(repo, ref string) string {
	id := strings.Trim(marketIDRe.ReplaceAllString(strings.ToLower(repo), "-"), "-")
	if ref != "" {
		id += "-" + strings.Trim(marketIDRe.ReplaceAllString(strings.ToLower(ref), "-"), "-")
	}
	return id
}

// marketItem is one installable skill in the catalogue.
type marketItem struct {
	Source      string `json:"source"` // source id
	Repo        string `json:"repo"`   // owner/name
	Path        string `json:"path"`   // directory of the skill inside the repository ("" = repo root)
	Name        string `json:"name"`   // frontmatter name (the install directory name)
	Description string `json:"description"`
	License     string `json:"license,omitempty"`
	Commit      string `json:"commit"`  // commit the listing was read at
	Version     string `json:"version"` // tree sha of the skill directory: changes only when the skill's files change
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
	HasScripts  bool   `json:"has_scripts"`
	// Filled per request from the local skills directory / usage ledger:
	Installed       bool   `json:"installed"`
	InstalledFrom   string `json:"installed_from,omitempty"` // "owner/repo:path" when installed through the marketplace
	UpdateAvailable bool   `json:"update_available"`
	Requested       int    `json:"requested"` // times agents asked for a skill with this name and found none
}

type marketTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // blob | tree
	SHA  string `json:"sha"`
	Size int64  `json:"size"`
}

// marketClient talks to GitHub. The base URLs are fields so tests can point it at a fake server; requests (and
// redirects) are only ever allowed to hosts of those two bases — there is no user-supplied URL anywhere.
type marketClient struct {
	http    *http.Client
	apiBase string // https://api.github.com
	rawBase string // https://raw.githubusercontent.com
	token   string // optional GITHUB_TOKEN, sent only to apiBase
}

func newMarketClient(proxy string) *marketClient {
	c := &marketClient{apiBase: "https://api.github.com", rawBase: "https://raw.githubusercontent.com", token: strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))}
	c.http = c.buildHTTP(proxy)
	return c
}

func (c *marketClient) allowedHost(h string) bool {
	for _, b := range []string{c.apiBase, c.rawBase} {
		if u, err := url.Parse(b); err == nil && strings.EqualFold(u.Host, h) {
			return true
		}
	}
	return false
}

func (c *marketClient) buildHTTP(proxy string) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(proxy); p != "" {
		if pu, err := url.Parse(p); err == nil && pu.Host != "" {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &http.Client{
		Timeout:   marketDefaultTimeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("too many redirects")
			}
			if !c.allowedHost(req.URL.Host) {
				return fmt.Errorf("redirect to %s refused", req.URL.Host)
			}
			return nil
		},
	}
}

type marketHTTPError struct {
	Status int
	Msg    string
}

func (e *marketHTTPError) Error() string { return e.Msg }

func (c *marketClient) get(ctx context.Context, base, p string, limit int64, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+p, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", "artex-skill-market")
	if c.token != "" && base == c.apiBase {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, &marketHTTPError{502, "response from GitHub is too large"}
	}
	switch {
	case resp.StatusCode == 200:
		return body, nil
	case resp.StatusCode == 404:
		return nil, &marketHTTPError{404, "not found on GitHub (check the repository name and that it is public)"}
	case resp.StatusCode == 403 || resp.StatusCode == 429:
		// 403 is not always a rate limit (blocked repository, egress policy, ...), so report what GitHub said.
		var gh struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &gh)
		limited := resp.StatusCode == 429 || resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(gh.Message), "rate limit")
		msg := "GitHub refused the request"
		if gh.Message != "" {
			msg += ": " + gh.Message
		}
		if limited {
			msg += ". This looks like a rate limit: wait a few minutes, or start ARTEX with a GITHUB_TOKEN environment variable to raise the limit"
			if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
				msg += " (resets at unix time " + reset + ")"
			}
		}
		return nil, &marketHTTPError{429, msg}
	default:
		return nil, &marketHTTPError{502, fmt.Sprintf("GitHub answered HTTP %d", resp.StatusCode)}
	}
}

// marketRepoSnapshot is what we learned about a repository at one commit.
type marketRepoSnapshot struct {
	Commit    string
	TreeSHA   string
	Entries   []marketTreeEntry
	FetchedAt time.Time
}

func (c *marketClient) snapshot(ctx context.Context, repo, ref string) (*marketRepoSnapshot, error) {
	if ref == "" {
		b, err := c.get(ctx, c.apiBase, "/repos/"+repo, 1<<20, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}
		var meta struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := json.Unmarshal(b, &meta); err != nil || meta.DefaultBranch == "" {
			return nil, errors.New("could not determine the default branch")
		}
		ref = meta.DefaultBranch
	}
	// Plain-text commit sha for the ref, so every later request is pinned to one immutable commit.
	b, err := c.get(ctx, c.apiBase, "/repos/"+repo+"/commits/"+url.PathEscape(ref), 4<<10, "application/vnd.github.sha")
	if err != nil {
		return nil, err
	}
	commit := strings.TrimSpace(string(b))
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(commit) {
		return nil, errors.New("unexpected commit id from GitHub")
	}
	tb, err := c.get(ctx, c.apiBase, "/repos/"+repo+"/git/trees/"+commit+"?recursive=1", 32<<20, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var tree struct {
		SHA       string            `json:"sha"`
		Tree      []marketTreeEntry `json:"tree"`
		Truncated bool              `json:"truncated"`
	}
	if err := json.Unmarshal(tb, &tree); err != nil {
		return nil, fmt.Errorf("bad tree response: %w", err)
	}
	if tree.Truncated {
		return nil, errors.New("the repository is too large to scan (GitHub truncated its file tree)")
	}
	return &marketRepoSnapshot{Commit: commit, TreeSHA: tree.SHA, Entries: tree.Tree, FetchedAt: time.Now()}, nil
}

func (c *marketClient) raw(ctx context.Context, repo, commit, file string, limit int64) ([]byte, error) {
	segs := strings.Split(file, "/")
	for i := range segs {
		segs[i] = url.PathEscape(segs[i])
	}
	return c.get(ctx, c.rawBase, "/"+repo+"/"+commit+"/"+strings.Join(segs, "/"), limit, "")
}

// parseSkillFrontmatter extracts name / description / license from a SKILL.md.
func parseSkillFrontmatter(md []byte) (name, desc, license string) {
	text := strings.ReplaceAll(string(md), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", ""
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", ""
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		License     string `yaml:"license"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		// Malformed YAML: fall back to the line-based name reader the upload path uses.
		return skillNameFromFrontmatter(md), "", ""
	}
	return strings.TrimSpace(fm.Name), strings.Join(strings.Fields(fm.Description), " "), strings.TrimSpace(fm.License)
}

// marketSkillDirs derives the skill directories (and their file lists) from a repository tree.
func marketSkillDirs(entries []marketTreeEntry, rootSHA string) map[string]*marketItem {
	dirSHA := map[string]string{"": rootSHA}
	for _, e := range entries {
		if e.Type == "tree" {
			dirSHA[e.Path] = e.SHA
		}
	}
	items := map[string]*marketItem{}
	for _, e := range entries {
		if e.Type != "blob" || (e.Path != "SKILL.md" && !strings.HasSuffix(e.Path, "/SKILL.md")) {
			continue
		}
		dir := strings.TrimSuffix(strings.TrimSuffix(e.Path, "SKILL.md"), "/")
		if strings.Count(dir, "/") >= marketMaxTreeDepth || strings.Contains("/"+dir+"/", "/node_modules/") || strings.Contains("/"+dir+"/", "/.git/") {
			continue
		}
		items[dir] = &marketItem{Path: dir, Version: dirSHA[dir]}
	}
	// A repository whose root is itself a skill would otherwise swallow every nested skill; keep nesting honest by
	// attributing each file to its deepest skill directory.
	dirs := make([]string, 0, len(items))
	for d := range items {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, e := range entries {
		if e.Type != "blob" {
			continue
		}
		for _, d := range dirs {
			if d == "" || strings.HasPrefix(e.Path, d+"/") {
				it := items[d]
				it.Files++
				it.Bytes += e.Size
				rel := strings.TrimPrefix(strings.TrimPrefix(e.Path, d), "/")
				if strings.HasPrefix(rel, "scripts/") {
					it.HasScripts = true
				}
				break
			}
		}
	}
	return items
}

// ---------------------------------------------------------------- catalogue cache

type marketCacheEntry struct {
	snap  *marketRepoSnapshot
	items []marketItem
	err   error
	at    time.Time
}

type skillMarket struct {
	mu     sync.Mutex
	client *marketClient
	cache  map[string]*marketCacheEntry // key: repo@ref
	proxy  string
}

func (m *skillMarket) clientFor(proxy string) *marketClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		m.client = newMarketClient(proxy)
		m.proxy = proxy
	} else if m.proxy != proxy {
		m.client.http = m.client.buildHTTP(proxy)
		m.proxy = proxy
		m.cache = nil
	}
	return m.client
}

// load returns the (possibly cached) catalogue of one source.
func (m *skillMarket) load(ctx context.Context, c *marketClient, src marketSource, refresh bool) (*marketRepoSnapshot, []marketItem, error) {
	key := src.Repo + "@" + src.Ref
	m.mu.Lock()
	if m.cache == nil {
		m.cache = map[string]*marketCacheEntry{}
	}
	if e := m.cache[key]; e != nil && !refresh && time.Since(e.at) < marketIndexTTL && e.err == nil {
		m.mu.Unlock()
		return e.snap, e.items, nil
	}
	m.mu.Unlock()

	snap, err := c.snapshot(ctx, src.Repo, src.Ref)
	if err != nil {
		return nil, nil, err
	}
	dirs := marketSkillDirs(snap.Entries, snap.TreeSHA)
	if len(dirs) > marketMaxSkills {
		return nil, nil, fmt.Errorf("repository lists %d skills, more than the supported %d", len(dirs), marketMaxSkills)
	}
	paths := make([]string, 0, len(dirs))
	for d := range dirs {
		paths = append(paths, d)
	}
	sort.Strings(paths)

	// Frontmatter of every SKILL.md, a few at a time.
	var wg sync.WaitGroup
	sem := make(chan struct{}, marketFetchWorkers)
	var fetchErr error
	var emu sync.Mutex
	for _, d := range paths {
		it := dirs[d]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			file := "SKILL.md"
			if it.Path != "" {
				file = it.Path + "/SKILL.md"
			}
			b, err := c.raw(ctx, src.Repo, snap.Commit, file, marketMaxMetaBytes)
			if err != nil {
				var he *marketHTTPError
				if errors.As(err, &he) && he.Status == 502 {
					it.Description = "(could not read SKILL.md)"
					return
				}
				emu.Lock()
				if fetchErr == nil {
					fetchErr = err
				}
				emu.Unlock()
				return
			}
			it.Name, it.Description, it.License = parseSkillFrontmatter(b)
		}()
	}
	wg.Wait()
	if fetchErr != nil {
		return nil, nil, fetchErr
	}

	items := make([]marketItem, 0, len(paths))
	for _, d := range paths {
		it := *dirs[d]
		if it.Name == "" {
			it.Name = path.Base(d)
			if d == "" {
				it.Name = path.Base(src.Repo)
			}
		}
		if !validSkillName(it.Name) {
			continue // cannot be installed under this name
		}
		it.Source, it.Repo, it.Commit = src.ID, src.Repo, snap.Commit
		items = append(items, it)
	}
	m.mu.Lock()
	m.cache[key] = &marketCacheEntry{snap: snap, items: items, at: time.Now()}
	m.mu.Unlock()
	return snap, items, nil
}

// ---------------------------------------------------------------- sources setting

func (s *Server) marketSources() []marketSource {
	if s.m.pg == nil {
		return defaultMarketSources
	}
	raw, ok, _ := s.m.pg.GetSetting(settingSkillMarketSources)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultMarketSources
	}
	var out []marketSource
	if json.Unmarshal([]byte(raw), &out) != nil {
		return defaultMarketSources
	}
	return out // an explicitly emptied list stays empty; only a never-configured instance gets the defaults
}

func (s *Server) saveMarketSources(list []marketSource) error {
	b, _ := json.Marshal(list)
	return s.m.pg.SetSetting(settingSkillMarketSources, string(b))
}

func (s *Server) marketProxy() string {
	if s.m.pg == nil {
		return ""
	}
	v, _, _ := s.m.pg.GetSetting(settingSkillMarketProxy)
	return strings.TrimSpace(v)
}

func (s *Server) marketClientForRequest() *marketClient {
	if s.market == nil {
		return newMarketClient(s.marketProxy())
	}
	return s.market.clientFor(s.marketProxy())
}

// ---------------------------------------------------------------- local state

type skillOrigin struct {
	Source      string `json:"source"`
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Commit      string `json:"commit"`
	Version     string `json:"version"`
	InstalledAt string `json:"installed_at"`
}

func (s *Server) readSkillOrigin(name string) *skillOrigin {
	b, err := os.ReadFile(filepath.Join(s.skillDir, name, skillOriginFile))
	if err != nil {
		return nil
	}
	var o skillOrigin
	if json.Unmarshal(b, &o) != nil {
		return nil
	}
	return &o
}

// decorateMarketItems marks installed / updatable skills and attaches agent demand.
func (s *Server) decorateMarketItems(items []marketItem) {
	requested := map[string]int{}
	if s.m.pg != nil {
		if missing, err := s.m.pg.MissingSkillStats(200); err == nil {
			for _, m := range missing {
				requested[strings.ToLower(strings.TrimSpace(m.Skill))] = m.Calls
			}
		}
	}
	for i := range items {
		it := &items[i]
		it.Requested = requested[strings.ToLower(it.Name)]
		if st, err := os.Stat(filepath.Join(s.skillDir, it.Name)); err != nil || !st.IsDir() {
			continue
		}
		it.Installed = true
		if o := s.readSkillOrigin(it.Name); o != nil {
			it.InstalledFrom = o.Repo + ":" + o.Path
			it.UpdateAvailable = o.Repo == it.Repo && o.Path == it.Path && o.Version != "" && o.Version != it.Version
		}
	}
}

// ---------------------------------------------------------------- handlers

func (s *Server) marketListSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"sources": s.marketSources()})
}

func (s *Server) marketAddSource(w http.ResponseWriter, r *http.Request) {
	if s.pg(w) == nil {
		return
	}
	var req struct {
		Repo  string `json:"repo"`
		Ref   string `json:"ref"`
		Label string `json:"label"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	repo := strings.TrimSpace(req.Repo)
	repo = strings.TrimPrefix(strings.TrimPrefix(repo, "https://github.com/"), "http://github.com/")
	repo = strings.TrimSuffix(strings.Trim(repo, "/"), ".git")
	ref := strings.TrimSpace(req.Ref)
	if !marketRepoRe.MatchString(repo) {
		writeErr(w, 400, "repository must look like owner/name (a github.com repository)")
		return
	}
	if !marketRefRe.MatchString(ref) || strings.Contains(ref, "..") {
		writeErr(w, 400, "invalid branch / tag name")
		return
	}
	id := marketSourceID(repo, ref)
	list := s.marketSources()
	for _, e := range list {
		if e.ID == id {
			writeErr(w, 409, "this source is already in the list")
			return
		}
	}
	// Verify the repository is reachable and really contains skills before saving it.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	src := marketSource{ID: id, Repo: repo, Ref: ref, Label: strings.TrimSpace(req.Label)}
	if src.Label == "" {
		src.Label = repo
	}
	_, items, err := (&skillMarket{}).load(ctx, s.marketClientForRequest(), src, true)
	if err != nil {
		writeErr(w, marketErrStatus(err), err.Error())
		return
	}
	if len(items) == 0 {
		writeErr(w, 400, "no skills (SKILL.md files) found in this repository")
		return
	}
	list = append(append([]marketSource{}, list...), src)
	if err := s.saveMarketSources(list); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"source": src, "skills": len(items)})
}

func (s *Server) marketDeleteSource(w http.ResponseWriter, r *http.Request) {
	if s.pg(w) == nil {
		return
	}
	id := r.PathValue("id")
	list := s.marketSources()
	out := make([]marketSource, 0, len(list))
	found := false
	for _, e := range list {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		writeErr(w, 404, "source not found")
		return
	}
	if err := s.saveMarketSources(out); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": id})
}

func marketErrStatus(err error) int {
	var he *marketHTTPError
	if errors.As(err, &he) {
		return he.Status
	}
	return 502
}

// GET /api/skill-market/skills?source=&q=&refresh=true
func (s *Server) marketSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	only := r.URL.Query().Get("source")
	refresh := r.URL.Query().Get("refresh") == "true"
	c := s.marketClientForRequest()
	mk := s.market
	if mk == nil {
		mk = &skillMarket{}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	type srcErr struct {
		Source string `json:"source"`
		Error  string `json:"error"`
	}
	all := []marketItem{}
	errs := []srcErr{}
	for _, src := range s.marketSources() {
		if only != "" && only != src.ID {
			continue
		}
		_, items, err := mk.load(ctx, c, src, refresh)
		if err != nil {
			errs = append(errs, srcErr{src.ID, err.Error()})
			continue
		}
		for _, it := range items {
			if q != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.Description), q) {
				continue
			}
			all = append(all, it)
		}
	}
	s.decorateMarketItems(all)
	// Skills agents keep asking for come first, then alphabetical.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Requested != all[j].Requested {
			return all[i].Requested > all[j].Requested
		}
		return all[i].Name < all[j].Name
	})
	writeJSON(w, 200, map[string]any{"skills": all, "errors": errs})
}

func (s *Server) marketFindSource(id string) (marketSource, bool) {
	for _, src := range s.marketSources() {
		if src.ID == id {
			return src, true
		}
	}
	return marketSource{}, false
}

// marketSkillFiles returns the blob entries that belong to one skill directory (nested skills excluded).
func marketSkillFiles(snap *marketRepoSnapshot, dir string) []marketTreeEntry {
	nested := []string{}
	for _, e := range snap.Entries {
		if e.Type == "blob" && strings.HasSuffix(e.Path, "/SKILL.md") {
			d := strings.TrimSuffix(e.Path, "/SKILL.md")
			if d != dir && (dir == "" || strings.HasPrefix(d, dir+"/")) {
				nested = append(nested, d)
			}
		}
	}
	var out []marketTreeEntry
	for _, e := range snap.Entries {
		if e.Type != "blob" {
			continue
		}
		if dir != "" && !strings.HasPrefix(e.Path, dir+"/") {
			continue
		}
		skip := false
		for _, n := range nested {
			if strings.HasPrefix(e.Path, n+"/") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}

// GET /api/skill-market/preview?source=&path=
func (s *Server) marketPreview(w http.ResponseWriter, r *http.Request) {
	src, ok := s.marketFindSource(r.URL.Query().Get("source"))
	if !ok {
		writeErr(w, 404, "unknown source")
		return
	}
	dir := strings.Trim(r.URL.Query().Get("path"), "/")
	c := s.marketClientForRequest()
	mk := s.market
	if mk == nil {
		mk = &skillMarket{}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	snap, items, err := mk.load(ctx, c, src, false)
	if err != nil {
		writeErr(w, marketErrStatus(err), err.Error())
		return
	}
	var item *marketItem
	for i := range items {
		if items[i].Path == dir {
			item = &items[i]
		}
	}
	if item == nil {
		writeErr(w, 404, "skill not found in this source")
		return
	}
	file := "SKILL.md"
	if dir != "" {
		file = dir + "/SKILL.md"
	}
	md, err := c.raw(ctx, src.Repo, snap.Commit, file, 512<<10)
	if err != nil {
		writeErr(w, marketErrStatus(err), err.Error())
		return
	}
	type fileInfo struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	files := []fileInfo{}
	for _, e := range marketSkillFiles(snap, dir) {
		rel := strings.TrimPrefix(strings.TrimPrefix(e.Path, dir), "/")
		files = append(files, fileInfo{rel, e.Size})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	one := []marketItem{*item}
	s.decorateMarketItems(one)
	writeJSON(w, 200, map[string]any{"skill": one[0], "skill_md": string(md), "files": files})
}

// POST /api/skill-market/install {source, path, overwrite}
func (s *Server) marketInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source    string `json:"source"`
		Path      string `json:"path"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	src, ok := s.marketFindSource(req.Source)
	if !ok {
		writeErr(w, 404, "unknown source")
		return
	}
	dir := strings.Trim(req.Path, "/")
	if dir != "" {
		if clean, msg := skillRelPath(dir); msg != "" || clean != dir {
			writeErr(w, 400, "invalid skill path")
			return
		}
	}
	c := s.marketClientForRequest()
	mk := s.market
	if mk == nil {
		mk = &skillMarket{}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	snap, items, err := mk.load(ctx, c, src, false)
	if err != nil {
		writeErr(w, marketErrStatus(err), err.Error())
		return
	}
	var item *marketItem
	for i := range items {
		if items[i].Path == dir {
			item = &items[i]
		}
	}
	if item == nil {
		writeErr(w, 404, "skill not found in this source (refresh the catalogue and try again)")
		return
	}
	buf, ierr := s.marketBuildArchive(ctx, c, src, snap, item)
	if ierr != nil {
		writeErr(w, ierr.Status, ierr.Msg)
		return
	}
	origin, _ := json.MarshalIndent(skillOrigin{
		Source: src.ID, Repo: src.Repo, Path: item.Path, Commit: snap.Commit, Version: item.Version,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	name, files, ierr := s.installSkillFromZip(buf, skillInstallOpts{
		Overwrite:   req.Overwrite,
		NameHint:    item.Name,
		RequireName: item.Name,
		Origin:      origin,
	})
	if ierr != nil {
		writeErr(w, ierr.Status, ierr.Msg)
		return
	}
	writeJSON(w, 201, map[string]any{"name": name, "files": files, "commit": snap.Commit})
}

// marketBuildArchive downloads one skill's files at the pinned commit and packs them into an in-memory zip with
// SKILL.md at the archive root, so the standard installer can validate and extract it.
func (s *Server) marketBuildArchive(ctx context.Context, c *marketClient, src marketSource, snap *marketRepoSnapshot, item *marketItem) ([]byte, *skillInstallError) {
	files := marketSkillFiles(snap, item.Path)
	if len(files) == 0 {
		return nil, installErr(404, "the skill has no files")
	}
	if len(files) > marketMaxFilesPerSk || len(files) > maxSkillEntries {
		return nil, installErr(400, "the skill has too many files")
	}
	var total int64
	for _, f := range files {
		if f.Size > maxSkillFileBytes {
			return nil, installErr(400, "file too large: "+f.Path)
		}
		total += f.Size
	}
	if total > maxSkillZipBytes {
		return nil, installErr(400, fmt.Sprintf("the skill is %d MB, larger than the %d MB install limit", total>>20, maxSkillZipBytes>>20))
	}

	type result struct {
		data []byte
		err  error
	}
	results := make([]result, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, marketFetchWorkers)
	for i, f := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			data, err := c.raw(ctx, src.Repo, snap.Commit, f.Path, maxSkillFileBytes)
			results[i] = result{data, err}
		}()
	}
	wg.Wait()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, f := range files {
		if results[i].err != nil {
			return nil, installErr(marketErrStatus(results[i].err), "download of "+f.Path+" failed: "+results[i].err.Error())
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(f.Path, item.Path), "/")
		w, err := zw.Create(rel)
		if err != nil {
			return nil, installErr(500, err.Error())
		}
		if _, err := w.Write(results[i].data); err != nil {
			return nil, installErr(500, err.Error())
		}
	}
	if err := zw.Close(); err != nil {
		return nil, installErr(500, err.Error())
	}
	return buf.Bytes(), nil
}
