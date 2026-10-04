package main

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/JSJ-Experiments/devlink/internal/link"
)

func TestOptionalADBFailureRollsBackAndReturnsNoForwardPort(t *testing.T) {
	for _, output := range []string{"permission denied", "garbage", "0", "65536"} {
		t.Run(output, func(t *testing.T) {
			dir = t.TempDir()
			calls := []string{}
			action := func(name string) (string, error) {
				calls = append(calls, name)
				if name == "off" {
					return "", nil
				}
				if output == "permission denied" {
					return output, errors.New("setup failed")
				}
				return output, nil
			}
			port, e := setupOptionalADB(state{Enrollment: &link.Enrollment{}}, action)
			if port != 0 || e == nil || !reflect.DeepEqual(calls, []string{"on", "off"}) {
				t.Fatalf("%d %v %v", port, e, calls)
			}
		})
	}
}

func TestOptionalADBBorrowSuccessAndPublicKeyPermissions(t *testing.T) {
	dir = t.TempDir()
	calls := []string{}
	port, e := setupOptionalADB(state{Enrollment: &link.Enrollment{ADBKey: "public-key"}}, func(action string) (string, error) {
		calls = append(calls, action)
		return "5555", nil
	})
	if e != nil || port != 5555 || !reflect.DeepEqual(calls, []string{"on"}) {
		t.Fatalf("%d %v %v", port, e, calls)
	}
	key, e := os.ReadFile(path("adbkey.pub"))
	if e != nil || string(key) != "public-key\n" {
		t.Fatalf("%q %v", key, e)
	}
	info, _ := os.Stat(path("adbkey.pub"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}
