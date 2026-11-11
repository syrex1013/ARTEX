// Package vpn provides a thin wrapper around the Mullvad VPN daemon CLI.
// It executes `mullvad` as a subprocess — no external library needed.
// All commands are non-blocking and timeout in 10 seconds.
package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// Status is the current VPN state.
type Status struct {
	Connected  bool   `json:"connected"`
	State      string `json:"state"` // connected|connecting|disconnected|blocked
	Location   string `json:"location,omitempty"`
	IPAddress  string `json:"ip_address,omitempty"`
	ExitServer string `json:"exit_server,omitempty"`
	Available  bool   `json:"available"` // false when the mullvad binary is not found
}

// RelayCountry groups servers by country.
type RelayCountry struct {
	Name    string        `json:"name"`
	Code    string        `json:"code"`
	Cities  []RelayCity   `json:"cities"`
}

// RelayCity groups servers by city.
type RelayCity struct {
	Name    string        `json:"name"`
	Code    string        `json:"code"`
	Relays  []RelayServer `json:"relays"`
}

// RelayServer is one VPN server.
type RelayServer struct {
	Hostname string `json:"hostname"`
	Country  string `json:"country"`
	City     string `json:"city"`
	Active   bool   `json:"active"`
}

// Available reports whether the mullvad CLI is on PATH.
func Available() bool {
	_, err := exec.LookPath("mullvad")
	return err == nil
}

func run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "mullvad", args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// GetStatus returns the current VPN state.
func GetStatus() Status {
	if !Available() {
		return Status{Available: false}
	}
	out, err := run("status")
	if err != nil {
		return Status{Available: true, State: "error"}
	}
	s := Status{Available: true}
	lower := strings.ToLower(out)
	switch {
	case strings.Contains(lower, "connected"):
		s.Connected = true
		s.State = "connected"
		// parse "Connected to <server> in <city>, <country>"
		if idx := strings.Index(lower, "connected to "); idx >= 0 {
			rest := out[idx+len("connected to "):]
			s.ExitServer = strings.Fields(rest)[0]
		}
	case strings.Contains(lower, "connecting"):
		s.State = "connecting"
	case strings.Contains(lower, "blocked"):
		s.State = "blocked"
	default:
		s.State = "disconnected"
	}
	return s
}

// Connect activates the VPN, optionally to a specific server.
func Connect(server string) error {
	if server != "" {
		if _, err := run("relay", "set", "hostname", server); err != nil {
			return err
		}
	}
	_, err := run("connect")
	return err
}

// Disconnect deactivates the VPN.
func Disconnect() error {
	_, err := run("disconnect")
	return err
}

// ListRelays parses `mullvad relay list --json` output.
func ListRelays() ([]RelayCountry, error) {
	if !Available() {
		return nil, nil
	}
	out, err := run("relay", "list", "--json")
	if err != nil {
		// Fallback: return empty list rather than error
		return []RelayCountry{}, nil
	}
	var countries []RelayCountry
	if err := json.Unmarshal([]byte(out), &countries); err != nil {
		return []RelayCountry{}, nil
	}
	return countries, nil
}

// SetAccount links a Mullvad account number.
func SetAccount(account string) error {
	_, err := run("account", "set", account)
	return err
}

// GetAccount returns the current account number (masked).
func GetAccount() string {
	out, err := run("account", "get")
	if err != nil {
		return ""
	}
	return out
}
