package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify"
)

// Configuration export / import.
//
// The bundle is a plain JSON document holding the instance's *configuration* (never task data):
// global settings, LLM profiles (with their API keys), the LLM retry policy, MCP servers and notification
// channels. It is meant for moving a setup between instances or keeping a backup, so — by explicit product
// decision — secrets are written in plaintext. The export endpoint is behind the normal admin auth and the
// UI puts a warning in front of it.
//
// Deliberately NOT part of a bundle: the admin password hash, the JWT signing key, the database DSN / config.json,
// and one-shot migration flags. Browser-only preferences (theme, layout) are appended by the web client.

const (
	configBundleFormat   = "artex-config"
	configBundleVersion  = 1
	maxConfigBundleBytes = 5 << 20 // 5 MB

	cfgSecSettings = "settings"
	cfgSecLLM      = "llm"
	cfgSecMCP      = "mcp"
	cfgSecNotify   = "notify"
)

var configSections = []string{cfgSecSettings, cfgSecLLM, cfgSecMCP, cfgSecNotify}

// configSettingKeys are the settings a bundle may carry — exactly the writable fields of PUT /api/settings,
// minus machine-specific ones (python_interpreter is an absolute path on the source host).
var configSettingKeys = []string{
	"traffic_capture", "agent_traffic_binding", "llm_record",
	"web_search_enabled", "web_search_backend", "brave_search_api_key", "tavily_search_api_key", "web_search_proxy",
	"global_proxy", "workers",
	"task_concurrency_enabled", "task_concurrency_limit",
	"llm_pool_enabled", "llm_pool_bind_fallback",
	"constraints_inject_planner", "constraints_inject_worker",
	"noa_compaction",
	"notify_enabled", "notify_public_base_url", "notify_digest_interval_min", "notify_summary_max_runes", "notify_assets_max_shown",
}

// configSecretSettingKeys are echoed as part of the bundle only because the user asked for secrets.
var configSecretSettingKeys = map[string]bool{"brave_search_api_key": true, "tavily_search_api_key": true}

// configProfile is an LLM profile as it travels in a bundle: db.LLMProfile never serializes the key, so the key
// is carried by a sibling field, and Streaming is a pointer so an absent value defaults to streaming.
type configProfile struct {
	db.LLMProfile
	APIKey    string `json:"api_key"`
	Streaming *bool  `json:"streaming"`
}

type configMCP struct {
	Name           string          `json:"name"`
	Transport      string          `json:"transport"`
	Command        string          `json:"command,omitempty"`
	Args           json.RawMessage `json:"args,omitempty"`
	Env            json.RawMessage `json:"env,omitempty"`
	URL            string          `json:"url,omitempty"`
	Enabled        bool            `json:"enabled"`
	Insecure       bool            `json:"insecure"`
	TimeoutSeconds int             `json:"timeout_seconds"`
}

type configChannel struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Enabled    bool           `json:"enabled"`
	Config     map[string]any `json:"config"`
	Mode       string         `json:"mode"`
	Filter     *notify.Filter `json:"filter,omitempty"`
	RatePerMin int            `json:"rate_per_min"`
}

type configBundle struct {
	Format          string             `json:"format"`
	Version         int                `json:"version"`
	ExportedAt      string             `json:"exported_at,omitempty"`
	IncludesSecrets bool               `json:"includes_secrets"`
	Settings        map[string]any     `json:"settings,omitempty"`
	LLMRetryPolicy  *db.LLMRetryPolicy `json:"llm_retry_policy,omitempty"`
	LLMProfiles     []configProfile    `json:"llm_profiles,omitempty"`
	MCPServers      []configMCP        `json:"mcp_servers,omitempty"`
	NotifyChannels  []configChannel    `json:"notify_channels,omitempty"`
	Preferences     map[string]string  `json:"preferences,omitempty"` // browser-only; written and applied by the web client
}

// configImportSection is the per-section outcome of an import (or its dry run).
type configImportSection struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

func (c *configImportSection) fail(format string, a ...any) {
	c.Skipped++
	c.Errors = append(c.Errors, fmt.Sprintf(format, a...))
}

type configImportResult struct {
	DryRun   bool                            `json:"dry_run"`
	Sections map[string]*configImportSection `json:"sections"`
}

func parseConfigSections(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return configSections, nil
	}
	valid := map[string]bool{}
	for _, s := range configSections {
		valid[s] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		if !valid[part] {
			return nil, fmt.Errorf("unknown section %q (available: %s)", part, strings.Join(configSections, ", "))
		}
		seen[part] = true
		out = append(out, part)
	}
	if len(out) == 0 {
		return configSections, nil
	}
	return out, nil
}

// buildConfigBundle reads the requested sections from the database.
func (s *Server) buildConfigBundle(sections []string) (*configBundle, error) {
	pg := s.m.pg
	b := &configBundle{
		Format:          configBundleFormat,
		Version:         configBundleVersion,
		ExportedAt:      time.Now().UTC().Format(time.RFC3339),
		IncludesSecrets: true,
	}
	for _, sec := range sections {
		switch sec {
		case cfgSecSettings:
			payload := s.settingsPayload()
			_, _, brave, tavily, _ := s.m.WebSearch()
			payload["brave_search_api_key"] = strings.TrimSpace(brave)
			payload["tavily_search_api_key"] = strings.TrimSpace(tavily)
			b.Settings = map[string]any{}
			for _, k := range configSettingKeys {
				v, ok := payload[k]
				if !ok {
					continue
				}
				// An empty secret means "not configured"; omitting it keeps import from clearing the target's key.
				if str, isStr := v.(string); isStr && configSecretSettingKeys[k] && str == "" {
					continue
				}
				b.Settings[k] = v
			}
		case cfgSecLLM:
			ps, err := pg.ListProfilesWithKeys()
			if err != nil {
				return nil, fmt.Errorf("read LLM profiles: %w", err)
			}
			for _, p := range ps {
				streaming := p.Streaming
				cp := configProfile{LLMProfile: *p, APIKey: p.APIKey, Streaming: &streaming}
				cp.ID = 0
				cp.APIKeyHint = ""
				b.LLMProfiles = append(b.LLMProfiles, cp)
			}
			pol := pg.LLMRetryPolicy()
			b.LLMRetryPolicy = &pol
		case cfgSecMCP:
			ms, err := pg.ListMCP()
			if err != nil {
				return nil, fmt.Errorf("read MCP servers: %w", err)
			}
			for _, m := range ms {
				b.MCPServers = append(b.MCPServers, configMCP{
					Name: m.Name, Transport: m.Transport, Command: m.Command, Args: m.Args, Env: m.Env, URL: m.URL,
					Enabled: m.Enabled, Insecure: m.Insecure, TimeoutSeconds: m.TimeoutSeconds,
				})
			}
		case cfgSecNotify:
			cs, err := pg.ListNotificationChannels(context.Background())
			if err != nil {
				return nil, fmt.Errorf("read notification channels: %w", err)
			}
			for _, c := range cs {
				cc := configChannel{Name: c.Name, Kind: c.Kind, Enabled: c.IsEnabled(), Mode: c.Mode, RatePerMin: c.RatePerMin}
				_ = json.Unmarshal(c.Config, &cc.Config)
				if len(c.Filter) > 0 {
					var f notify.Filter
					if json.Unmarshal(c.Filter, &f) == nil {
						cc.Filter = &f
					}
				}
				b.NotifyChannels = append(b.NotifyChannels, cc)
			}
		}
	}
	return b, nil
}

// GET /api/config/export?sections=settings,llm,mcp,notify
func (s *Server) configExport(w http.ResponseWriter, r *http.Request) {
	if s.pg(w) == nil {
		return
	}
	sections, err := parseConfigSections(r.URL.Query().Get("sections"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	b, err := s.buildConfigBundle(sections)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Audit line: what was exported, never the values.
	log.Printf("[config] export with secrets requested: sections=%s remote=%s", strings.Join(sections, ","), r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="artex-config-%s.json"`, time.Now().UTC().Format("20060102-150405")))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(b)
}

// POST /api/config/import?dry_run=true|false — body is a bundle. Always a merge: existing entries with the same
// natural key are updated, nothing is ever deleted.
func (s *Server) configImport(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBundleBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, 400, "bundle is unreadable or larger than 5 MB")
		return
	}
	var b configBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		writeErr(w, 400, "bundle is not valid JSON: "+err.Error())
		return
	}
	if b.Format != configBundleFormat {
		writeErr(w, 400, fmt.Sprintf("not an ARTEX config bundle (format %q)", b.Format))
		return
	}
	if b.Version < 1 || b.Version > configBundleVersion {
		writeErr(w, 400, fmt.Sprintf("unsupported bundle version %d (this server understands up to %d)", b.Version, configBundleVersion))
		return
	}
	dry := r.URL.Query().Get("dry_run") == "true"
	res := &configImportResult{DryRun: dry, Sections: map[string]*configImportSection{}}

	if b.Settings != nil {
		res.Sections[cfgSecSettings] = s.importSettings(r.Context(), b.Settings, dry)
	}
	if len(b.LLMProfiles) > 0 || b.LLMRetryPolicy != nil {
		res.Sections[cfgSecLLM] = s.importLLM(&b, dry)
	}
	if len(b.MCPServers) > 0 {
		res.Sections[cfgSecMCP] = s.importMCP(b.MCPServers, dry)
	}
	if len(b.NotifyChannels) > 0 {
		res.Sections[cfgSecNotify] = s.importNotify(r.Context(), b.NotifyChannels, dry)
	}
	if !dry {
		log.Printf("[config] import applied: %s", summarizeImport(res))
	}
	writeJSON(w, 200, res)
}

func summarizeImport(res *configImportResult) string {
	names := make([]string, 0, len(res.Sections))
	for k := range res.Sections {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, k := range names {
		sec := res.Sections[k]
		parts = append(parts, fmt.Sprintf("%s(+%d ~%d skip %d)", k, sec.Created, sec.Updated, sec.Skipped))
	}
	return strings.Join(parts, " ")
}

// memResponse is a minimal in-memory http.ResponseWriter, used to replay settings through the PUT /api/settings
// handler so an import gets exactly the same validation and side effects (agent rebuild, pool refresh) as the UI.
type memResponse struct {
	h    http.Header
	code int
	body bytes.Buffer
}

func (m *memResponse) Header() http.Header {
	if m.h == nil {
		m.h = http.Header{}
	}
	return m.h
}
func (m *memResponse) Write(p []byte) (int, error) { return m.body.Write(p) }
func (m *memResponse) WriteHeader(c int)           { m.code = c }

func (s *Server) importSettings(ctx context.Context, in map[string]any, dry bool) *configImportSection {
	sec := &configImportSection{}
	allowed := map[string]bool{}
	for _, k := range configSettingKeys {
		allowed[k] = true
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	apply := map[string]any{}
	for _, k := range keys {
		if !allowed[k] {
			sec.fail("setting %q is not importable, skipped", k)
			continue
		}
		apply[k] = in[k]
	}
	if len(apply) == 0 || dry {
		sec.Updated = len(apply)
		return sec
	}
	body, _ := json.Marshal(apply)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "/api/settings", bytes.NewReader(body))
	if err != nil {
		sec.fail("internal error: %v", err)
		return sec
	}
	rec := &memResponse{}
	s.putSettings(rec, req)
	if rec.code != 0 && rec.code != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.body.Bytes(), &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("HTTP %d", rec.code)
		}
		// putSettings applies fields in order and may have stopped midway; say so rather than pretend.
		sec.Errors = append(sec.Errors, "settings were only partially applied: "+e.Error)
		sec.Skipped += len(apply)
		return sec
	}
	sec.Updated = len(apply)
	return sec
}

var validProfileFormats = map[string]bool{"openai": true, "anthropic": true, "openai-responses": true, "ollama": true}

func (s *Server) importLLM(b *configBundle, dry bool) *configImportSection {
	sec := &configImportSection{}
	pg := s.m.pg
	existing, err := pg.ListProfilesWithKeys()
	if err != nil {
		sec.fail("read existing profiles: %v", err)
		return sec
	}
	byName := map[string]*db.LLMProfile{}
	hadActive := false
	for _, p := range existing {
		byName[p.Name] = p
		hadActive = hadActive || p.IsDefault
	}
	changed := false
	var activate string
	for i := range b.LLMProfiles {
		cp := b.LLMProfiles[i]
		p := cp.LLMProfile
		p.Name = strings.TrimSpace(p.Name)
		p.APIKey = strings.TrimSpace(cp.APIKey)
		p.Streaming = cp.Streaming == nil || *cp.Streaming
		if p.Name == "" {
			sec.fail("profile #%d has no name, skipped", i+1)
			continue
		}
		if !validProfileFormats[p.Format] {
			sec.fail("profile %q: unknown format %q, skipped", p.Name, p.Format)
			continue
		}
		if strings.TrimSpace(p.Model) == "" {
			sec.fail("profile %q has no model, skipped", p.Name)
			continue
		}
		if err := normalizeProfileForSave(&p); err != nil {
			sec.fail("profile %q: %v", p.Name, err)
			continue
		}
		cur := byName[p.Name]
		p.ID = 0
		if cur != nil {
			p.ID = cur.ID
		}
		if cp.IsDefault {
			activate = p.Name
		}
		if dry {
			if cur != nil {
				sec.Updated++
			} else {
				sec.Created++
			}
			continue
		}
		id, err := pg.SaveProfile(&p)
		if err != nil {
			sec.fail("profile %q: %v", p.Name, err)
			continue
		}
		changed = true
		if cur != nil {
			sec.Updated++
		} else {
			sec.Created++
			np := p
			np.ID = id
			byName[p.Name] = &np
		}
	}
	// The local active profile is only replaced when there is none yet; importing must not silently re-route a
	// running instance to a different model.
	if !dry && !hadActive && activate != "" {
		if t := byName[activate]; t != nil && t.ID != 0 {
			if err := pg.SetActiveProfile(t.ID); err != nil {
				sec.fail("activate profile %q: %v", activate, err)
			} else {
				changed = true
			}
		}
	}
	if b.LLMRetryPolicy != nil {
		if dry {
			sec.Updated++
		} else if err := pg.SetLLMRetryPolicy(*b.LLMRetryPolicy); err != nil {
			sec.fail("retry policy: %v", err)
		} else {
			s.applyRetryPolicy()
			changed = true
			sec.Updated++
		}
	}
	if changed {
		s.invalidateProfileAgents()
		s.reapplyActiveProfile()
	}
	return sec
}

func (s *Server) importMCP(in []configMCP, dry bool) *configImportSection {
	sec := &configImportSection{}
	pg := s.m.pg
	existing, err := pg.ListMCP()
	if err != nil {
		sec.fail("read existing MCP servers: %v", err)
		return sec
	}
	byName := map[string]*db.MCPServer{}
	for _, m := range existing {
		byName[m.Name] = m
	}
	for i, c := range in {
		name := strings.TrimSpace(c.Name)
		switch {
		case name == "":
			sec.fail("MCP server #%d has no name, skipped", i+1)
			continue
		case c.Transport != "stdio" && c.Transport != "http" && c.Transport != "sse":
			sec.fail("MCP server %q: unknown transport %q, skipped", name, c.Transport)
			continue
		case c.Transport == "stdio" && strings.TrimSpace(c.Command) == "":
			sec.fail("MCP server %q: stdio transport needs a command, skipped", name)
			continue
		case c.Transport != "stdio" && strings.TrimSpace(c.URL) == "" && name != "ScopeSentry":
			sec.fail("MCP server %q: %s transport needs a url, skipped", name, c.Transport)
			continue
		}
		m := &db.MCPServer{
			Name: name, Transport: c.Transport, Command: c.Command, Args: c.Args, Env: c.Env, URL: c.URL,
			Enabled: c.Enabled, Insecure: c.Insecure, TimeoutSeconds: c.TimeoutSeconds,
		}
		cur := byName[name]
		if cur != nil {
			m.ID = cur.ID
		}
		if dry {
			if cur != nil {
				sec.Updated++
			} else {
				sec.Created++
			}
			continue
		}
		id, err := pg.SaveMCP(m)
		if err != nil {
			sec.fail("MCP server %q: %v", name, err)
			continue
		}
		if cur != nil {
			sec.Updated++
			continue
		}
		sec.Created++
		// New enabled servers get their tool list cached in the background (same as adding one in the UI),
		// so a slow or broken server cannot stall the import.
		if m.Enabled {
			m.ID = id
			go func(m db.MCPServer) {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				if err := s.discoverAndCacheMCP(ctx, &m); err != nil {
					log.Printf("[mcp] tool discovery after import of %s failed: %v", m.Name, err)
				}
			}(*m)
		}
	}
	return sec
}

func (s *Server) importNotify(ctx context.Context, in []configChannel, dry bool) *configImportSection {
	sec := &configImportSection{}
	pg := s.m.pg
	existing, err := pg.ListNotificationChannels(ctx)
	if err != nil {
		sec.fail("read existing channels: %v", err)
		return sec
	}
	byName := map[string]*db.NotificationChannel{}
	for _, c := range existing {
		byName[c.Name] = c
	}
	for i, c := range in {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			sec.fail("channel #%d has no name, skipped", i+1)
			continue
		}
		if !notify.ValidKind(c.Kind) {
			sec.fail("channel %q: unknown type %q, skipped", name, c.Kind)
			continue
		}
		impl, _ := notify.Get(c.Kind)
		cfg := c.Config
		if cfg == nil {
			cfg = map[string]any{}
		}
		cur := byName[name]
		// A masked placeholder means "keep the stored secret". That is only possible when updating an existing
		// channel of the same kind; otherwise the bundle simply does not carry the secret.
		for k, v := range cfg {
			str, ok := v.(string)
			if !ok || !notify.IsMasked(str) {
				continue
			}
			if cur == nil || cur.Kind != c.Kind {
				sec.fail("channel %q: field %q is masked and there is no stored value to keep, skipped", name, k)
				cfg = nil
				break
			}
			var stored map[string]any
			_ = json.Unmarshal(cur.Config, &stored)
			cfg[k] = stored[k]
		}
		if cfg == nil {
			continue
		}
		if err := impl.Validate(cfg); err != nil {
			sec.fail("channel %q: %v", name, err)
			continue
		}
		mode := c.Mode
		if mode == "" {
			mode = db.NotifyModeRealtime
		}
		if !db.ValidNotifyMode(mode) {
			sec.fail("channel %q: invalid delivery mode %q, skipped", name, mode)
			continue
		}
		if c.RatePerMin < 0 {
			sec.fail("channel %q: rate limit must not be negative, skipped", name)
			continue
		}
		ch := &db.NotificationChannel{Name: name, Kind: c.Kind, Mode: mode, RatePerMin: c.RatePerMin}
		enabled := c.Enabled
		ch.Enabled = &enabled
		if c.Filter != nil {
			if err := c.Filter.Validate(); err != nil {
				sec.fail("channel %q: %v", name, err)
				continue
			}
			ch.Filter, _ = json.Marshal(c.Filter)
		}
		ch.Config, _ = json.Marshal(cfg)
		if cur != nil {
			ch.ID = cur.ID
		}
		if dry {
			if cur != nil {
				sec.Updated++
			} else {
				sec.Created++
			}
			continue
		}
		if _, err := pg.SaveNotificationChannel(ctx, ch); err != nil {
			sec.fail("channel %q: %v", name, err)
			continue
		}
		if cur != nil {
			sec.Updated++
		} else {
			sec.Created++
		}
	}
	return sec
}
