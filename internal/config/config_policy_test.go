package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProxyPolicyDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server_keys":["local"],"anonymous":true}`), 0o600); err != nil {
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
}
