package link

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestRestrictedListeners(t *testing.T) {
	patterns := Allowed(18622, 18623)
	for _, v := range []string{"R:127.0.0.1:18622", "R:127.0.0.1:18623"} {
		ok := false
		for _, p := range patterns {
			ok = ok || regexp.MustCompile(p).MatchString(v)
		}
		if !ok {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"R:0.0.0.0:18622", "R:127.0.0.1:186220", "R:127x0x0x1:18622", "R:127.0.0.1:18624", "R:socks", "socks", "127.0.0.1:22", "prefixR:127.0.0.1:18622"} {
		for _, p := range patterns {
			if regexp.MustCompile(p).MatchString(v) {
				t.Fatal("unauthorized:", v)
			}
		}
	}
}
func TestAtomicState(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	if e := WriteJSON(p, map[string]string{"secret": "private"}); e != nil {
		t.Fatal(e)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0600 {
		t.Fatal(fi.Mode())
	}
	var v map[string]string
	if e := ReadJSON(p, &v); e != nil || v["secret"] != "private" {
		t.Fatal(e, v)
	}
	s, e := Random()
	if e != nil || !SecretPattern.MatchString(s) || len(Identity(s)) != 32 {
		t.Fatal(s, e)
	}
}
