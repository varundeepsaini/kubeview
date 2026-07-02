package main

import (
	"context"
	"errors"
	"fmt"
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

	// signalChannelBuffer holds at least one pending OS signal so a delivery is
	// never dropped while serve() is busy starting up.
	signalChannelBuffer = 1
	// errChannelBuffer lets the server goroutine report a startup error without
	// blocking, even if no one is selecting yet.
	errChannelBuffer = 1
)

// KubeEvent is the response shape for a cluster event. JSON tags must match
// what the frontend expects in kubeview-frontend/src/lib/api.ts.
type KubeEvent struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Object    string `json:"object"`
	Namespace string `json:"namespace"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
	Source    string `json:"source"`
	Count     int32  `json:"count"`
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

// run is split from main() so tests can drive the same code path with a
// controllable signal channel.
func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	client, err := NewClient()
	if err != nil {
		return fmt.Errorf("init kubernetes client: %w", err)
	}

	corsOrigins := parseCORSOrigins(os.Getenv("CORS_ORIGIN"))
	log.Printf("CORS allowed origins: %v", corsOrigins)

	server := new(http.Server)
	server.Addr = ":" + port
	server.Handler = withCORS(newRouter(client), corsOrigins)
	server.ReadTimeout = readTimeout
	server.WriteTimeout = writeTimeout
	server.IdleTimeout = idleTimeout

	stop := make(chan os.Signal, signalChannelBuffer)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	return serve(server, stop)
}

// serve runs the HTTP server until the stop channel fires, then performs a
// graceful shutdown bounded by shutdownTimeout. The stop channel is a
// parameter so tests can substitute it.
func serve(server *http.Server, stop <-chan os.Signal) error {
	errCh := make(chan error, errChannelBuffer)

	go func() {
		log.Printf("KubeView API listening on %s", server.Addr)

		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}

		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-stop:
		return shutdown(server)
	}
}

// shutdown performs a graceful shutdown bounded by shutdownTimeout.
func shutdown(server *http.Server) error {
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	return nil
}
