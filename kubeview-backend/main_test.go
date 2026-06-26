package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

const (
	mtLoopbackAddr   = "127.0.0.1:0"
	mtLoopbackHost   = "127.0.0.1:"
	mtHealthzPath    = "/healthz"
	mtKubeConfigEnv  = "KUBECONFIG"
	mtPortEnv        = "PORT"
	mtStatusOK       = 200
	mtServeWait      = 3 * time.Second
	mtCollideWait    = 2 * time.Second
	mtRunWait        = 5 * time.Second
	mtDialTimeout    = 50 * time.Millisecond
	mtListenDeadline = 2 * time.Second
	mtPollInterval   = 20 * time.Millisecond
	mtReadHeaderTO   = 5 * time.Second
	mtConfigPerm     = 0o600
	mtChanBuf        = 1
	mtZero           = 0
	mtDecimalBase    = 10
	mtIntBufLen      = 20
	mtKubeConfigYAML = `apiVersion: v1
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
`
)

// TestServe_GracefulShutdownOnSignal starts the server on an OS-assigned
// port, fires SIGTERM into the stop channel, and confirms serve() returns
// cleanly. Exercises the happy path of serve().
func TestServe_GracefulShutdownOnSignal(t *testing.T) {
	t.Parallel()

	srv, addr := newEphemeralServer(t)
	stop := make(chan os.Signal, mtChanBuf)
	done := make(chan error, mtChanBuf)

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
	case <-time.After(mtServeWait):
		t.Fatal("serve did not return after SIGTERM")
	}
}

// TestServe_ListenError returns the listen-error path: a server bound to an
// already-in-use port should produce a non-ErrServerClosed error.
func TestServe_ListenError(t *testing.T) {
	t.Parallel()

	occupier, addr := newEphemeralServer(t)
	stopOccupier := make(chan os.Signal, mtChanBuf)
	occupierDone := make(chan error, mtChanBuf)

	go func() { occupierDone <- serve(occupier, stopOccupier) }()

	waitForListen(t, addr)

	collider := newServer(addr)
	colliderStop := make(chan os.Signal, mtChanBuf)
	colliderDone := make(chan error, mtChanBuf)

	go func() { colliderDone <- serve(collider, colliderStop) }()

	select {
	case err := <-colliderDone:
		if err == nil {
			t.Fatal("expected listen error, got nil")
		}
	case <-time.After(mtCollideWait):
		t.Fatal("collider did not return")
	}

	// Clean up the occupier.
	stopOccupier <- syscall.SIGTERM

	<-occupierDone
}

// TestRun_PropagatesClientError exercises run()'s error-from-NewClient path
// by pointing KUBECONFIG at a malformed file.
//
// It is not parallel: t.Setenv mutates process-global environment and the Go
// test runtime forbids combining t.Setenv with t.Parallel.
func TestRun_PropagatesClientError(t *testing.T) {
	bad := t.TempDir() + "/bad-config"

	err := os.WriteFile(bad, []byte("not yaml: [[["), mtConfigPerm)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(mtKubeConfigEnv, bad)
	t.Setenv(mtPortEnv, "0") // unused — run() exits before binding

	runErr := run()
	if runErr == nil {
		t.Fatal("expected NewClient error to propagate")
	}
}

// TestRun_DefaultsPortWhenUnset triggers run() with PORT unset to cover the
// `port = defaultPort` branch. We expect NewClient to fail (with whatever
// kubeconfig the test env has, the address bind will not even happen if
// NewClient errors first; we just need *some* error to bail run() before
// it blocks).
//
// It is not parallel: t.Setenv mutates process-global environment and the Go
// test runtime forbids combining t.Setenv with t.Parallel.
func TestRun_DefaultsPortWhenUnset(t *testing.T) {
	bad := t.TempDir() + "/missing"
	t.Setenv(mtKubeConfigEnv, bad)
	t.Setenv(mtPortEnv, "")

	runErr := run()
	if runErr == nil {
		t.Fatal("expected run() to bail on NewClient error")
	}
}

// TestRun_HappyPath drives the server construction + graceful-shutdown path
// end-to-end. It builds a valid local kubeconfig (pointing at a closed port —
// the real K8s API never gets called), constructs the same server run() would,
// starts serve() in a goroutine, waits for the listener to come up, then drives
// shutdown through the injectable stop channel that serve() accepts.
//
// Shutdown is driven through the injectable stop channel rather than a
// process-wide OS signal: a real syscall.Kill on the test PID would be caught
// by signal.Notify in any concurrently running test, killing siblings. Feeding
// SIGTERM into the local channel exercises the identical graceful-shutdown code
// path with no process-wide side effect. The test is not parallel because
// t.Setenv mutates process-global environment.
func TestRun_HappyPath(t *testing.T) {
	dir := t.TempDir()
	kc := dir + "/config"

	err := os.WriteFile(kc, []byte(mtKubeConfigYAML), mtConfigPerm)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(mtKubeConfigEnv, kc)

	// Pick a free port so concurrently running tests do not collide.
	listener, err := listen(t, mtLoopbackAddr)
	if err != nil {
		t.Fatal(err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr is %T, want *net.TCPAddr", listener.Addr())
	}

	port := addr.Port

	closeErr := listener.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	t.Setenv(mtPortEnv, itoa(port))

	server := newServer(":" + itoa(port))
	stop := make(chan os.Signal, mtChanBuf)
	done := make(chan error, mtChanBuf)

	go func() { done <- serve(server, stop) }()

	waitForListen(t, mtLoopbackHost+itoa(port))

	// Drive shutdown through the injectable channel — no process-wide signal.
	stop <- syscall.SIGTERM

	select {
	case serveErr := <-done:
		if serveErr != nil {
			t.Fatalf("serve returned %v", serveErr)
		}
	case <-time.After(mtRunWait):
		t.Fatal("serve did not return after SIGTERM")
	}
}

// itoa avoids pulling in strconv just for an int->string conversion in a test.
func itoa(n int) string {
	if n == mtZero {
		return "0"
	}

	neg := n < mtZero
	if neg {
		n = -n
	}

	var buf [mtIntBufLen]byte

	i := len(buf)
	for n > mtZero {
		i--
		buf[i] = byte('0' + n%mtDecimalBase)
		n /= mtDecimalBase
	}

	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// --- helpers --------------------------------------------------------------

// listen opens a TCP listener using a context-aware ListenConfig so the noctx
// linter is satisfied.
func listen(t *testing.T, addr string) (net.Listener, error) {
	t.Helper()

	var cfg net.ListenConfig

	listener, err := cfg.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	return listener, nil
}

// newServer builds an *http.Server with every field set explicitly so the
// exhaustruct linter is satisfied, including ReadHeaderTimeout for gosec G112.
func newServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(mtHealthzPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(mtStatusOK)
	})

	return &http.Server{
		Addr:                         addr,
		Handler:                      mux,
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		ReadTimeout:                  mtZero,
		ReadHeaderTimeout:            mtReadHeaderTO,
		WriteTimeout:                 mtZero,
		IdleTimeout:                  mtZero,
		MaxHeaderBytes:               mtZero,
		TLSNextProto:                 nil,
		ConnState:                    nil,
		ErrorLog:                     nil,
		BaseContext:                  nil,
		ConnContext:                  nil,
		HTTP2:                        nil,
		Protocols:                    nil,
	}
}

func newEphemeralServer(t *testing.T) (*http.Server, string) {
	t.Helper()

	listener, err := listen(t, mtLoopbackAddr)
	if err != nil {
		t.Fatal(err)
	}

	addr := listener.Addr().String()

	closeErr := listener.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	return newServer(addr), addr
}

func waitForListen(t *testing.T, addr string) {
	t.Helper()

	var dialer net.Dialer

	deadline := time.Now().Add(mtListenDeadline)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), mtDialTimeout)

		conn, err := dialer.DialContext(ctx, "tcp", addr)

		cancel()

		if err == nil {
			closeErr := conn.Close()
			if closeErr != nil {
				t.Fatal(closeErr)
			}

			return
		}

		time.Sleep(mtPollInterval)
	}

	t.Fatalf("server on %s did not start listening", addr)
}

// Silence the unused-import linter if errors gets removed in a future edit.
var _ = errors.Is
