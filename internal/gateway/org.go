package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"opencode2api/internal/config"
)

// consoleConfigURL is the endpoint the official client reads its workspace
// header from before its first model request. The free tier attributes a
// keyed request to the workspace named in x-opencode-org-id, so a key whose
// console lookup failed is not indistinguishable from the anonymous lane.
const consoleConfigURL = "https://opencode.ai/console/api/v2/config"

// consoleConfig is the subset of the console configuration the gateway needs:
// the workspace header is nested under the provider entry, and the provider
// name may change, so every provider is inspected.
type consoleConfig struct {
	Providers map[string]struct {
		Headers map[string]string `json:"headers"`
	} `json:"providers"`
}

// fetchOrgIDFrom reads the workspace id the console associates with key. It is
// split from fetchOrgID so tests can serve the response locally.
func fetchOrgIDFrom(ctx context.Context, client *http.Client, endpoint, key string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "opencode/latest/2.1.4/cli")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("console config: HTTP %d", resp.StatusCode)
	}
	var payload consoleConfig
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("console config: %w", err)
	}
	for _, provider := range payload.Providers {
		if org := strings.TrimSpace(provider.Headers["x-opencode-org-id"]); org != "" {
			return org, nil
		}
	}
	return "", errors.New("console config has no x-opencode-org-id")
}

// LoadOrgIDs resolves the workspace of every key before the pool serves
// traffic. Lookups run in parallel and are best effort: a key without a
// resolved workspace still serves, but its free-tier requests cannot be
// attributed to the workspace and may be treated as anonymous.
func (p *upstreamNodes) LoadOrgIDs(logger *slog.Logger) {
	if p == nil || len(p.nodes) == 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	client := &http.Client{Timeout: 5 * time.Second}
	var wg sync.WaitGroup
	for _, node := range p.nodes {
		wg.Add(1)
		go func(node *upstreamNode) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			org, err := fetchOrgIDFrom(ctx, client, consoleConfigURL, node.key)
			if err != nil {
				logger.Debug("console workspace lookup failed", "component", "upstream", "event", "org_lookup_failed", "key_id", config.KeyDisplayID(node.key), "error", err)
				return
			}
			node.org = org
			logger.Debug("console workspace resolved", "component", "upstream", "event", "org_resolved", "key_id", config.KeyDisplayID(node.key), "org", org)
		}(node)
	}
	wg.Wait()
}
