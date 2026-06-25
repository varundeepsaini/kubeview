package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestServe_GracefulShutdownOnSignal starts the server on an OS-assigned
// port, fires SIGTERM into the stop channel, and confirms serve() returns
// cleanly. Exercises the happy path of serve().
func TestServe_GracefulShutdownOnSignal(t *testing.T) {
	srv, addr := newEphemeralServer(t)
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- serve(srv, stop) }()

	// Wait for the listener to be up — a sub-second probe loop with bounded
	// total wait.
	waitForListen(t, addr)

	stop <- syscall.SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after SIGTERM")
	}
}

// TestServe_ListenError returns the listen-error path: a server bound to an
// already-in-use port should produce a non-ErrServerClosed error.
func TestServe_ListenError(t *testing.T) {
	occupier, addr := newEphemeralServer(t)
	stopOccupier := make(chan os.Signal, 1)
	occupierDone := make(chan error, 1)
	go func() { occupierDone <- serve(occupier, stopOccupier) }()
	waitForListen(t, addr)

	collider := &http.Server{Addr: addr, Handler: http.NewServeMux()}
	colliderStop := make(chan os.Signal, 1)
	colliderDone := make(chan error, 1)
	go func() { colliderDone <- serve(collider, colliderStop) }()

	select {
	case err := <-colliderDone:
		if err == nil {
			t.Fatal("expected listen error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("collider did not return")
	}

	// Clean up the occupier
	stopOccupier <- syscall.SIGTERM
	<-occupierDone
}

// TestRun_PropagatesClientError exercises run()'s error-from-NewClient path
// by pointing KUBECONFIG at a malformed file.
func TestRun_PropagatesClientError(t *testing.T) {
	bad := t.TempDir() + "/bad-config"
	if err := os.WriteFile(bad, []byte("not yaml: [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", bad)
	t.Setenv("PORT", "0") // unused — run() exits before binding
	if err := run(); err == nil {
		t.Fatal("expected NewClient error to propagate")
	}
}

// TestRun_DefaultsPortWhenUnset triggers run() with PORT unset to cover the
// `port = defaultPort` branch. We expect NewClient to fail (with whatever
// kubeconfig the test env has, the address bind will not even happen if
// NewClient errors first; we just need *some* error to bail run() before
// it blocks).
func TestRun_DefaultsPortWhenUnset(t *testing.T) {
	bad := t.TempDir() + "/missing"
	t.Setenv("KUBECONFIG", bad)
	t.Setenv("PORT", "")
	if err := run(); err == nil {
		t.Fatal("expected run() to bail on NewClient error")
	}
}

// TestRun_HappyPath drives run() end-to-end: it builds a valid local
// kubeconfig (pointing at a closed port — the real K8s API never gets called),
// starts run() in a goroutine, waits for the listener to come up, then
// SIGTERMs the test process so the signal.Notify path inside run() fires.
// Covers the server construction + signal.Notify + serve() invocation.
func TestRun_HappyPath(t *testing.T) {
	dir := t.TempDir()
	kc := dir + "/config"
	if err := os.WriteFile(kc, []byte(`apiVersion: v1
kind: Config
current-context: c
clusters:
- cluster: {server: https://127.0.0.1:1, insecure-skip-tls-verify: true}
  name: cl
contexts:
- context: {cluster: cl, user: u}
  name: c
users:
- name: u
  user: {token: fake}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kc)
	// Pick a free port so other parallel tests don't collide.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	t.Setenv("PORT", itoa(port))

	done := make(chan error, 1)
	go func() { done <- run() }()
	waitForListen(t, "127.0.0.1:"+itoa(port))

	// Fire SIGTERM at our own process — run()'s signal.Notify will catch it.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return after SIGTERM")
	}
}

// itoa avoids pulling in strconv just for an int->string conversion in a test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// --- helpers --------------------------------------------------------------

func newEphemeralServer(t *testing.T) (*http.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv, addr
}

func waitForListen(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server on %s did not start listening", addr)
}

// Silence the unused-import linter if errors gets removed in a future edit.
var _ = errors.Is
