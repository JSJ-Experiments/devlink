package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/JSJ-Experiments/devlink/internal/link"
	chserver "github.com/jpillora/chisel/server"
)

type registry struct {
	mu           sync.Mutex
	path         string
	devices      []link.Enrollment
	ch           *chserver.Server
	keys, adbkey string
	max          int
	lanes        int
	lastNew      time.Time
}

func (r *registry) load() error {
	e := link.ReadJSON(r.path, &r.devices)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func (r *registry) enroll(w http.ResponseWriter, q *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if q.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	q.Body = http.MaxBytesReader(w, q.Body, 4096)
	var req link.EnrollmentRequest
	if e := json.NewDecoder(q.Body).Decode(&req); e != nil || !link.SecretPattern.MatchString(req.Secret) || len(req.Label) > 120 {
		http.Error(w, "invalid enrollment", 400)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := link.Identity(req.Secret)
	for _, d := range r.devices {
		if d.ID == id {
			if d.Revoked || subtle.ConstantTimeCompare([]byte(req.Secret), []byte(d.Secret)) != 1 {
				http.Error(w, "revoked", 403)
				return
			}
			d.Fingerprint = r.ch.GetFingerprint()
			d.SSHKeys = r.keys
			d.ADBKey = r.adbkey
			json.NewEncoder(w).Encode(d)
			return
		}
	}
	if len(r.devices) >= r.max {
		http.Error(w, "enrollment capacity reached", 503)
		return
	}
	// Global limit bounds anonymous enrollment regardless of proxies/IP spoofing.
	if time.Since(r.lastNew) < 5*time.Second {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "try again shortly", 429)
		return
	}
	used := map[int]bool{}
	for _, d := range r.devices {
		used[d.SSHPort] = true
		used[d.ADBPort] = true
		for _, p := range d.SSHPorts {
			used[p] = true
		}
	}
	sp, ap := 0, 0
	for p := 18622; p < 19622; p += 2 {
		if used[p] || used[p+1] {
			continue
		}
		if portsFree(p, p+1) {
			sp = p
			ap = p + 1
			break
		}
	}
	if sp == 0 {
		http.Error(w, "no free port pairs", 503)
		return
	}
	lanePorts := []int{sp}
	used[sp] = true
	used[ap] = true
	count := r.lanes
	if count < 1 {
		count = 1
	}
	for p := 18622; p < 19622 && len(lanePorts) < count; p++ {
		if used[p] {
			continue
		}
		if portFree(p) {
			lanePorts = append(lanePorts, p)
			used[p] = true
		}
	}
	if len(lanePorts) < count {
		http.Error(w, "no free lane ports", 503)
		return
	}
	d := link.Enrollment{SSHPorts: lanePorts, ID: id, Secret: req.Secret, Label: req.Label, SSHPort: sp, ADBPort: ap, Fingerprint: r.ch.GetFingerprint(), SSHKeys: r.keys, ADBKey: r.adbkey, Created: time.Now().UTC()}
	r.devices = append(r.devices, d)
	if e := link.WriteJSON(r.path, r.devices); e != nil {
		r.devices = r.devices[:len(r.devices)-1]
		http.Error(w, "persistence failed", 500)
		return
	}
	if e := r.ch.AddUser(d.ID, d.Secret, link.AllowedEnrollment(d)...); e != nil {
		http.Error(w, "authorization failed", 500)
		return
	}
	r.lastNew = time.Now()
	log.Printf("enrolled %s label=%q ssh=%d adb=%d", d.ID, d.Label, sp, ap)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}
func portFree(p int) bool {
	l, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	if e != nil {
		return false
	}
	l.Close()
	return true
}
func portsFree(a, b int) bool {
	l, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", a))
	if e != nil {
		return false
	}
	defer l.Close()
	m, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", b))
	if e != nil {
		return false
	}
	m.Close()
	return true
}
func ensureKey(path string) error {
	if _, e := os.Stat(path); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	b, e := x509.MarshalECPrivateKey(k)
	if e != nil {
		return e
	}
	return link.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), 0600)
}
func main() {
	state := flag.String("state", "/var/lib/devlink", "state directory")
	listen := flag.String("listen", "127.0.0.1:18790", "loopback enrollment/proxy listener")
	backend := flag.String("backend", "127.0.0.1:18791", "loopback chisel listener")
	keysPath := flag.String("ssh-keys", "/etc/devlink/authorized_keys", "operator SSH public keys")
	adbPath := flag.String("adb-key", "/etc/devlink/adbkey.pub", "optional operator ADB public key")
	max := flag.Int("max-devices", 64, "anonymous enrollment capacity")
	lanes := flag.Int("lanes", 4, "reserved independent SSH tunnel lanes per device (1..16)")
	prefix := flag.String("path", "/devlink", "public subpath, e.g. /tools/devlink")
	flag.Parse()
	if *lanes < 1 || *lanes > 16 {
		log.Fatal("lanes must be 1..16")
	}
	*prefix = strings.TrimRight(*prefix, "/")
	if *prefix == "" || !strings.HasPrefix(*prefix, "/") || strings.ContainsAny(*prefix, "?# ") || strings.Contains(*prefix, "..") {
		log.Fatal("invalid subpath")
	}
	if e := os.MkdirAll(*state, 0700); e != nil {
		log.Fatal(e)
	}
	if flag.NArg() > 0 {
		var ds []link.Enrollment
		if e := link.ReadJSON(filepath.Join(*state, "devices.json"), &ds); e != nil {
			log.Fatal(e)
		}
		switch flag.Arg(0) {
		case "devices":
			for _, d := range ds {
				fmt.Printf("%s\t%q\tSSH 127.0.0.1:%d\tADB 127.0.0.1:%d\trevoked=%t\n", d.ID, d.Label, d.SSHPort, d.ADBPort, d.Revoked)
				fmt.Printf("  SSH lanes: %v\n", link.LanePorts(d))
			}
		case "revoke":
			found := false
			for i := range ds {
				if ds[i].ID == flag.Arg(1) {
					ds[i].Revoked = true
					found = true
				}
			}
			if !found {
				log.Fatal("unknown device")
			}
			if e := link.WriteJSON(filepath.Join(*state, "devices.json"), ds); e != nil {
				log.Fatal(e)
			}
			fmt.Println("Revoked. Restart devlink-server to disconnect sessions and apply.")
		default:
			log.Fatal("expected devices or revoke ID")
		}
		return
	}
	for _, a := range []string{*listen, *backend} {
		h, _, e := net.SplitHostPort(a)
		if e != nil || h != "127.0.0.1" {
			log.Fatal("listeners must bind 127.0.0.1")
		}
	}
	keyPath := filepath.Join(*state, "server.pem")
	if e := ensureKey(keyPath); e != nil {
		log.Fatal(e)
	}
	ch, e := chserver.NewServer(&chserver.Config{KeyFile: keyPath, Reverse: true, KeepAlive: 25 * time.Second})
	if e != nil {
		log.Fatal(e)
	}
	// Chisel permits anonymous access when the user index is empty. Keep a
	// never-issued deny-all sentinel even before the first enrollment.
	sentinel, e := link.Random()
	if e != nil {
		log.Fatal(e)
	}
	if e = ch.AddUser("__deny__", sentinel, `a^`); e != nil {
		log.Fatal(e)
	}
	keys, e := os.ReadFile(*keysPath)
	if e != nil || len(strings.TrimSpace(string(keys))) == 0 {
		log.Fatal("SSH public keys required")
	}
	adb, _ := os.ReadFile(*adbPath)
	r := &registry{path: filepath.Join(*state, "devices.json"), ch: ch, keys: string(keys), adbkey: strings.TrimSpace(string(adb)), max: *max, lanes: *lanes}
	if e = r.load(); e != nil {
		log.Fatal(e)
	}
	for _, d := range r.devices {
		if !d.Revoked {
			if e = ch.AddUser(d.ID, d.Secret, link.AllowedEnrollment(d)...); e != nil {
				log.Fatal(e)
			}
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	host, port, _ := net.SplitHostPort(*backend)
	if e = ch.StartContext(ctx, host, port); e != nil {
		log.Fatal(e)
	}
	defer ch.Close()
	target, _ := url.Parse("http://" + *backend)
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux := http.NewServeMux()
	mux.HandleFunc(*prefix+"/enroll", r.enroll)
	mux.HandleFunc(*prefix+"/tunnel", func(w http.ResponseWriter, q *http.Request) {
		if !strings.EqualFold(q.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "WebSocket required", 400)
			return
		}
		proxy.ServeHTTP(w, q)
	})
	mux.HandleFunc(*prefix+"/health", func(w http.ResponseWriter, q *http.Request) { fmt.Fprintln(w, "devlink ok") })
	mux.HandleFunc("/", func(w http.ResponseWriter, q *http.Request) { http.NotFound(w, q) })
	h := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() { <-ctx.Done(); h.Close() }()
	log.Printf("DevLink %s fingerprint=%s", *listen, ch.GetFingerprint())
	if e = h.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
