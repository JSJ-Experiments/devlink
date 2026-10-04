package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type agentDevice struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	Connected     bool      `json:"connected"`
	ActiveLanes   int       `json:"active_lanes"`
	SSHHost       string    `json:"ssh_host"`
	SSHPort       int       `json:"ssh_port"`
	SSHUser       string    `json:"ssh_user"`
	ADBHost       string    `json:"adb_host"`
	ADBPort       int       `json:"adb_port"`
	ADBForwarding bool      `json:"adb_forwarding"`
	Revoked       bool      `json:"revoked"`
	Created       time.Time `json:"created"`
}

// Inspect listener state without creating SSH probes/radio traffic on devices.
// A silent dead tunnel can remain reported until Chisel detects its dead ping.
func loopbackListeners() map[int]bool {
	ports := map[int]bool{}
	b, e := os.ReadFile("/proc/net/tcp")
	if e != nil {
		return ports
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[3] != "0A" {
			continue
		}
		addr := strings.Split(f[1], ":")
		if len(addr) != 2 || addr[0] != "0100007F" {
			continue
		}
		p, e := strconv.ParseUint(addr[1], 16, 16)
		if e == nil {
			ports[int(p)] = true
		}
	}
	return ports
}
func (r *registry) devicesHandler(w http.ResponseWriter, q *http.Request) {
	if q.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	listeners := loopbackListeners()
	r.mu.Lock()
	defer r.mu.Unlock()
	result := []agentDevice{}
	for _, d := range r.devices {
		lanes := 0
		if !d.Revoked {
			for _, p := range d.TunnelPorts {
				if listeners[p] {
					lanes++
				}
			}
		}
		v := agentDevice{ID: d.ID, Label: d.Label, Connected: lanes > 0, ActiveLanes: lanes, SSHHost: "127.0.0.1", SSHPort: d.SSHPort, SSHUser: "root", ADBHost: "127.0.0.1", ADBPort: d.ADBPort, ADBForwarding: !d.Revoked && listeners[d.ADBPort], Revoked: d.Revoked, Created: d.Created}
		if q.URL.Query().Get("connected") == "1" && !v.Connected {
			continue
		}
		result = append(result, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(result)
}
