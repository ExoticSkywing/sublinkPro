package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDistributionGeoIPSettings(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyPath, []byte("test-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUBLINK_IPDATA_API_KEY_FILE", keyPath)
	t.Setenv("SUBLINK_IP2REGION_V4_PATH", "/data/v4.xdb")
	t.Setenv("SUBLINK_IP2REGION_V6_PATH", "/data/v6.xdb")
	t.Setenv("SUBLINK_IPDATA_DAILY_LIMIT", "37")
	// Never inherit a developer's real API key in tests.
	original, exists := os.LookupEnv("SUBLINK_IPDATA_API_KEY")
	if err := os.Unsetenv("SUBLINK_IPDATA_API_KEY"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv("SUBLINK_IPDATA_API_KEY", original)
		} else {
			_ = os.Unsetenv("SUBLINK_IPDATA_API_KEY")
		}
	})
	cfg := GetDistributionGeoIPSettings()
	if cfg.APIKey != "test-key" || cfg.DailyLimit != 37 || cfg.IPv4Path != "/data/v4.xdb" || cfg.IPv6Path != "/data/v6.xdb" {
		t.Fatal("file config not loaded")
	}
	t.Setenv("SUBLINK_IPDATA_API_KEY", "override")
	if GetDistributionGeoIPSettings().APIKey != "override" {
		t.Fatal("environment precedence")
	}
	t.Setenv("SUBLINK_IPDATA_API_KEY", "")
	if GetDistributionGeoIPSettings().APIKey != "" {
		t.Fatal("explicit empty key must disable")
	}
	t.Setenv("SUBLINK_IPDATA_DAILY_LIMIT", "0")
	if GetDistributionGeoIPSettings().DailyLimit != 0 {
		t.Fatal("zero must disable external queries")
	}
}
