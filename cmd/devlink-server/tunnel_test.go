package main

import (
	"context"
	"fmt"
	"github.com/JSJ-Experiments/devlink/internal/link"
	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"io"
	"net"
	"testing"
	"time"
)

func unused(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func TestRealTunnelAndDeniedForward(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := chserver.NewServer(&chserver.Config{Reverse: true, KeepAlive: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	bp, rp := unused(t), unused(t)
	s.AddUser("sentinel", "not-issued", "a^")
	s.AddUser("device", "password", link.Allowed(rp, rp+1)...)
	if e = s.StartContext(ctx, "127.0.0.1", fmt.Sprint(bp)); e != nil {
		t.Fatal(e)
	}
	echo, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	config := func(remotes []string) *chclient.Config {
		return &chclient.Config{Server: fmt.Sprintf("http://127.0.0.1:%d/custom/tunnel", bp), Auth: "device:password", Fingerprint: s.GetFingerprint(), Remotes: remotes, KeepAlive: time.Second, MaxRetryCount: 1, MinRetryInterval: time.Second, MaxRetryInterval: time.Second}
	}
	cl, e := chclient.NewClient(config([]string{fmt.Sprintf("R:127.0.0.1:%d:%s", rp, echo.Addr())}))
	if e != nil {
		t.Fatal(e)
	}
	defer cl.Close()
	if e = cl.Start(ctx); e != nil {
		t.Fatal(e)
	}
	if !cl.Ready(ctx) {
		t.Fatal("did not connect")
	}
	var c net.Conn
	for i := 0; i < 100; i++ {
		c, e = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", rp), 100*time.Millisecond)
		if e == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	c.Write([]byte("devlink"))
	b := make([]byte, 7)
	if _, e = io.ReadFull(c, b); e != nil || string(b) != "devlink" {
		t.Fatal(e, string(b))
	}
	// Device credentials cannot use cfarm as an SSRF/normal-forward/SOCKS exit.
	denied, e := chclient.NewClient(config([]string{fmt.Sprintf("127.0.0.1:%d:%s", unused(t), echo.Addr())}))
	if e != nil {
		t.Fatal(e)
	}
	defer denied.Close()
	denied.Start(ctx)
	short, stop := context.WithTimeout(ctx, 2300*time.Millisecond)
	defer stop()
	if denied.Ready(short) {
		t.Fatal("ordinary forward unexpectedly authorized")
	}
	wrong := config([]string{fmt.Sprintf("R:0.0.0.0:%d:%s", unused(t), echo.Addr())})
	wrong.Auth = "unknown:invalid"
	unknown, e := chclient.NewClient(wrong)
	if e != nil {
		t.Fatal(e)
	}
	defer unknown.Close()
	unknown.Start(ctx)
	short2, stop2 := context.WithTimeout(ctx, 2300*time.Millisecond)
	defer stop2()
	if unknown.Ready(short2) {
		t.Fatal("anonymous client accepted")
	}
}
