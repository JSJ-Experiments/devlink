package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JSJ-Experiments/devlink/internal/link"
	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
)

func testFreePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func TestEveryLaneHasUnlimitedBoundedRetries(t *testing.T) {
	s := state{Endpoint: "https://example.com/custom", Secret: "secret", Enrollment: &link.Enrollment{ID: "device", Fingerprint: "pinned"}}
	for _, remotes := range [][]string{{"R:127.0.0.1:22000:127.0.0.1:18022", "R:127.0.0.1:18623:127.0.0.1:5555"}, {"R:127.0.0.1:22001:127.0.0.1:18022"}} {
		c := tunnelConfig(s, remotes, http.Header{}, "")
		if c.MaxRetryCount != -1 || c.MinRetryInterval != time.Second || c.MaxRetryInterval != 5*time.Minute || c.KeepAlive != 25*time.Second {
			t.Fatal("retry policy", c)
		}
		if c.Server != "https://example.com/custom/tunnel" || c.Fingerprint != "pinned" || c.Auth != "device:secret" {
			t.Fatal("identity changed")
		}
	}
}
func TestRealChiselRetriesInitialFailuresAndDroppedConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server, e := chserver.NewServer(&chserver.Config{Reverse: true})
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	bp, rp := testFreePort(t), testFreePort(t)
	if e = server.AddUser("device", "secret", link.Allowed(rp, rp+1)...); e != nil {
		t.Fatal(e)
	}
	if e = server.StartContext(ctx, "127.0.0.1", fmt.Sprint(bp)); e != nil {
		t.Fatal(e)
	}
	s := state{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", bp), Secret: "secret", Enrollment: &link.Enrollment{ID: "device", Fingerprint: server.GetFingerprint()}}
	cfg := tunnelConfig(s, []string{fmt.Sprintf("R:127.0.0.1:%d:127.0.0.1:18022", rp)}, nil, "")
	cfg.MinRetryInterval = 20 * time.Millisecond
	cfg.MaxRetryInterval = 50 * time.Millisecond
	var attempts atomic.Int32
	sockets := make(chan net.Conn, 16)
	cfg.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if attempts.Add(1) <= 2 {
			return nil, errors.New("simulated initial offline network")
		}
		c, e := (&net.Dialer{}).DialContext(ctx, network, addr)
		if e == nil {
			sockets <- c
		}
		return c, e
	}
	client, e := chclient.NewClient(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	if e = client.Start(ctx); e != nil {
		t.Fatal(e)
	}
	exited := make(chan error, 1)
	go func() { exited <- client.Wait() }()
	if !client.Ready(ctx) {
		t.Fatal("did not retry initial failures")
	}
	first := <-sockets
	first.Close()
	select {
	case <-sockets:
	case e := <-exited:
		t.Fatalf("gave up after disconnect: %v", e)
	case <-ctx.Done():
		t.Fatal("no reconnect dial")
	}
	if !client.Ready(ctx) {
		t.Fatal("failed to reconnect")
	}
	if attempts.Load() < 4 {
		t.Fatal("retry not exercised")
	}
	client.Close()
	select {
	case e := <-exited:
		if e != nil {
			t.Fatal("cancellation", e)
		}
	case <-ctx.Done():
		t.Fatal("shutdown blocked")
	}
}
