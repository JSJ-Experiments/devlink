package link

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Only these two exact loopback reverse listeners are granted. No forward/SOCKS access.
func Allowed(sshPort, adbPort int) []string {
	return []string{fmt.Sprintf(`^R:127\.0\.0\.1:%d$`, sshPort), fmt.Sprintf(`^R:127\.0\.0\.1:%d$`, adbPort)}
}
func AllowedEnrollment(d Enrollment) []string {
	if len(d.TunnelPorts) > 0 {
		a := []string{fmt.Sprintf(`^R:127\.0\.0\.1:%d$`, d.ADBPort)}
		for _, p := range d.TunnelPorts {
			a = append(a, fmt.Sprintf(`^R:127\.0\.0\.1:%d$`, p))
		}
		return a
	}
	a := Allowed(d.SSHPort, d.ADBPort)
	for _, p := range d.SSHPorts {
		if p != d.SSHPort {
			a = append(a, fmt.Sprintf(`^R:127\.0\.0\.1:%d$`, p))
		}
	}
	return a
}
func LanePorts(d Enrollment) []int {
	if len(d.TunnelPorts) > 0 {
		return d.TunnelPorts
	}
	if len(d.SSHPorts) == 0 {
		return []int{d.SSHPort}
	}
	return d.SSHPorts
}
func Random() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func Identity(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:16])
}

var SecretPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type EnrollmentRequest struct {
	Secret string `json:"secret"`
	Label  string `json:"label"`
}
type Enrollment struct {
	ID          string    `json:"id"`
	Secret      string    `json:"secret,omitempty"`
	Label       string    `json:"label"`
	SSHPort     int       `json:"ssh_port"`
	TunnelPorts []int     `json:"tunnel_ports,omitempty"`
	SSHPorts    []int     `json:"ssh_ports,omitempty"`
	ADBPort     int       `json:"adb_port"`
	Fingerprint string    `json:"fingerprint"`
	SSHKeys     string    `json:"ssh_keys"`
	ADBKey      string    `json:"adb_key,omitempty"`
	Created     time.Time `json:"created"`
	Revoked     bool      `json:"revoked,omitempty"`
}

func WriteJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return WriteFile(path, append(b, '\n'), 0600)
}
func WriteFile(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".devlink-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func ReadJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
