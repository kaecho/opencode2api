package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func test429() *http.Response {
	return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
}

func TestZen429FreezePolicyPersists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := openKeyStateStore(configPath, logger)
	if err != nil {
		t.Fatal(err)
	}
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	policy := keyFreezePolicy{shortFreeze: 10 * time.Minute, longFreeze: 24 * time.Hour, threshold: 3, window: 24 * time.Hour}
	pool.bindKeyState(store, "zen", policy)
	node := pool.nodes[0]

	pool.MarkFailure(node, test429(), nil)
	if node.cooldownUntil.Load() <= time.Now().UnixNano() {
		t.Fatal("first 429 did not apply a short freeze")
	}
	if node.consecutive429.Load() != 1 || node.quotaExhausted.Load() {
		t.Fatalf("first 429 count=%d quota=%v", node.consecutive429.Load(), node.quotaExhausted.Load())
	}
	pool.MarkFailure(node, test429(), nil)
	pool.MarkFailure(node, test429(), nil)
	if node.longFreezeUntil.Load() <= time.Now().UnixNano() {
		t.Fatal("third 429 did not apply a long freeze")
	}
	if !node.quotaExhausted.Load() || node.cooldownUntil.Load() != 0 {
		t.Fatalf("quota=%v short=%d after threshold", node.quotaExhausted.Load(), node.cooldownUntil.Load())
	}

	// A fresh store and pool loaded from disk must see the long freeze.
	if err := store.close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openKeyStateStore(configPath, logger)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.bindKeyState(reopened, "zen", policy)
	loaded := reloaded.nodes[0]
	if !loaded.quotaExhausted.Load() || loaded.longFreezeUntil.Load() <= time.Now().UnixNano() {
		t.Fatalf("persisted quota state missing: quota=%v long=%d", loaded.quotaExhausted.Load(), loaded.longFreezeUntil.Load())
	}

	reloaded.MarkSuccess(loaded)
	if loaded.quotaExhausted.Load() || loaded.longFreezeUntil.Load() != 0 || loaded.consecutive429.Load() != 0 {
		t.Fatal("success did not clear the persisted freeze state")
	}
	_ = reopened.close()
}

func TestZen429WindowResets(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pool.bindKeyState(nil, "zen", keyFreezePolicy{
		shortFreeze: 10 * time.Minute, longFreeze: 24 * time.Hour, threshold: 3, window: 24 * time.Hour,
	})
	node := pool.nodes[0]
	node.windowStart.Store(time.Now().Add(-25 * time.Hour).UnixNano())
	node.consecutive429.Store(2)
	pool.MarkFailure(node, test429(), nil)
	if node.consecutive429.Load() != 1 {
		t.Fatalf("window reset count=%d, want 1", node.consecutive429.Load())
	}
	if node.longFreezeUntil.Load() != 0 {
		t.Fatal("expired window still triggered a long freeze")
	}
}
