// DevLink's control process exits after each command. Only an enabled session
// runs a daemon, containing the chisel client and one small Dropbear process.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/JSJ-Experiments/devlink/internal/link"
	chclient "github.com/jpillora/chisel/client"
)

var defaultEndpoint = "https://cfarm.jadenjsj.com/devlink"
var defaultFingerprint = ""
var version = "dev"

type state struct {
	Lanes      int              `json:"lanes,omitempty"`
	LanesUntil int64            `json:"lanes_until,omitempty"`
	Enabled    bool             `json:"enabled"`
	Until      int64            `json:"until,omitempty"`
	ADB        bool             `json:"adb"`
	Endpoint   string           `json:"endpoint"`
	TLSName    string           `json:"tls_name,omitempty"`
	AllowHTTP  bool             `json:"allow_http,omitempty"`
	Secret     string           `json:"secret"`
	Enrollment *link.Enrollment `json:"enrollment,omitempty"`
}
type status struct {
	Lanes          int    `json:"lanes"`
	LanesUntil     int64  `json:"lanes_until,omitempty"`
	LanesAvailable int    `json:"lanes_available,omitempty"`
	Enabled        bool   `json:"enabled"`
	Running        bool   `json:"running"`
	Connected      bool   `json:"connected"`
	ADB            bool   `json:"adb"`
	Until          int64  `json:"until"`
	Endpoint       string `json:"endpoint"`
	ID             string `json:"id,omitempty"`
	SSHPort        int    `json:"ssh_port,omitempty"`
	ADBPort        int    `json:"adb_port,omitempty"`
	TLSName        string `json:"tls_name,omitempty"`
	AllowHTTP      bool   `json:"allow_http,omitempty"`
	Version        string `json:"version"`
	Error          string `json:"error,omitempty"`
}

var dir, mod string

func path(s string) string { return filepath.Join(dir, s) }
func load() (state, error) { var s state; e := link.ReadJSON(path("state.json"), &s); return s, e }
func save(s state) error   { return link.WriteJSON(path("state.json"), s) }
func initState() error {
	if _, e := os.Stat(path("state.json")); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	secret, e := link.Random()
	if e != nil {
		return e
	}
	return save(state{Endpoint: defaultEndpoint, Secret: secret})
}
func lock(name string, nonblock bool) (*os.File, error) {
	f, e := os.OpenFile(path(name), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	how := syscall.LOCK_EX
	if nonblock {
		how |= syscall.LOCK_NB
	}
	if e = syscall.Flock(int(f.Fd()), how); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func lockContext(ctx context.Context, name string) (*os.File, error) {
	for {
		f, e := lock(name, true)
		if e == nil {
			return f, nil
		}
		if !errors.Is(e, syscall.EWOULDBLOCK) {
			return nil, e
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func pid() int {
	b, _ := os.ReadFile(path("daemon.pid"))
	p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if p <= 1 {
		return 0
	}
	cmd, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", p))
	parts := strings.Split(string(cmd), "\x00")
	if len(parts) < 2 || parts[1] != "daemon" || !strings.Contains(parts[0], "devlink") {
		return 0
	}
	if syscall.Kill(p, 0) != nil {
		return 0
	}
	return p
}
func stopDaemon() error {
	p := pid()
	if p == 0 {
		return nil
	}
	if e := syscall.Kill(p, syscall.SIGTERM); e != nil {
		return e
	}
	for i := 0; i < 300; i++ {
		if pid() == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("daemon cleanup timed out; inspect devlink.log (not force-killing adbd restore)")
}
func startDaemon() error {
	if pid() != 0 {
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	// Bound log storage, without a periodic logger process.
	if fi, e := os.Stat(path("devlink.log")); e == nil && fi.Size() > 2<<20 {
		os.Rename(path("devlink.log"), path("devlink.old.log"))
	}
	f, e := os.OpenFile(path("devlink.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c := exec.Command(exe, "daemon")
	c.Env = os.Environ()
	c.Stdout = f
	c.Stderr = f
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = c.Start(); e != nil {
		return e
	}
	c.Process.Release()
	for i := 0; i < 30; i++ {
		if pid() != 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("daemon did not start; inspect devlink.log")
}
func activateAsync(source, installer string) error {
	if source == "" {
		source = mod
	}
	script := filepath.Join(source, "scripts", "hotreload.sh")
	if _, e := os.Stat(script); e != nil {
		return e
	}
	f, e := os.OpenFile(path("hotreload.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c := exec.Command("/system/bin/sh", script, source, installer)
	c.Env = os.Environ()
	c.Stdout = f
	c.Stderr = f
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = c.Start(); e != nil {
		return e
	}
	return c.Process.Release()
}
func restartAsync() error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path("devlink.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c := exec.Command(exe, "restart-daemon")
	c.Env = os.Environ()
	c.Stdout = f
	c.Stderr = f
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = c.Start(); e != nil {
		return e
	}
	return c.Process.Release()
}
func restartWorker() error {
	// Detached from the SSH process group so changing an endpoint or ADB mode
	// cannot kill the controller before it restarts the tunnel.
	time.Sleep(500 * time.Millisecond)
	f, e := lock("control.lock", false)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = stopDaemon(); e != nil {
		return e
	}
	s, e := load()
	if e != nil {
		return e
	}
	if !s.Enabled {
		return nil
	}
	return startDaemon()
}
func getStatus() status {
	s, e := load()
	v := status{Version: version}
	if e != nil {
		v.Error = e.Error()
		return v
	}
	v.Enabled = s.Enabled
	v.Until = s.Until
	v.Endpoint = s.Endpoint
	v.Lanes = activeLanes(s)
	v.LanesUntil = s.LanesUntil
	v.TLSName = s.TLSName
	v.AllowHTTP = s.AllowHTTP
	v.ADB = s.ADB
	v.Running = pid() != 0
	var live status
	if link.ReadJSON(path("status.json"), &live) == nil {
		v.Error = live.Error
		if v.Running {
			v.Connected = live.Connected
		}
	}
	if s.Enrollment != nil {
		v.ID = s.Enrollment.ID
		v.SSHPort = s.Enrollment.SSHPort
		v.LanesAvailable = len(link.LanePorts(*s.Enrollment))
		v.ADBPort = s.Enrollment.ADBPort
	}
	return v
}
func cli(args []string) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	f, e := lock("control.lock", false)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = initState(); e != nil {
		return e
	}
	s, e := load()
	if e != nil {
		return e
	}
	switch args[0] {
	case "init":
		return nil
	case "stop-runtime":
		return stopDaemon()
	case "activate":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: devlink activate MODULE_DIR [INSTALLER_PID]")
		}
		parent := "0"
		if len(args) == 3 {
			if _, e := strconv.Atoi(args[2]); e != nil {
				return e
			}
			parent = args[2]
		}
		return activateAsync(args[1], parent)
	case "reload":
		source := mod
		if _, e := os.Stat("/data/adb/modules_update/devlink/module.prop"); e == nil {
			source = "/data/adb/modules_update/devlink"
		}
		return activateAsync(source, "0")
	case "status":
		v := getStatus()
		if len(args) > 1 && args[1] == "--json" {
			return json.NewEncoder(os.Stdout).Encode(v)
		}
		fmt.Printf("DevLink %s\nenabled=%t running=%t connected=%t adb=%t\nendpoint=%s\n", version, v.Enabled, v.Running, v.Connected, v.ADB, v.Endpoint)
		if v.Until > 0 {
			fmt.Println("until:", time.Unix(v.Until, 0).Format(time.RFC3339))
		}
		if v.ID != "" {
			fmt.Printf("device=%s\nssh root@127.0.0.1 -p %d\nadb connect 127.0.0.1:%d (when enabled)\n", v.ID, v.SSHPort, v.ADBPort)
		}
		if v.Error != "" {
			fmt.Println("last error:", v.Error)
		}
		return nil
	case "on":
		s.Enabled = true
		s.Until = 0
		if len(args) > 1 {
			d, e := time.ParseDuration(args[1])
			if e != nil || d <= 0 {
				return errors.New("use a positive duration, e.g. 2h or 30m")
			}
			s.Until = time.Now().Add(d).Unix()
		}
	case "lanes":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: devlink lanes COUNT [15m]")
		}
		n, e := strconv.Atoi(args[1])
		if e != nil || n < 1 || s.Enrollment == nil || n > len(link.LanePorts(*s.Enrollment)) {
			return errors.New("lane count exceeds enrollment; check devlink status --json")
		}
		if !s.Enabled {
			return errors.New("enable DevLink first")
		}
		s.Lanes = n
		s.LanesUntil = 0
		if len(args) == 3 {
			d, e := time.ParseDuration(args[2])
			if e != nil || d <= 0 {
				return errors.New("positive lane duration required")
			}
			s.LanesUntil = time.Now().Add(d).Unix()
		}
	case "off":
		s.Enabled = false
		s.Until = 0
		s.ADB = false
		s.Lanes = 1
		s.LanesUntil = 0
	case "boot":
		if s.Until > 0 && time.Now().Unix() >= s.Until {
			s.Enabled = false
			s.Until = 0
			s.ADB = false
			s.Lanes = 1
			s.LanesUntil = 0
		}
	case "adb":
		if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
			return errors.New("usage: devlink adb on|off")
		}
		s.ADB = args[1] == "on"
		if s.ADB && !s.Enabled {
			return errors.New("enable the tunnel first: devlink on [duration]")
		}
	case "endpoint":
		if len(args) == 1 {
			fmt.Println(s.Endpoint)
			return nil
		}
		raw := args[1]
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return errors.New("use https://HOST[:PORT]/devlink")
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/devlink"
		}
		s.TLSName = ""
		s.AllowHTTP = false
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--tls-name":
				i++
				if i >= len(args) {
					return errors.New("--tls-name requires a hostname")
				}
				s.TLSName = args[i]
			case "--allow-http":
				s.AllowHTTP = true
			default:
				return errors.New("unknown endpoint option")
			}
		}
		if u.Scheme == "http" && !s.AllowHTTP {
			return errors.New("HTTP exposes enrollment credentials; explicitly opt in with --allow-http, or use HTTPS IP --tls-name CERT_HOST")
		}
		s.Endpoint = strings.TrimRight(u.String(), "/")
		// Same server identity/ports work on cfarm, direct arm, and direct IP.
		// Different independent servers require reset-enrollment.
	case "reset-enrollment":
		if s.Enabled {
			return errors.New("devlink off before resetting enrollment")
		}
		s.Secret, e = link.Random()
		if e != nil {
			return e
		}
		s.Enrollment = nil
	default:
		return errors.New("usage: devlink on [2h] | off | status [--json] | adb on|off | endpoint [URL] [--tls-name HOST] | reset-enrollment")
	}
	if e = save(s); e != nil {
		return e
	}
	if !s.Enabled {
		return stopDaemon()
	}
	if args[0] == "adb" || args[0] == "endpoint" {
		return restartAsync()
	}
	existing := pid()
	if e = startDaemon(); e != nil {
		return e
	}
	if existing != 0 {
		syscall.Kill(existing, syscall.SIGHUP)
	}
	return nil
}
func rootPool() *x509.CertPool {
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	for _, d := range []string{"/apex/com.android.conscrypt/cacerts", "/system/etc/security/cacerts"} {
		es, _ := os.ReadDir(d)
		for _, e := range es {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(d, e.Name()))
			if err == nil {
				pool.AppendCertsFromPEM(b)
			}
		}
	}
	return pool
}
func enroll(ctx context.Context, s state) (*link.Enrollment, error) {
	label := strings.TrimSpace(commandOutput("getprop", "ro.product.model"))
	if label == "" {
		label, _ = os.Hostname()
	}
	b, _ := json.Marshal(link.EnrollmentRequest{Secret: s.Secret, Label: label})
	req, e := http.NewRequestWithContext(ctx, "POST", s.Endpoint+"/enroll", bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	if s.TLSName != "" {
		req.Host = s.TLSName
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool(), ServerName: s.TLSName, MinVersion: tls.VersionTLS12}}
	defer tr.CloseIdleConnections()
	h := &http.Client{Timeout: 30 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := h.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("enrollment HTTP %d", resp.StatusCode)
	}
	var en link.Enrollment
	if e = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&en); e != nil {
		return nil, e
	}
	if en.ID != link.Identity(s.Secret) || en.Secret != s.Secret || len(en.Fingerprint) != 44 || en.SSHPort < 1024 || en.SSHPort > 65535 || en.ADBPort < 1024 || en.ADBPort > 65535 || en.SSHPort == en.ADBPort || strings.TrimSpace(en.SSHKeys) == "" {
		return nil, errors.New("invalid enrollment response")
	}
	seen := map[int]bool{en.ADBPort: true, en.SSHPort: true}
	ports := link.LanePorts(en)
	if len(en.TunnelPorts) == 0 || len(ports) > 16 {
		return nil, errors.New("invalid SSH lanes")
	}
	for _, p := range ports {
		if p < 1024 || p > 65535 || seen[p] {
			return nil, errors.New("invalid SSH lane ports")
		}
		seen[p] = true
	}
	if defaultFingerprint != "" && en.Fingerprint != defaultFingerprint {
		return nil, errors.New("server identity differs from pinned build; use module built for this server")
	}
	return &en, nil
}
func commandOutput(name string, args ...string) string {
	c := exec.Command(name, args...)
	b, _ := c.Output()
	return string(b)
}
func helper(action string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "/system/bin/sh", filepath.Join(mod, "scripts", "adb.sh"), action)
	c.Env = append(os.Environ(), "DEVLINK_STATE="+dir)
	b, e := c.CombinedOutput()
	return strings.TrimSpace(string(b)), e
}
func setupSSH(s state) (*exec.Cmd, error) {
	sshDir := path("ssh")
	if e := os.MkdirAll(sshDir, 0700); e != nil {
		return nil, e
	}
	if e := link.WriteFile(filepath.Join(sshDir, "authorized_keys"), []byte(s.Enrollment.SSHKeys), 0600); e != nil {
		return nil, e
	}
	key := filepath.Join(sshDir, "host_ed25519")
	if _, e := os.Stat(key); os.IsNotExist(e) {
		c := exec.Command(filepath.Join(mod, "bin", "dropbearkey"), "-t", "ed25519", "-f", key)
		if b, e := c.CombinedOutput(); e != nil {
			return nil, fmt.Errorf("host key: %s: %w", b, e)
		}
	}
	c := exec.Command(filepath.Join(mod, "bin", "dropbear"), "-F", "-e", "-p", "127.0.0.1:18022", "-P", path("dropbear.pid"), "-r", key, "-D", sshDir, "-I", "600")
	c.Env = append(os.Environ(), "HOME="+sshDir, "PATH="+filepath.Join(mod, "bin")+":/system/bin:/system/xbin:/vendor/bin")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	if e := c.Start(); e != nil {
		return nil, e
	}
	return c, nil
}
func disabled() bool {
	for _, f := range []string{"disable", "remove"} {
		if _, e := os.Stat(filepath.Join(mod, f)); e == nil {
			return true
		}
	}
	return false
}
func activeLanes(s state) int {
	if s.Lanes < 1 || (s.LanesUntil > 0 && time.Now().Unix() >= s.LanesUntil) {
		return 1
	}
	return s.Lanes
}

// The Chisel CLI sets MaxRetryCount=-1; NewClient's Go zero value means
// no retries. Share an explicit policy across the primary and every extra lane.
func tunnelConfig(s state, remotes []string, headers http.Header, caPath string) *chclient.Config {
	return &chclient.Config{Headers: headers, Server: s.Endpoint + "/tunnel", Auth: s.Enrollment.ID + ":" + s.Secret, Fingerprint: s.Enrollment.Fingerprint, KeepAlive: 25 * time.Second, MaxRetryCount: -1, MinRetryInterval: time.Second, MaxRetryInterval: 5 * time.Minute, Remotes: remotes, TLS: chclient.TLSConfig{ServerName: s.TLSName, CA: caPath}}
}

func daemon() (retErr error) {
	f, e := lock("run.lock", true)
	if e != nil {
		return nil
	}
	defer f.Close()
	if e = link.WriteFile(path("daemon.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); e != nil {
		return e
	}
	defer os.Remove(path("daemon.pid"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)
	hup := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case sig := <-signals:
				if sig == syscall.SIGHUP {
					select {
					case hup <- struct{}{}:
					default:
					}
				} else {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	// Expiry/disable detection also runs during offline enrollment backoff.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s, e := load()
				if e != nil {
					cancel()
					return
				}
				if !s.Enabled || disabled() || (s.Until > 0 && time.Now().Unix() >= s.Until) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		// Never wait for the CLI lock during shutdown: off holds it while
		// waiting for this daemon to finish. A concurrent command owns state.
		if control, e := lock("control.lock", true); e == nil {
			s, e := load()
			if e == nil && s.Until > 0 && time.Now().Unix() >= s.Until {
				s.Enabled = false
				s.ADB = false
				s.Until = 0
				s.Lanes = 1
				s.LanesUntil = 0
				save(s)
			}
			control.Close()
		}
		v := status{Version: version}
		if retErr != nil {
			v.Error = retErr.Error()
		}
		link.WriteJSON(path("status.json"), v)
	}()
	s, e := load()
	if e != nil {
		return e
	}
	if !s.Enabled || disabled() {
		return nil
	}
	update := func(connected bool, err string) {
		v := getStatus()
		v.Connected = connected
		v.Error = err
		link.WriteJSON(path("status.json"), v)
	}
	update(false, "")
	for delay := time.Second; s.Enrollment == nil || len(s.Enrollment.TunnelPorts) == 0; {
		en, e := enroll(ctx, s)
		if e == nil {
			// Preserve control changes made while enrollment was in flight.
			control, e := lockContext(ctx, "control.lock")
			if e != nil {
				return e
			}
			fresh, e := load()
			if e == nil {
				fresh.Enrollment = en
				e = save(fresh)
				s = fresh
			}
			control.Close()
			if e != nil {
				return e
			}
			break
		}
		update(false, e.Error())
		log.Print(e)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		delay *= 2
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
	}
	if ctx.Err() != nil || !s.Enabled {
		return nil
	}
	sshCmd, e := setupSSH(s)
	if e != nil {
		update(false, e.Error())
		return e
	}
	sshExit := make(chan error, 1)
	go func() { sshExit <- sshCmd.Wait() }()
	// Killing the process group also closes established SSH sessions.
	defer func() {
		if sshCmd.Process != nil {
			syscall.Kill(-sshCmd.Process.Pid, syscall.SIGTERM)
			select {
			case <-sshExit:
			case <-time.After(2 * time.Second):
			}
		}
	}()
	adbPort := 0
	if s.ADB {
		if s.Enrollment.ADBKey != "" {
			if e = link.WriteFile(path("adbkey.pub"), []byte(s.Enrollment.ADBKey+"\n"), 0600); e != nil {
				return e
			}
		}
		p, e := helper("on")
		if e != nil {
			helper("off")
			update(false, "ADB: "+p)
			return fmt.Errorf("ADB: %s: %w", p, e)
		}
		adbPort, e = strconv.Atoi(p)
		if e != nil {
			return fmt.Errorf("ADB helper returned invalid port: %q", p)
		}
		defer func() {
			if p, e := helper("off"); e != nil {
				log.Printf("ADB cleanup: %s: %v", p, e)
			}
		}()
	}
	remotes := []string{fmt.Sprintf("R:127.0.0.1:%d:127.0.0.1:18022", link.LanePorts(*s.Enrollment)[0])}
	if adbPort > 0 {
		remotes = append(remotes, fmt.Sprintf("R:127.0.0.1:%d:127.0.0.1:%d", s.Enrollment.ADBPort, adbPort))
	}
	// Linux cross-builds don't automatically find Android's certificate stores.
	// Export a PEM bundle used by chisel's default RootCAs as well as enrollment.
	var ca []byte
	for _, d := range []string{"/apex/com.android.conscrypt/cacerts", "/system/etc/security/cacerts"} {
		es, _ := os.ReadDir(d)
		for _, e := range es {
			if !e.IsDir() {
				b, _ := os.ReadFile(filepath.Join(d, e.Name()))
				ca = append(ca, b...)
				ca = append(ca, '\n')
			}
		}
	}
	caPath := ""
	if len(ca) > 0 {
		caPath = path("ca.pem")
		if e = link.WriteFile(caPath, ca, 0600); e != nil {
			return e
		}
	}
	headers := http.Header{}
	if s.TLSName != "" {
		headers.Set("Host", s.TLSName)
	}
	cl, e := chclient.NewClient(tunnelConfig(s, remotes, headers, caPath))
	if e != nil {
		return e
	}
	if e = cl.Start(ctx); e != nil {
		return e
	}
	defer cl.Close()
	extras := []*chclient.Client{}
	defer func() {
		for _, lane := range extras {
			lane.Close()
		}
	}()
	reconcile := func() error {
		desired := activeLanes(s) - 1
		ports := link.LanePorts(*s.Enrollment)
		if desired >= len(ports) {
			desired = len(ports) - 1
		}
		for len(extras) > desired {
			extras[len(extras)-1].Close()
			extras = extras[:len(extras)-1]
		}
		for len(extras) < desired {
			lane, e := chclient.NewClient(tunnelConfig(s, []string{fmt.Sprintf("R:127.0.0.1:%d:127.0.0.1:18022", ports[len(extras)+1])}, headers, caPath))
			if e != nil {
				return e
			}
			if e = lane.Start(ctx); e != nil {
				return e
			}
			extras = append(extras, lane)
		}
		return nil
	}
	if e = reconcile(); e != nil {
		return e
	}
	exited := make(chan error, 1)
	go func() { exited <- cl.Wait() }()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-sshExit:
			update(false, "Dropbear exited")
			return fmt.Errorf("Dropbear exited: %v", e)
		case e := <-exited:
			return e
		case <-hup:
			fresh, e := load()
			if e != nil {
				return e
			}
			s = fresh
			if e = reconcile(); e != nil {
				return e
			}
		case <-tick.C:
			if e = reconcile(); e != nil {
				return e
			}
			readyCtx, c := context.WithTimeout(ctx, 100*time.Millisecond)
			ready := cl.Ready(readyCtx)
			c()
			update(ready, "")
			if fi, e := os.Stat(path("devlink.log")); e == nil && fi.Size() > 4<<20 {
				os.Truncate(path("devlink.log"), 0)
			}
		}
	}
}
func main() {
	dir = os.Getenv("DEVLINK_STATE")
	if dir == "" {
		dir = "/data/adb/devlink"
	}
	mod = os.Getenv("DEVLINK_MODULE")
	if mod == "" {
		exe, _ := os.Executable()
		mod = filepath.Dir(filepath.Dir(exe))
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		log.Fatal(e)
	}
	args := os.Args[1:]
	var e error
	if len(args) > 0 && args[0] == "restart-daemon" {
		e = restartWorker()
	} else if len(args) > 0 && args[0] == "daemon" {
		e = daemon()
	} else {
		e = cli(args)
	}
	if e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
