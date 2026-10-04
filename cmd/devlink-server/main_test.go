package main

import (
	"encoding/json"
	"github.com/JSJ-Experiments/devlink/internal/link"
	chserver "github.com/jpillora/chisel/server"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newRegistry(t *testing.T) *registry {
	t.Helper()
	ch, e := chserver.NewServer(&chserver.Config{Reverse: true})
	if e != nil {
		t.Fatal(e)
	}
	ch.AddUser("sentinel", "never-issued", "a^")
	t.Cleanup(func() { ch.Close() })
	return &registry{ch: ch, path: filepath.Join(t.TempDir(), "devices.json"), max: 2, lanes: 4, keys: "ssh-ed25519 public"}
}
func request(r *registry, secret string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.enroll(w, httptest.NewRequest("POST", "/custom/enroll", strings.NewReader(`{"secret":"`+secret+`","label":"Generic Android"}`)))
	return w
}
func TestEnrollment(t *testing.T) {
	r := newRegistry(t)
	s, _ := link.Random()
	w := request(r, s)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a link.Enrollment
	json.Unmarshal(w.Body.Bytes(), &a)
	if a.ID != link.Identity(s) || a.SSHPort == a.ADBPort || len(a.SSHPorts) != 4 {
		t.Fatal(a)
	}
	w = request(r, s)
	if w.Code != 200 || len(r.devices) != 1 {
		t.Fatal(w.Code)
	}
	s2, _ := link.Random()
	if w = request(r, s2); w.Code != 429 {
		t.Fatal("rate limit", w.Code)
	}
	r.lastNew = time.Time{}
	if w = request(r, s2); w.Code != 200 {
		t.Fatal(w.Code)
	}
	s3, _ := link.Random()
	if w = request(r, s3); w.Code != 503 {
		t.Fatal("capacity", w.Code)
	}
	r.devices[0].Revoked = true
	if w = request(r, s); w.Code != 403 {
		t.Fatal("revocation", w.Code)
	}
	r2 := newRegistry(t)
	r2.path = r.path
	if e := r2.load(); e != nil || len(r2.devices) != 2 {
		t.Fatal(e)
	}
}
func TestInvalidEnrollment(t *testing.T) {
	r := newRegistry(t)
	if w := request(r, "guessable"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w := httptest.NewRecorder()
	r.enroll(w, httptest.NewRequest("GET", "/enroll", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	r.enroll(w, httptest.NewRequest("POST", "/enroll", strings.NewReader(strings.Repeat("x", 5000))))
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
