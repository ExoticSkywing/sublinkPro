package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DistributionGeoIPSettings are deployment-only options, deliberately excluded
// from AppConfig serialization and all browser-facing settings responses.
type DistributionGeoIPSettings struct {
	IPv4Path, IPv6Path string
	APIKey             string
	DailyLimit         int
}

func GetDistributionGeoIPSettings() DistributionGeoIPSettings {
	path := func(env, fallback string) string {
		if value := strings.TrimSpace(os.Getenv(envPrefix + env)); value != "" {
			return value
		}
		return filepath.Join(GetDBPath(), fallback)
	}
	cfg := DistributionGeoIPSettings{
		IPv4Path:   path("IP2REGION_V4_PATH", "ip2region_v4.xdb"),
		IPv6Path:   path("IP2REGION_V6_PATH", "ip2region_v6.xdb"),
		DailyLimit: 500,
	}
	if key, present := os.LookupEnv(envPrefix + "IPDATA_API_KEY"); present {
		cfg.APIKey = strings.TrimSpace(key)
	} else if key, err := os.ReadFile(path("IPDATA_API_KEY_FILE", "secrets/ipdata-api-key")); err == nil {
		cfg.APIKey = strings.TrimSpace(string(key))
	}
	if value, err := strconv.Atoi(os.Getenv(envPrefix + "IPDATA_DAILY_LIMIT")); err == nil && value >= 0 {
		cfg.DailyLimit = value
	}
	return cfg
}
