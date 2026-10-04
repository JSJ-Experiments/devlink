package main

import (
	"strings"
	"testing"
	"time"
)

func TestEndpointAndPreservedState(t *testing.T) {
	dir = t.TempDir()
	mod = t.TempDir()
	if e := cli([]string{"init"}); e != nil {
		t.Fatal(e)
	}
	s, _ := load()
	secret := s.Secret
	for _, u := range []string{"arm.jadenjsj.com", "https://cfarm.jadenjsj.com/tools/nested/link", "https://192.0.2.1/custom"} {
		if e := cli([]string{"endpoint", u, "--tls-name", "cfarm.jadenjsj.com"}); e != nil {
			t.Fatal(e)
		}
		s, _ = load()
		if s.Secret != secret || s.TLSName != "cfarm.jadenjsj.com" || !strings.HasPrefix(s.Endpoint, "https://") {
			t.Fatal(s)
		}
	}
	if e := cli([]string{"endpoint", "http://192.0.2.1/custom"}); e == nil {
		t.Fatal("HTTP must be explicit")
	}
	if e := cli([]string{"endpoint", "http://127.0.0.1:18790/custom", "--allow-http"}); e != nil {
		t.Fatal(e)
	}
	for _, u := range []string{"file:///tmp/key", "https://name:password@example.com/devlink", "https://example.com/path?token=a"} {
		if e := cli([]string{"endpoint", u}); e == nil {
			t.Fatal(u)
		}
	}
	if e := cli([]string{"adb", "on"}); e == nil {
		t.Fatal("ADB cannot enable tunnel implicitly")
	}
}
func TestExpiredBoot(t *testing.T) {
	dir = t.TempDir()
	mod = t.TempDir()
	initState()
	s, _ := load()
	s.Enabled = true
	s.ADB = true
	s.Until = time.Now().Add(-time.Hour).Unix()
	save(s)
	if e := cli([]string{"boot"}); e != nil {
		t.Fatal(e)
	}
	s, _ = load()
	if s.Enabled || s.ADB || s.Until != 0 {
		t.Fatal(s)
	}
}
func TestInvalidDurations(t *testing.T) {
	dir = t.TempDir()
	mod = t.TempDir()
	for _, d := range []string{"0s", "-1h", "two hours"} {
		if e := cli([]string{"on", d}); e == nil {
			t.Fatal(d)
		}
	}
}
func TestLaneLeaseExpiry(t *testing.T) {
	s := state{Lanes: 4, LanesUntil: time.Now().Add(time.Minute).Unix()}
	if activeLanes(s) != 4 {
		t.Fatal("active lanes")
	}
	s.LanesUntil = time.Now().Add(-time.Minute).Unix()
	if activeLanes(s) != 1 {
		t.Fatal("expired lane lease")
	}
	s.Lanes = 0
	if activeLanes(s) != 1 {
		t.Fatal("baseline")
	}
}
