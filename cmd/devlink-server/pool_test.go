package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JSJ-Experiments/devlink/internal/link"
)

func mockSSH(t *testing.T, name string) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				fmt.Fprintf(c, "SSH-2.0-%s\r\n", name)
				data, _ := io.ReadAll(c)
				c.Write(data)
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}
func unusedPort(t *testing.T) int {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func TestSinglePortPoolRoundRobinAndFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := mockSSH(t, "lane-a"), mockSSH(t, "lane-b")
	l, e := startSSHPool(ctx, link.Enrollment{SSHPort: unusedPort(t), TunnelPorts: []int{a, b}})
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	for _, want := range []string{"lane-a", "lane-b", "lane-a", "lane-b"} {
		c, e := net.Dial("tcp", l.Addr().String())
		if e != nil {
			t.Fatal(e)
		}
		r := bufio.NewReader(c)
		banner, e := r.ReadString('\n')
		if e != nil || !strings.Contains(banner, want) {
			t.Fatal(banner, e)
		}
		c.Write([]byte("keep reply after half-close"))
		c.(*net.TCPConn).CloseWrite()
		reply, e := io.ReadAll(r)
		c.Close()
		if e != nil || string(reply) != "keep reply after half-close" {
			t.Fatal(string(reply), e)
		}
	}
	fallback, e := startSSHPool(ctx, link.Enrollment{SSHPort: unusedPort(t), TunnelPorts: []int{unusedPort(t), a}})
	if e != nil {
		t.Fatal(e)
	}
	defer fallback.Close()
	c, e := net.Dial("tcp", fallback.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	banner, e := bufio.NewReader(c).ReadString('\n')
	if e != nil || !strings.Contains(banner, "lane-a") {
		t.Fatal(banner, e)
	}
}
func TestDiscoveryRedactsSecretsAndFiltersConnected(t *testing.T) {
	r := newRegistry(t)
	p := mockSSH(t, "connected")
	r.devices = []link.Enrollment{{ID: "one", Label: "Phone", Secret: "NEVER-EXPOSE", SSHKeys: "PRIVATE-REGISTRY-DATA", SSHPort: 18622, ADBPort: 18623, TunnelPorts: []int{p}}, {ID: "two", Label: "Offline", TunnelPorts: []int{unusedPort(t)}}}
	w := httptest.NewRecorder()
	r.devicesHandler(w, httptest.NewRequest("GET", "/devices?connected=1", nil))
	var got []agentDevice
	if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	if len(got) != 1 || got[0].ID != "one" || got[0].SSHPort != 18622 || got[0].ActiveLanes != 1 {
		t.Fatal(got)
	}
	for _, secret := range []string{"NEVER-EXPOSE", "PRIVATE-REGISTRY-DATA", "tunnel_ports"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("registry leak", secret)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cache")
	}
	r.devices[0].Revoked = true
	w = httptest.NewRecorder()
	r.devicesHandler(w, httptest.NewRequest("GET", "/devices?connected=1", nil))
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	r.devicesHandler(w, httptest.NewRequest("POST", "/devices", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}
func TestMigrationPreservesPublicPortsAndIdentity(t *testing.T) {
	r := newRegistry(t)
	r.devices = []link.Enrollment{{ID: "a", Secret: "unchanged", SSHPort: 18622, ADBPort: 18623}, {ID: "b", SSHPort: 18628, ADBPort: 18629}}
	if e := r.migrateBackends(); e != nil {
		t.Fatal(e)
	}
	if r.devices[0].SSHPort != 18622 || r.devices[0].Secret != "unchanged" || len(r.devices[0].TunnelPorts) != 4 {
		t.Fatal(r.devices)
	}
	for _, a := range r.devices[0].TunnelPorts {
		for _, b := range r.devices[1].TunnelPorts {
			if a == b {
				t.Fatal("shared backend")
			}
		}
	}
	ports := fmt.Sprint(r.devices[0].TunnelPorts)
	if e := r.migrateBackends(); e != nil || fmt.Sprint(r.devices[0].TunnelPorts) != ports {
		t.Fatal(e)
	}
}
