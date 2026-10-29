package gateway

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// keyState is the persisted scheduling state of one upstream key. It is
// keyed by tier and key fingerprint, never by the key value. CooldownUntil is
// set from the upstream Retry-After header on a Zen 429.
type keyState struct {
	CooldownUntil int64
	Failures      uint32
}

// keyStateStore persists key state in SQLite beside the configuration file,
// so a process restart does not resurrect a frozen key.
type keyStateStore struct {
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

func openKeyStateStore(configPath string, logger *slog.Logger) (*keyStateStore, error) {
	path := ":memory:"
	persistent := configPath != ""
	if persistent {
		path = configPath + ".keys.db"
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create key state directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open key state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if persistent {
		if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure key state database: %w", err)
		}
	}
	// short_freeze_until is the historical column name for the key cooldown.
	// The other 429 policy columns remain for existing databases and are always
	// written as zero; current scheduling uses only the Retry-After cooldown.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS upstream_key_state (
		tier TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		short_freeze_until INTEGER NOT NULL DEFAULT 0,
		long_freeze_until INTEGER NOT NULL DEFAULT 0,
		quota_exhausted INTEGER NOT NULL DEFAULT 0,
		consecutive_429 INTEGER NOT NULL DEFAULT 0,
		window_start INTEGER NOT NULL DEFAULT 0,
		failures INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (tier, fingerprint)
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create key state table: %w", err)
	}
	return &keyStateStore{db: db, logger: logger}, nil
}

func (s *keyStateStore) load(tier, fingerprint string) (keyState, bool, error) {
	if s == nil {
		return keyState{}, false, nil
	}
	var state keyState
	err := s.db.QueryRow(`SELECT short_freeze_until, failures FROM upstream_key_state
		WHERE tier = ? AND fingerprint = ?`, tier, fingerprint).
		Scan(&state.CooldownUntil, &state.Failures)
	if err == sql.ErrNoRows {
		return keyState{}, false, nil
	}
	if err != nil {
		return keyState{}, false, err
	}
	return state, true, nil
}

func (s *keyStateStore) save(tier, fingerprint string, state keyState) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO upstream_key_state
		(tier, fingerprint, short_freeze_until, long_freeze_until, quota_exhausted,
		 consecutive_429, window_start, failures, updated_at)
		VALUES (?, ?, ?, 0, 0, 0, 0, ?, ?)
		ON CONFLICT(tier, fingerprint) DO UPDATE SET
			short_freeze_until=excluded.short_freeze_until,
			long_freeze_until=0,
			quota_exhausted=0,
			consecutive_429=0,
			window_start=0,
			failures=excluded.failures,
			updated_at=excluded.updated_at`,
		tier, fingerprint, state.CooldownUntil, state.Failures, time.Now().UnixNano())
	return err
}

func (s *keyStateStore) delete(tier, fingerprint string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM upstream_key_state WHERE tier = ? AND fingerprint = ?`, tier, fingerprint)
	return err
}

func (s *keyStateStore) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
