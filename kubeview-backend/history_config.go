package main

// History configuration follows the PORT/CORS_ORIGIN precedent — environment
// variables parsed by pure functions called from run():
//
//	HISTORY_RETENTION_HOURS  how far back the recorder keeps state
//	                         (default 72; 0 or negative disables history)
//	HISTORY_DIR              where the store file lives (default: the user
//	                         cache dir, falling back to the system temp dir)

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	envHistoryRetention = "HISTORY_RETENTION_HOURS"
	envHistoryDir       = "HISTORY_DIR"

	defaultRetentionHours = 72

	// historyDirMode keeps the store directory owner-only, matching the
	// database file's permissions.
	historyDirMode = 0o700

	historyFileName = "history.db"
)

// parseHistoryRetention interprets HISTORY_RETENTION_HOURS: empty or invalid
// values fall back to the default; zero or negative disables history.
func parseHistoryRetention(raw string) time.Duration {
	if raw == emptyString {
		return defaultRetentionHours * time.Hour
	}

	hours, err := strconv.Atoi(raw)
	if err != nil {
		//nolint:gosec // G706: the value is operator-set (env var), %q-quoted.
		log.Printf(
			"history: invalid %s %q, using default %dh",
			envHistoryRetention, raw, defaultRetentionHours,
		)

		return defaultRetentionHours * time.Hour
	}

	if hours <= zeroCount {
		return zeroCount
	}

	return time.Duration(hours) * time.Hour
}

// historyStorePath resolves where the store file lives and ensures the
// directory exists: HISTORY_DIR when set, else a "kubeview" folder in the
// user cache dir, else one in the system temp dir (containers often run
// without a home directory).
func historyStorePath(configuredDir string) (string, error) {
	dir := configuredDir
	if dir == emptyString {
		cacheDir, err := os.UserCacheDir()
		if err == nil {
			dir = filepath.Join(cacheDir, "kubeview")
		} else {
			dir = filepath.Join(os.TempDir(), "kubeview")
		}
	}

	//nolint:gosec // G703: the directory is operator-set (HISTORY_DIR env or
	// a system default), never request input.
	err := os.MkdirAll(dir, historyDirMode)
	if err != nil {
		return emptyString, fmt.Errorf("create history dir: %w", err)
	}

	return filepath.Join(dir, historyFileName), nil
}

// setupHistory opens the store and starts the flight recorder for the default
// context, returning the API surface and a shutdown func. History is
// best-effort: any failure logs a warning and the server runs without it
// rather than blocking the live dashboard.
func setupHistory(manager *ClientManager) (*historyAPI, func()) {
	noop := func() {}

	retention := parseHistoryRetention(os.Getenv(envHistoryRetention))
	if retention <= zeroCount {
		log.Printf("history: disabled (%s <= 0)", envHistoryRetention)

		return nil, noop
	}

	path, err := historyStorePath(os.Getenv(envHistoryDir))
	if err != nil {
		log.Printf("history: disabled: %v", err)

		return nil, noop
	}

	store, err := OpenHistoryStore(path)
	if err != nil {
		log.Printf("history: disabled: %v", err)

		return nil, noop
	}

	recorders := NewRecorderManager(store, retention)
	recorders.Start()

	// The default context is recorded from startup; others start when first
	// browsed (see historyAPI.recording).
	client, err := manager.ClientFor(emptyString)
	if err == nil {
		recorders.EnsureRecording(client)
	}

	//nolint:gosec // G706: the path is operator-set, never request input.
	log.Printf("history: recording to %s (retention %s)", path, retention)

	api := new(historyAPI)
	api.store = store
	api.recorders = recorders
	api.retention = retention

	stop := func() {
		recorders.Stop()

		err := store.Close()
		if err != nil {
			log.Printf(logHistoryError, err)
		}
	}

	return api, stop
}
