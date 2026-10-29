package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"opencode2api/internal/config"
)

func test429(retryAfter string) *http.Response {
	header := http.Header{}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: http.StatusTooManyRequests, Header: header}
}

func cooldownRemaining(node *upstreamNode) time.Duration {
	return time.Until(time.Unix(0, node.cooldownUntil.Load()))
}

func TestZen429UsesRetryAfterAndPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
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
	pool.bindKeyState(store, "zen")
	node := pool.nodes[0]

	pool.MarkFailure(node, test429("3600"), nil)
	if remaining := cooldownRemaining(node); remaining < 59*time.Minute || remaining > 61*time.Minute {
		t.Fatalf("cooldown = %s, want about one hour", remaining)
	}
	if node.failures.Load() != 1 {
		t.Fatalf("failures = %d, want 1", node.failures.Load())
	}

	if err := store.close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openKeyStateStore(configPath, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	reloaded, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.bindKeyState(reopened, "zen")
	loaded := reloaded.nodes[0]
	if remaining := cooldownRemaining(loaded); remaining < 59*time.Minute || remaining > 61*time.Minute {
		t.Fatalf("persisted cooldown = %s, want about one hour", remaining)
	}
	reloaded.MarkSuccess(loaded)
	if loaded.cooldownUntil.Load() != 0 || loaded.failures.Load() != 0 {
		t.Fatal("success did not clear the cooldown and failure count")
	}
}

func TestZen429WithoutRetryAfterUsesDefault(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pool.bindKeyState(nil, "zen")
	node := pool.nodes[0]
	pool.MarkFailure(node, test429(""), nil)
	if remaining := cooldownRemaining(node); remaining < 55*time.Second || remaining > 65*time.Second {
		t.Fatalf("cooldown = %s, want about 60 seconds", remaining)
	}
}

func TestZen429RetryAfterHTTPDate(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pool.bindKeyState(nil, "zen")
	node := pool.nodes[0]
	pool.MarkFailure(node, test429(time.Now().Add(2*time.Hour).UTC().Format(http.TimeFormat)), nil)
	if remaining := cooldownRemaining(node); remaining < 119*time.Minute || remaining > 121*time.Minute {
		t.Fatalf("cooldown = %s, want about two hours", remaining)
	}
}

func TestZen429RetryAfterReplacesCooldown(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{"zen-key-1234567890"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pool.bindKeyState(nil, "zen")
	node := pool.nodes[0]
	pool.MarkFailure(node, test429("3600"), nil)
	pool.MarkFailure(node, test429("120"), nil)
	if remaining := cooldownRemaining(node); remaining < 110*time.Second || remaining > 130*time.Second {
		t.Fatalf("cooldown = %s, want about two minutes", remaining)
	}
}

func TestLegacyZenPolicyStateIsIgnored(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := openKeyStateStore("", logger)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	key := "zen-key-1234567890"
	_, err = store.db.Exec(`INSERT INTO upstream_key_state
		(tier, fingerprint, short_freeze_until, long_freeze_until, quota_exhausted,
		 consecutive_429, window_start, failures, updated_at)
		VALUES ('zen', ?, 0, ?, 1, 3, ?, 4, ?)`,
		config.Fingerprint(key), time.Now().Add(24*time.Hour).UnixNano(), time.Now().UnixNano(), time.Now().UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{key}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pool.bindKeyState(store, "zen")
	if pool.nodes[0].cooldownUntil.Load() != 0 {
		t.Fatal("legacy long freeze became a Retry-After cooldown")
	}
	if !pool.hasReadyKey(time.Now()) {
		t.Fatal("legacy quota marker still blocks the key")
	}
}
