package main

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseLsofSkipsOwnGroup(t *testing.T) {
	raw := "p111\nccnode\np222\ncnode\n"
	got := parseLsof(raw, 0)
	if len(got) != 2 || got[0].pid != 111 || got[1].cmd != "node" {
		t.Fatalf("parsed %v", got)
	}
}

func TestFreePortKillsTheOtherListenerOnly(t *testing.T) {
	port := freeTCPPort(t)
	other := exec.Command("python3", "-c", "import socket,time; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); s.bind(('127.0.0.1', "+port+")); s.listen(1); time.sleep(30)")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill() }()
	waitListening(t, port)

	own := exec.Command("sleep", "30")
	if err := own.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = own.Process.Kill() }()

	n, err := freePort(port, own.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("signaled %d listeners, want 1", n)
	}
	done := make(chan error, 1)
	go func() { done <- other.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the other listener is still alive")
	}
	if err := syscall.Kill(own.Process.Pid, 0); err != nil {
		t.Fatal("the panel's own process was killed")
	}
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("python3", "-c", "import socket; s=socket.socket(); s.bind(('127.0.0.1', 0)); print(s.getsockname()[1]); s.close()").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func waitListening(t *testing.T, port string) {
	t.Helper()
	p, _ := strconv.Atoi(port)
	if p == 0 {
		t.Fatal("no port")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		holders, err := listenersOn(port, 0)
		if err == nil && len(holders) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("listener never appeared on " + port)
}
