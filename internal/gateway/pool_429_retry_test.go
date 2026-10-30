package gateway

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/identity"
	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
)

// fakeKeyUpstream answers every request with the next scripted status and
// records whether the attempt asked for a non-reusable connection. A #pool
// transport only rotates its exit IP on a new connection, so the retry path
// must set req.Close on every attempt.
type fakeKeyUpstream struct {
	mu       sync.Mutex
	statuses []int
	calls    int
	closed   []bool
}

func (f *fakeKeyUpstream) roundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := f.statuses[min(f.calls, len(f.statuses)-1)]
	f.calls++
	f.closed = append(f.closed, req.Close)
	header := http.Header{}
	body := `{"id":"ok"}`
	statusText := "200 OK"
	if status == http.StatusTooManyRequests {
		header.Set("Retry-After", "60")
		body = `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached."}}`
		statusText = "429 Too Many Requests"
	}
	return &http.Response{
		StatusCode:    status,
		Status:        statusText,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func newRetryGateway(t *testing.T, statuses []int, pool bool) (*Gateway, *upstreamNodes, *fakeKeyUpstream) {
	t.Helper()
	fake := &fakeKeyUpstream{statuses: statuses}
	proxy := testProxy(0, "socks5://user:pass@127.0.0.1:8899#pool", false)
	proxy.pool = pool
	client := &http.Client{Transport: roundTripFunc(fake.roundTrip)}
	proxy.client = client
	proxy.bun = client
	transports := testTransports(proxy)
	nodes, err := nodePool([]string{"key-0123456789abcdef"}, transports, time.Minute, true)
	if err != nil {
		t.Fatalf("nodePool: %v", err)
	}
	// markZen429 keys off the tier label.
	nodes.tier = "zen"
	g := &Gateway{
		cfg: config.Config{
			Upstream: config.UpstreamConfig{Zen: "https://upstream.test"},
			Retry:    config.RetryConfig{MaxAttempts: 1},
		},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		transports: transports,
		zenNodes:   nodes,
		catalog:    models.Catalog(config.TierZen, nil),
	}
	return g, nodes, fake
}

func retryTestRequest(t *testing.T, g *Gateway) (*http.Response, error, int) {
	t.Helper()
	route := models.Route{
		ID:        "test-model",
		Tier:      config.TierZen,
		Protocol:  wire.Chat,
		Protocols: map[config.Tier]wire.Protocol{config.TierZen: wire.Chat},
	}
	body := map[config.Tier][]byte{
		config.TierZen: []byte(`{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`),
	}
	ids := identity.RequestIDs{Session: "ses-1", Request: "req-1", Project: "proj-1"}
	return g.doKeyUpstream(context.Background(), route, body, ids, 0)
}

func TestPoolKey429RetriesOnFreshExit(t *testing.T) {
	g, nodes, fake := newRetryGateway(t, []int{429, 200}, true)
	resp, err, attempts := retryTestRequest(t, g)
	if err != nil {
		t.Fatalf("doKeyUpstream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if fake.calls != pool429Attempts {
		t.Fatalf("upstream calls = %d, want %d", fake.calls, pool429Attempts)
	}
	if attempts != 1 {
		t.Fatalf("key attempts = %d, want 1", attempts)
	}
	for i, closed := range fake.closed {
		if !closed {
			t.Fatalf("attempt %d did not request a fresh connection", i)
		}
	}
	if got := nodes.nodes[0].cooldownUntil.Load(); got > time.Now().Unix() {
		t.Fatalf("key cooled after a successful retry: cooldown until %d", got)
	}
	if got := nodes.nodes[0].failures.Load(); got != 0 {
		t.Fatalf("failures = %d, want 0", got)
	}
}

func TestPoolKey429FreezesOnlyAfterTwoAttempts(t *testing.T) {
	g, nodes, fake := newRetryGateway(t, []int{429, 429}, true)
	resp, err, attempts := retryTestRequest(t, g)
	if err != nil {
		t.Fatalf("doKeyUpstream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if fake.calls != pool429Attempts {
		t.Fatalf("upstream calls = %d, want %d", fake.calls, pool429Attempts)
	}
	if attempts != 1 {
		t.Fatalf("key attempts = %d, want 1", attempts)
	}
	if got := nodes.nodes[0].cooldownUntil.Load(); got <= time.Now().Unix()+30 {
		t.Fatalf("key not cooled after the second 429: cooldown until %d", got)
	}
	if got := nodes.nodes[0].failures.Load(); got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
}

func TestStaticProxyKey429FreezesImmediately(t *testing.T) {
	g, nodes, fake := newRetryGateway(t, []int{429, 200}, false)
	resp, err, _ := retryTestRequest(t, g)
	if err != nil {
		t.Fatalf("doKeyUpstream: %v", err)
	}
	defer resp.Body.Close()
	if fake.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", fake.calls)
	}
	if got := nodes.nodes[0].cooldownUntil.Load(); got <= time.Now().Unix()+30 {
		t.Fatalf("key not cooled after a 429 on a static proxy: cooldown until %d", got)
	}
}

func TestPoolKey429StopsRetryingOnOtherOutcome(t *testing.T) {
	g, nodes, fake := newRetryGateway(t, []int{429, 500}, true)
	resp, err, _ := retryTestRequest(t, g)
	if err != nil {
		t.Fatalf("doKeyUpstream: %v", err)
	}
	defer resp.Body.Close()
	if fake.calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", fake.calls)
	}
	if got := nodes.nodes[0].cooldownUntil.Load(); got <= time.Now().Unix() {
		t.Fatalf("key not cooled after a 500: cooldown until %d", got)
	}
}

// TestPoolTransportOpensFreshConnectionPerAttempt pins the transport contract
// the 429 retries rely on: a #pool entry must not reuse a connection, or both
// tries would land on the same exit IP. HTTP/2 must stay off too, because a
// multiplexed CONNECT tunnel would also pin one exit.
func TestPoolTransportOpensFreshConnectionPerAttempt(t *testing.T) {
	transports, err := transportPool([]string{"socks5://user:pass@127.0.0.1:8899#pool"}, config.PerformanceConfig{}, 0)
	if err != nil {
		t.Fatalf("transportPool: %v", err)
	}
	tr, ok := transports.items[0].client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", transports.items[0].client.Transport)
	}
	if !tr.DisableKeepAlives {
		t.Fatal("pool transport must disable keep-alives")
	}
	if tr.ForceAttemptHTTP2 || len(tr.TLSNextProto) != 0 {
		t.Fatal("pool transport must not negotiate HTTP/2")
	}
	if tr.TLSClientConfig == nil || len(tr.TLSClientConfig.NextProtos) != 1 || tr.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatalf("pool transport ALPN = %v, want [http/1.1]", tr.TLSClientConfig)
	}
}
