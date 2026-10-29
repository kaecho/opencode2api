package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProxyPolicyDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server_keys":["local"],"anonymous":true,"performance":{"zen_key_short_freeze_seconds":3600,"zen_key_long_freeze_seconds":86400,"zen_key_429_threshold":2,"zen_key_429_window_seconds":86400,"max_client_tools":17,"zero_width_keywords":["AI agent"," AI agent ",""]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Performance.ProxyBlockThreshold != 3 {
		t.Fatalf("proxy_block_threshold = %d, want 3", cfg.Performance.ProxyBlockThreshold)
	}
	if cfg.Performance.ProxyBlockSeconds != 900 {
		t.Fatalf("proxy_block_seconds = %d, want 900", cfg.Performance.ProxyBlockSeconds)
	}
	if cfg.Performance.RequireProxy {
		t.Fatal("require_proxy defaulted to true")
	}
	if cfg.Performance.MaxClientTools != 17 || len(cfg.Performance.ZeroWidthKeywords) != 1 || cfg.Performance.ZeroWidthKeywords[0] != "AI agent" {
		t.Fatalf("sanitizer config not normalized: tools=%d keywords=%v", cfg.Performance.MaxClientTools, cfg.Performance.ZeroWidthKeywords)
	}
}
