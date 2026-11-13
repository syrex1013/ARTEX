package server

// Sliver C2 integration (https://sliver.sh). ARTEX acts as a Sliver operator:
// the operator config produced by `sliver new-operator --name artex` (or the
// client config saved by any Sliver console) is stored server-side, and the
// built-in gRPC client (bishopfox/sliver client/transport) connects to the
// Sliver server. Supported operations: list sessions/beacons, list/create/kill
// listeners (jobs), run a command in an interactive session, and submit a
// command to a beacon (poll the task list for its output). Implant generation
// stays in the Sliver console for now — it needs the full ImplantConfig
// (obfuscation, C2 profiles, builds) that deserves its own UI.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bishopfox/sliver/client/assets"
	"github.com/bishopfox/sliver/client/transport"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	clientpb "github.com/bishopfox/sliver/protobuf/clientpb"
	commonpb "github.com/bishopfox/sliver/protobuf/commonpb"
	sliverpb "github.com/bishopfox/sliver/protobuf/sliverpb"
	"google.golang.org/grpc"

	"github.com/Autumn-27/artex/db"
)

const settingSliverOperatorConfig = "sliver.operator_config"

// sliverCommandTimeout bounds one session command; beacons poll on their own
// interval so their tasks are submitted asynchronously by design.
const sliverCommandTimeout = 60 * time.Second

type sliverManager struct {
	pg  *db.DB
	mu  sync.Mutex
	cfg *assets.ClientConfig
	rpc rpcpb.SliverRPCClient
	conn *grpc.ClientConn
	err  string // last connect error, surfaced by GET /api/sliver
}

func newSliverManager(pg *db.DB) *sliverManager {
	sm := &sliverManager{pg: pg}
	if raw, ok, _ := pg.GetSetting(settingSliverOperatorConfig); ok && strings.TrimSpace(raw) != "" {
		if cfg, err := parseSliverConfig(raw); err != nil {
			sm.err = "stored operator config is invalid: " + err.Error()
		} else {
			sm.cfg = cfg
			sm.connect()
		}
	}
	return sm
}

// parseSliverConfig accepts the operator config JSON as saved by the Sliver
// client (clients/<operator>_<lhost>.cfg) — the same shape assets.ClientConfig
// marshals to.
func parseSliverConfig(raw string) (*assets.ClientConfig, error) {
	cfg := &assets.ClientConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.LHost) == "" || cfg.LPort == 0 {
		return nil, fmt.Errorf("lhost and lport are required")
	}
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.CACertificate) == "" ||
		strings.TrimSpace(cfg.Certificate) == "" || strings.TrimSpace(cfg.PrivateKey) == "" {
		return nil, fmt.Errorf("operator config is missing token/certificate fields")
	}
	return cfg, nil
}

// connect dials the Sliver server; a failure is recorded, not fatal — handlers
// retry lazily on the next call.
func (sm *sliverManager) connect() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.closeLocked()
	sm.err = ""
	rpc, conn, err := transport.MTLSConnect(sm.cfg)
	if err != nil {
		sm.err = err.Error()
		log.Printf("[sliver] connect to %s:%d failed: %v", sm.cfg.LHost, sm.cfg.LPort, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := rpc.GetVersion(ctx, &commonpb.Empty{}); err != nil {
		_ = conn.Close()
		sm.err = "connected but version check failed: " + err.Error()
		return
	}
	sm.rpc, sm.conn, sm.err = rpc, conn, ""
	log.Printf("[sliver] connected to %s:%d", sm.cfg.LHost, sm.cfg.LPort)
}

func (sm *sliverManager) closeLocked() {
	if sm.conn != nil {
		_ = sm.conn.Close()
	}
	sm.rpc, sm.conn = nil, nil
}

// client returns a live RPC client, reconnecting once if the cached one died.
func (sm *sliverManager) client() (rpcpb.SliverRPCClient, error) {
	sm.mu.Lock()
	cfg := sm.cfg
	rpc, lastErr := sm.rpc, sm.err
	sm.mu.Unlock()
	if cfg == nil {
		if lastErr != "" {
			return nil, fmt.Errorf("sliver operator config is invalid: %s", lastErr)
		}
		return nil, fmt.Errorf("no sliver operator config saved; add one first")
	}
	if rpc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := rpc.GetVersion(ctx, &commonpb.Empty{}); err == nil {
			return rpc, nil
		}
	}
	sm.connect()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.rpc == nil {
		return nil, fmt.Errorf("cannot connect to sliver server at %s:%d: %s", cfg.LHost, cfg.LPort, sm.err)
	}
	return sm.rpc, nil
}

// call runs fn with the live client, retrying once after a reconnect.
func (sm *sliverManager) call(fn func(rpc rpcpb.SliverRPCClient) error) error {
	rpc, err := sm.client()
	if err == nil {
		err = fn(rpc)
	}
	if err != nil {
		// One reconnect attempt: the server may have restarted (new connection needed).
		sm.connect()
		if rpc, err2 := sm.client(); err2 == nil {
			if err3 := fn(rpc); err3 == nil {
				return nil
			}
		}
	}
	return err
}

func (s *Server) sliverStatus(w http.ResponseWriter, r *http.Request) {
	sm := s.sliver
	sm.mu.Lock()
	defer sm.mu.Unlock()
	status := map[string]any{"configured": sm.cfg != nil, "connected": sm.rpc != nil}
	if sm.cfg != nil {
		status["host"] = sm.cfg.LHost
		status["port"] = sm.cfg.LPort
		status["operator"] = sm.cfg.Operator
	}
	if sm.err != "" {
		status["error"] = sm.err
	}
	writeJSON(w, 200, status)
}

func (s *Server) sliverSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperatorConfig string `json:"operator_config"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cfg, err := parseSliverConfig(req.OperatorConfig)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.sliver.pg.SetSetting(settingSliverOperatorConfig, req.OperatorConfig); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.sliver.mu.Lock()
	s.sliver.cfg = cfg
	s.sliver.mu.Unlock()
	s.sliver.connect()
	s.sliverStatus(w, r)
}

func (s *Server) sliverClearConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.sliver.pg.SetSetting(settingSliverOperatorConfig, ""); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.sliver.mu.Lock()
	s.sliver.cfg = nil
	s.sliver.closeLocked()
	s.sliver.err = ""
	s.sliver.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) sliverSessions(w http.ResponseWriter, r *http.Request) {
	sm := s.sliver
	var resp *clientpb.Sessions
	err := sm.call(func(rpc rpcpb.SliverRPCClient) error {
		out, rpcErr := rpc.GetSessions(r.Context(), &commonpb.Empty{})
		if rpcErr != nil {
			return rpcErr
		}
		resp = out
		return nil
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	sessions := make([]map[string]any, 0, len(resp.GetSessions()))
	for _, sess := range resp.GetSessions() {
		sessions = append(sessions, map[string]any{
			"id": sess.ID, "name": sess.Name, "hostname": sess.Hostname,
			"username": sess.Username, "uid": sess.UID, "os": sess.OS, "arch": sess.Arch,
			"transport": sess.Transport, "remote_address": sess.RemoteAddress,
			"pid": sess.PID, "filename": sess.Filename, "last_checkin": sess.LastCheckin,
			"active_c2": sess.ActiveC2, "version": sess.Version,
		})
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions})
}

func (s *Server) sliverBeacons(w http.ResponseWriter, r *http.Request) {
	sm := s.sliver
	var resp *clientpb.Beacons
	err := sm.call(func(rpc rpcpb.SliverRPCClient) error {
		out, rpcErr := rpc.GetBeacons(r.Context(), &commonpb.Empty{})
		if rpcErr != nil {
			return rpcErr
		}
		resp = out
		return nil
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	beacons := make([]map[string]any, 0, len(resp.GetBeacons()))
	for _, b := range resp.GetBeacons() {
		beacons = append(beacons, map[string]any{
			"id": b.ID, "name": b.Name, "hostname": b.Hostname,
			"username": b.Username, "uid": b.UID, "os": b.OS, "arch": b.Arch,
			"transport": b.Transport, "remote_address": b.RemoteAddress,
			"pid": b.PID, "filename": b.Filename, "last_checkin": b.LastCheckin,
			"active_c2": b.ActiveC2, "version": b.Version, "interval": b.Interval,
			"next_checkin": b.NextCheckin, "reconnect_interval": b.ReconnectInterval,
		})
	}
	writeJSON(w, 200, map[string]any{"beacons": beacons})
}

func (s *Server) sliverBeaconTasks(w http.ResponseWriter, r *http.Request) {
	sm := s.sliver
	var resp *clientpb.BeaconTasks
	err := sm.call(func(rpc rpcpb.SliverRPCClient) error {
		out, rpcErr := rpc.GetBeaconTasks(r.Context(), &clientpb.Beacon{ID: r.PathValue("id")})
		if rpcErr != nil {
			return rpcErr
		}
		resp = out
		return nil
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	tasks := make([]map[string]any, 0, len(resp.GetTasks()))
	for _, t := range resp.GetTasks() {
		tasks = append(tasks, sliverBeaconTaskDTO(t))
	}
	writeJSON(w, 200, map[string]any{"tasks": tasks})
}

// sliverBeaconTaskDTO carries the response as a UTF-8 string when valid —
// beacon command output is bytes, the UI renders text.
func sliverBeaconTaskDTO(t *clientpb.BeaconTask) map[string]any {
	return map[string]any{
		"id": t.ID, "beacon_id": t.BeaconID, "created_at": t.CreatedAt,
		"state": t.State, "sent_at": t.SentAt, "completed_at": t.CompletedAt,
		"description": t.Description, "response": string(t.Response),
	}
}

func (s *Server) sliverJobs(w http.ResponseWriter, r *http.Request) {
	sm := s.sliver
	var resp *clientpb.Jobs
	err := sm.call(func(rpc rpcpb.SliverRPCClient) error {
		out, rpcErr := rpc.GetJobs(r.Context(), &commonpb.Empty{})
		if rpcErr != nil {
			return rpcErr
		}
		resp = out
		return nil
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	jobs := make([]map[string]any, 0, len(resp.GetActive()))
	for _, j := range resp.GetActive() {
		jobs = append(jobs, map[string]any{
			"id": j.ID, "name": j.Name, "description": j.Description,
			"protocol": j.Protocol, "port": j.Port, "domains": j.Domains,
		})
	}
	writeJSON(w, 200, map[string]any{"jobs": jobs})
}

func (s *Server) sliverStartListener(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string   `json:"type"` // mtls | http | https | dns | wg
		Host    string   `json:"host"`
		Port    int      `json:"port"`
		Domains []string `json:"domains,omitempty"` // dns
		Website string   `json:"website,omitempty"` // http/https (serve an existing website name)
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if req.Host == "" {
		req.Host = "0.0.0.0"
	}
	if req.Port == 0 {
		writeErr(w, 400, "port is required")
		return
	}
	sm := s.sliver
	var jobID uint32
	err := sm.call(func(rpc rpcpb.SliverRPCClient) error {
		ctx, cancel := context.WithTimeout(r.Context(), sliverCommandTimeout)
		defer cancel()
		var rpcErr error
		var resp *clientpb.HTTPListener // all four return only JobID; one variable fits
		switch req.Type {
		case "mtls":
			var mtls *clientpb.MTLSListener
			mtls, rpcErr = rpc.StartMTLSListener(ctx, &clientpb.MTLSListenerReq{Host: req.Host, Port: uint32(req.Port)})
			if mtls != nil {
				resp = &clientpb.HTTPListener{JobID: mtls.JobID}
			}
		case "http":
			resp, rpcErr = rpc.StartHTTPListener(ctx, &clientpb.HTTPListenerReq{Host: req.Host, Port: uint32(req.Port), Domain: req.Website, Website: req.Website})
		case "https":
			resp, rpcErr = rpc.StartHTTPSListener(ctx, &clientpb.HTTPListenerReq{Host: req.Host, Port: uint32(req.Port), Domain: req.Website, Website: req.Website, Secure: true})
		case "dns":
			if len(req.Domains) == 0 {
				return fmt.Errorf("domains is required for a dns listener")
			}
			var dns *clientpb.DNSListener
			dns, rpcErr = rpc.StartDNSListener(ctx, &clientpb.DNSListenerReq{Host: req.Host, Port: uint32(req.Port), Domains: req.Domains, Canaries: true})
			if dns != nil {
				resp = &clientpb.HTTPListener{JobID: dns.JobID}
			}
		case "wg":
			var wg *clientpb.WGListener
			wg, rpcErr = rpc.StartWGListener(ctx, &clientpb.WGListenerReq{Host: req.Host, Port: uint32(req.Port)})
			if wg != nil {
				resp = &clientpb.HTTPListener{JobID: wg.JobID}
			}
		default:
			return fmt.Errorf("unknown listener type %q (mtls, http, https, dns, wg)", req.Type)
		}
		if resp != nil {
			jobID = resp.JobID
		}
		return rpcErr
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"job_id": jobID, "type": req.Type, "host": req.Host, "port": req.Port})
}

func (s *Server) sliverKillJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		writeErr(w, 400, "invalid job id")
		return
	}
	sm := s.sliver
	var killed *clientpb.KillJob
	err = sm.call(func(rpc rpcpb.SliverRPCClient) error {
		out, rpcErr := rpc.KillJob(r.Context(), &clientpb.KillJobReq{ID: uint32(id)})
		if rpcErr != nil {
			return rpcErr
		}
		killed = out
		return nil
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": killed.GetID(), "success": killed.GetSuccess()})
}

// sliverInteract runs one command line on a session (blocking, up to the
// timeout) or submits it to a beacon (async; read the output from
// GET /api/sliver/beacons/{id}/tasks).
func (s *Server) sliverInteract(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string   `json:"session_id,omitempty"`
		BeaconID  string   `json:"beacon_id,omitempty"`
		Argv      []string `json:"argv"` // ["whoami"] or ["cat", "/etc/passwd"]
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.SessionID == "" && req.BeaconID == "" {
		writeErr(w, 400, "session_id or beacon_id is required")
		return
	}
	if len(req.Argv) == 0 || strings.TrimSpace(req.Argv[0]) == "" {
		writeErr(w, 400, "argv is required")
		return
	}
	request := &commonpb.Request{Timeout: int64(sliverCommandTimeout)}
	if req.SessionID != "" {
		request.SessionID = req.SessionID
	} else {
		request.BeaconID = req.BeaconID
		request.Timeout = 0 // beacons pick the command up on their next check-in
	}
	sm := s.sliver
	var out *sliverpb.Execute
	err := sm.call(func(rpc rpcpb.SliverRPCClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), sliverCommandTimeout)
		defer cancel()
		resp, rpcErr := rpc.Execute(ctx, &sliverpb.ExecuteReq{Path: req.Argv[0], Args: req.Argv[1:], Output: true, Request: request})
		if rpcErr != nil {
			return rpcErr
		}
		if resp.Response != nil && resp.Response.Err != "" {
			return fmt.Errorf("implant error: %s", resp.Response.Err)
		}
		out = resp
		return nil
	})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"status":   out.GetStatus(),
		"pid":      out.GetPid(),
		"stdout":   string(out.GetStdout()),
		"stderr":   string(out.GetStderr()),
		"beacon":   req.BeaconID != "",
	})
}
