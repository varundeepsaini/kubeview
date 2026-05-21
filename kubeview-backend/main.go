package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultPort     = "5501"
	readTimeout     = 15 * time.Second
	writeTimeout    = 60 * time.Second // pod-log requests can be slow
	idleTimeout     = 120 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run wires up the kube client + HTTP server and blocks until SIGINT/SIGTERM,
// then triggers graceful shutdown. Split out from main() so tests can drive
// the same code path with a controllable signal channel.
func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	client, err := NewClient()
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      withCORS(newRouter(client)),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	return serve(server, stop)
}

// serve runs the HTTP server until the stop channel fires, then performs a
// graceful shutdown bounded by shutdownTimeout. The stop channel is a
// parameter so tests can substitute it.
func serve(server *http.Server, stop <-chan os.Signal) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("KubeView API running on http://localhost%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-stop:
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return err
		}
		return nil
	}
}
