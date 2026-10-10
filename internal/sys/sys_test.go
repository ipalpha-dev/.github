package sys

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParsers(t *testing.T) {
	if h := ParseLsof("p123\ncnode\np456\ncdocker\n"); len(h) != 2 || h[0].PID != 123 || h[1].Command != "docker" {
		t.Fatalf("lsof %+v", h)
	}
	ss := `LISTEN 0 511 *:3001 *:* users:(("node",pid=1234,fd=20),("node",pid=1235,fd=21))`
	if h := ParseSS(ss); len(h) != 2 || h[0].PID != 1234 || h[0].Command != "node" {
		t.Fatalf("ss %+v", h)
	}
	ns := "  TCP    0.0.0.0:3001           0.0.0.0:0              LISTENING       4321\r\n  TCP    [::]:3001              [::]:0                 LISTENING       4321\r\n  TCP    0.0.0.0:30010          0.0.0.0:0              LISTENING       99\r\n"
	if h := ParseNetstat(ns, "3001"); len(h) != 1 || h[0].PID != 4321 {
		t.Fatalf("netstat %+v", h)
	}
}

func TestPortBusyAndFree(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !PortBusy(port) {
		t.Fatal("listening port reported free")
	}
	p, err := FreePort(port, nil)
	if err != nil || p == port {
		t.Fatalf("FreePort: %d %v", p, err)
	}
	_ = strconv.Itoa(p)
}

func TestUnwrapShim(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules", "@x", "cli", "bin"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "@x", "cli", "bin", "cli.js"), []byte(""), 0o644)
	shim := filepath.Join(dir, "x.cmd")
	os.WriteFile(shim, []byte("@ECHO off\r\nGOTO start\r\n:start\r\nSETLOCAL\r\nCALL :find_dp0\r\n\"%_prog%\"  \"%dp0%\\node_modules\\@x\\cli\\bin\\cli.js\" %*\r\n"), 0o644)
	name, args := unwrap(shim, []string{"-p", "a & b"})
	if name != "node" || len(args) != 3 || filepath.Base(args[0]) != "cli.js" || args[2] != "a & b" {
		t.Fatalf("%s %q", name, args)
	}
	if n, _ := unwrap(filepath.Join(dir, "plain.exe"), nil); n != filepath.Join(dir, "plain.exe") {
		t.Fatal("non-shim changed")
	}
}
