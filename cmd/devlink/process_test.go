package main

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestParentBoundProcessSurvivesCallerAndRuntimeActivity(t *testing.T) {
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	defer writer.Close()
	c := exec.Command("/bin/sh", "-c", "read line; exit 7")
	c.Stdin = reader
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	exited, e := startParentBoundProcess(c)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 20; i++ {
		runtime.GC()
		runtime.Gosched()
	}
	select {
	case e := <-exited:
		t.Fatalf("child ended prematurely: %v", e)
	case <-time.After(100 * time.Millisecond):
	}
	writer.Write([]byte("done\n"))
	writer.Close()
	select {
	case e := <-exited:
		if e, ok := e.(*exec.ExitError); !ok || e.ExitCode() != 7 {
			t.Fatalf("expected exit 7, got %v", e)
		}
	case <-time.After(5 * time.Second):
		c.Process.Kill()
		t.Fatal("child was not reaped")
	}
}

func TestParentBoundProcessStartFailure(t *testing.T) {
	exited, e := startParentBoundProcess(exec.Command("/definitely/missing/devlink-child"))
	if e == nil || exited != nil {
		t.Fatalf("missing binary should fail clearly: %v %v", exited, e)
	}
}
