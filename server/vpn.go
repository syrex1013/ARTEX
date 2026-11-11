package server

import (
	"encoding/json"
	"net/http"

	"github.com/Autumn-27/artex/vpn"
)

// VPN API — wraps the Mullvad daemon CLI.

func (s *Server) vpnStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, vpn.GetStatus())
}

func (s *Server) vpnConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Server string `json:"server"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := vpn.Connect(req.Server); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, vpn.GetStatus())
}

func (s *Server) vpnDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := vpn.Disconnect(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, vpn.GetStatus())
}

func (s *Server) vpnRelays(w http.ResponseWriter, r *http.Request) {
	list, err := vpn.ListRelays()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) vpnAccount(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"account": vpn.GetAccount()})
}

func (s *Server) vpnSetAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account string `json:"account"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Account == "" {
		writeErr(w, 400, "account number required")
		return
	}
	if err := vpn.SetAccount(req.Account); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
