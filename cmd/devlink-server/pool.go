package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JSJ-Experiments/devlink/internal/link"
)

// One public loopback SSH port per device, many private Chisel listeners.
// Every new SSH TCP connection rotates across live independent WS/TCP lanes.
// Existing SSH sessions keep their selected lane; no protocol rewriting occurs.
func startSSHPool(ctx context.Context, d link.Enrollment) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", d.SSHPort))
	if err != nil {
		return nil, err
	}
	var next atomic.Uint64
	go func() { <-ctx.Done(); listener.Close() }()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			offset := int(next.Add(1) - 1)
			go relaySSH(ctx, conn, d.TunnelPorts, offset)
		}
	}()
	return listener, nil
}
func relaySSH(ctx context.Context, incoming net.Conn, ports []int, offset int) {
	defer incoming.Close()
	// Probe the SSH banner, not just TCP accept: an old reverse listener may
	// temporarily remain open while its dead tunnel is being detected.
	for i := 0; i < len(ports) && ctx.Err() == nil; i++ {
		p := ports[(offset+i)%len(ports)]
		outgoing, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 300*time.Millisecond)
		if err != nil {
			continue
		}
		outgoing.SetReadDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReaderSize(outgoing, 4096)
		banner, err := reader.ReadSlice('\n')
		if err != nil || len(banner) > 1024 || !strings.HasPrefix(string(banner), "SSH-") {
			outgoing.Close()
			continue
		}
		outgoing.SetReadDeadline(time.Time{})
		if _, err = incoming.Write(banner); err != nil {
			outgoing.Close()
			return
		}
		done := make(chan struct{}, 2)
		go func() {
			io.Copy(outgoing, incoming)
			if c, ok := outgoing.(*net.TCPConn); ok {
				c.CloseWrite()
			}
			done <- struct{}{}
		}()
		go func() {
			io.Copy(incoming, reader)
			if c, ok := incoming.(*net.TCPConn); ok {
				c.CloseWrite()
			}
			done <- struct{}{}
		}()
		select {
		case <-ctx.Done():
		case <-done:
			// A peer's write EOF must not discard the other direction's reply.
			select {
			case <-ctx.Done():
			case <-done:
			}
		}
		incoming.Close()
		outgoing.Close()
		// Closing both sockets also releases copy goroutines on cancellation.
		return
	}
}
func (r *registry) reserveBackends() ([]int, error) {
	used := map[int]bool{}
	for _, d := range r.devices {
		for _, p := range d.TunnelPorts {
			used[p] = true
		}
	}
	count := r.lanes
	if count < 1 {
		count = 1
	}
	ports := []int{}
	for p := 22000; p < 25000 && len(ports) < count; p++ {
		if !used[p] && portFree(p) {
			ports = append(ports, p)
		}
	}
	if len(ports) != count {
		return nil, fmt.Errorf("no free private lane ports")
	}
	return ports, nil
}
func (r *registry) migrateBackends() error {
	changed := false
	for i := range r.devices {
		if len(r.devices[i].TunnelPorts) > 0 {
			continue
		}
		ports, e := r.reserveBackends()
		if e != nil {
			return e
		}
		r.devices[i].TunnelPorts = ports
		changed = true
	}
	if changed {
		return link.WriteJSON(r.path, r.devices)
	}
	return nil
}
