package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
	"opencode2api/internal/identity"
	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
)

func testProxy(index int, name string, direct bool) *proxyTransport {
	proxy := &proxyTransport{index: index, name: name, direct: direct}
	proxy.healthy.Store(true)
	return proxy
}

func testTransports(proxies ...*proxyTransport) *proxyTransports {
	return &proxyTransports{items: proxies}
}

func TestNodeCursorSkipsCoolingAndBlockedKeys(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	pool, err := nodePool([]string{"key-a", "key-b"}, testTransports(proxy), time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pool.nodes[0].cooldownUntil.Store(time.Now().Add(time.Hour).UnixNano())
	cursor := pool.Cursor()
	if got := cursor.Next(); got == nil || got.key != "key-b" {
		t.Fatalf("cooling key was not skipped: %+v", got)
	}
	pool.nodes[0].cooldownUntil.Store(time.Now().Add(-time.Second).UnixNano())
	cursor = pool.Cursor()
	cursor.next = 0
	if got := cursor.Next(); got == nil {
		t.Fatal("expired cooldown did not reactivate the key")
	}
	proxy.rateBlockedUntil.Store(time.Now().Add(time.Hour).UnixNano())
	cursor = pool.Cursor()
	if got := cursor.Next(); got != nil {
		t.Fatalf("key on a blocked proxy was selected: %+v", got)
	}
	if pool.hasUsableKeyedProxy(time.Now()) {
		t.Fatal("blocked proxy still reported a usable key")
	}
}

func TestNodeCursorRequiresARealProxy(t *testing.T) {
	direct := testProxy(0, "direct", true)
	pool, err := nodePool([]string{"key-a"}, testTransports(direct), time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	cursor := pool.Cursor()
	if got := cursor.Next(); got != nil {
		t.Fatalf("direct transport was used while require_proxy is set: %+v", got)
	}
	if pool.hasUsableKeyedProxy(time.Now()) {
		t.Fatal("direct transport counted as a proxy")
	}
}

func TestNodePoolBindsKeysToRealProxyWhenRequired(t *testing.T) {
	direct := testProxy(0, "direct", true)
	real := testProxy(1, "http://proxy", false)
	pool, err := nodePool([]string{"key-a"}, testTransports(direct, real), time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := int(pool.nodes[0].proxyIndex.Load()); got != 1 {
		t.Fatalf("key bound to proxy %d, want real proxy 1", got)
	}
	cursor := pool.Cursor()
	if got := cursor.Next(); got == nil {
		t.Fatal("key on the real proxy was not selectable")
	}
}

func TestProxyRateBlock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &Gateway{logger: logger, proxyBlockThreshold: 2, proxyBlockDuration: time.Minute}
	proxy := testProxy(0, "http://proxy", false)
	g.observeProxyStatus(proxy, http.StatusTooManyRequests)
	if proxy.rateBlocked(time.Now()) {
		t.Fatal("proxy blocked before reaching the threshold")
	}
	g.observeProxyStatus(proxy, http.StatusOK)
	if proxy.rateFailures.Load() != 0 {
		t.Fatal("a non-429/403 response did not clear the streak")
	}
	g.observeProxyStatus(proxy, http.StatusTooManyRequests)
	g.observeProxyStatus(proxy, http.StatusForbidden)
	if !proxy.rateBlocked(time.Now()) {
		t.Fatal("proxy was not blocked at the threshold")
	}
	if proxy.rateFailures.Load() != 0 {
		t.Fatal("block did not reset the consecutive counter")
	}
}

func TestProxyRateBlockExpires(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	proxy.rateBlockedUntil.Store(time.Now().Add(-time.Second).UnixNano())
	if proxy.rateBlocked(time.Now()) {
		t.Fatal("expired proxy block still reported blocked")
	}
	if !proxy.available(time.Now(), false) {
		t.Fatal("expired block did not reactivate the proxy")
	}
}

func TestCopyErrorResponseRewritesUpstreamBodies(t *testing.T) {
	for _, test := range []struct {
		status  int
		message string
	}{
		{http.StatusForbidden, "暂时不可用"},
		{http.StatusTooManyRequests, "当前 rate limit"},
	} {
		rec := httptest.NewRecorder()
		resp := &http.Response{
			StatusCode: test.status,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"OpenCode internal detail"}}`)),
		}
		copyErrorResponse(rec, wire.Chat, resp, "req_test")
		if rec.Code != test.status {
			t.Fatalf("status = %d, want %d", rec.Code, test.status)
		}
		if !strings.Contains(rec.Body.String(), test.message) {
			t.Fatalf("body %q misses %q", rec.Body.String(), test.message)
		}
		if strings.Contains(rec.Body.String(), "OpenCode internal detail") {
			t.Fatalf("upstream body leaked: %q", rec.Body.String())
		}
	}
}

func TestWriteKeysCoolingError(t *testing.T) {
	rec := httptest.NewRecorder()
	err := &keysCoolingError{until: time.Now().Add(30 * time.Second)}
	if !writeKeysCoolingError(rec, wire.Chat, err, "req_test") {
		t.Fatal("cooling error was not handled")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "当前 rate limit") {
		t.Fatalf("body %q misses the local rate-limit message", rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After was not set from the cooling deadline")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestKeyPoolStopsAfterAllKeysCool(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	transports := testTransports(proxy)
	pool, err := nodePool([]string{"key-a", "key-b"}, transports, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	proxy.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"upstream rate limit"}}`)),
			Request:    req,
		}, nil
	})}
	g := &Gateway{
		cfg:    config.Config{Upstream: config.UpstreamConfig{Zen: "https://example.invalid/zen"}, Retry: config.RetryConfig{MaxAttempts: 5}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), transports: transports, zenNodes: pool,
		catalog: models.Catalog(config.TierZen, nil),
	}
	route := models.Route{ID: "paid-model", Tier: config.TierZen, Protocol: wire.Chat}
	bodies := map[config.Tier][]byte{config.TierZen: []byte(`{"model":"paid-model","messages":[],"stream":false}`)}
	ids := identity.RequestIDs{Session: "ses_test"}
	resp, err, attempts := g.doKeyUpstream(context.Background(), route, bodies, ids, 0)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("first response = %+v, want 429", resp)
	}
	httpx.DrainAndClose(resp.Body)
	if attempts != 2 || calls != 2 {
		t.Fatalf("first request attempts=%d calls=%d, want 2/2", attempts, calls)
	}
	resp, err, attempts = g.doKeyUpstream(context.Background(), route, bodies, ids, 0)
	if resp != nil || attempts != 0 {
		t.Fatalf("second request response=%+v attempts=%d, want nil/0", resp, attempts)
	}
	if _, ok := err.(*keysCoolingError); !ok {
		t.Fatalf("second request error = %T %v, want keysCoolingError", err, err)
	}
}

func TestKeyPoolNoProxyAfterProxyBlock(t *testing.T) {
	proxy := testProxy(0, "http://proxy", false)
	transports := testTransports(proxy)
	pool, err := nodePool([]string{"key-a"}, transports, time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	proxy.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"upstream rate limit"}}`)),
			Request:    req,
		}, nil
	})}
	g := &Gateway{
		cfg: config.Config{
			Upstream:    config.UpstreamConfig{Zen: "https://example.invalid/zen"},
			Retry:       config.RetryConfig{MaxAttempts: 1},
			Performance: config.PerformanceConfig{RequireProxy: true},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), transports: transports, zenNodes: pool,
		catalog: models.Catalog(config.TierZen, nil), proxyBlockThreshold: 1, proxyBlockDuration: time.Hour,
	}
	route := models.Route{ID: "paid-model", Tier: config.TierZen, Protocol: wire.Chat}
	bodies := map[config.Tier][]byte{config.TierZen: []byte(`{"model":"paid-model","messages":[],"stream":false}`)}
	resp, err, _ := g.doKeyUpstream(context.Background(), route, bodies, identity.RequestIDs{Session: "ses_test"}, 0)
	if err != nil || resp == nil {
		t.Fatalf("first request = %+v, %v", resp, err)
	}
	httpx.DrainAndClose(resp.Body)
	if !proxy.rateBlocked(time.Now()) {
		t.Fatal("proxy was not blocked after the threshold")
	}
	resp, err, attempts := g.doKeyUpstream(context.Background(), route, bodies, identity.RequestIDs{Session: "ses_test"}, 0)
	if resp != nil || attempts != 0 {
		t.Fatalf("second request response=%+v attempts=%d, want nil/0", resp, attempts)
	}
	if !errors.Is(err, errProxyUnavailable) {
		t.Fatalf("second request error = %v, want errProxyUnavailable", err)
	}
}

func TestAnonymousCursorSkipsDirectWhenProxyRequired(t *testing.T) {
	direct := testProxy(0, "direct", true)
	real := testProxy(1, "http://proxy", false)
	pool := anonymousPool(true, testTransports(direct, real), time.Minute, anonymousBackoff{}, true)
	for range 2 {
		cursor := pool.CursorFor("")
		node := cursor.Next()
		if node == nil {
			t.Fatal("anonymous cursor returned no node")
		}
		if node.proxy.direct {
			t.Fatal("anonymous cursor selected direct while require_proxy is set")
		}
	}
}
