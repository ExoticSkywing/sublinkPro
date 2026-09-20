package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sublink/services/distribution"
	"sublink/services/geoip"

	"github.com/gin-gonic/gin"
)

const loonTestUA = "Loon/975 CFNetwork/1402.0.4 Darwin/22.2.0"

func TestDistributionOneCityLimitAcrossClients(t *testing.T) {
	for _, client := range []struct{ ua, format string }{
		{loonTestUA, "v2ray"}, {"Clash/1.0", "clash"}, {"Shadowrocket/1.0", "v2ray"},
	} {
		t.Run(client.ua, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			cfg, _ := s.Settings(ctx)
			cfg.RegionLimit = 1
			if err := s.SaveSettings(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			first := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			second := geoip.CityLocation{Key: "CN:2", Country: "CN", City: "苏州"}
			for _, tc := range []struct {
				city    geoip.CityLocation
				allowed bool
			}{{first, true}, {second, false}, {first, true}} {
				nodes := distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", client.ua, "", tc.city), client.format)
				if len(nodes) == 0 {
					t.Fatal("response has no parsable nodes")
				}
				for _, node := range nodes {
					if tc.allowed && node.Name != "Private node" {
						t.Fatal("registered city lost resources")
					}
					if !tc.allowed && node.Server != "placeholder.invalid" {
						t.Fatal("second city received real resources")
					}
				}
			}
			state, err := s.PublicStatus(ctx, cred.Token)
			if err != nil || state.RegionLimit != 1 || len(state.Regions) != 1 || len(state.Candidates) != 1 {
				t.Fatal("denied city was bound or missing from application candidates", err)
			}
		})
	}
}

func TestDistributionLoonAutoMatchesLegacyAndExplicitConversion(t *testing.T) {
	s, cred := distributionFixture(t)
	ctx := context.Background()
	city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
	// An explicitly requested Loon config still requires the converter; failure
	// must not activate a trial or reserve a city.
	response := pullDistribution(s, cred.Token, "GET", loonTestUA, "loon", city)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("explicit Loon unexpectedly skipped conversion: %d", response.Code)
	}
	stored, err := s.ByID(ctx, cred.ID)
	if err != nil || stored.ActivatedAt != nil || len(stored.Regions) != 0 {
		t.Fatal("failed conversion activated or reserved a city", err)
	}

	r := gin.New()
	r.GET("/c/", GetClient)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/c/?token=legacy-test-token", nil)
	req.Header.Set("User-Agent", loonTestUA)
	legacy := httptest.NewRecorder()
	r.ServeHTTP(legacy, req)
	oldNodes := distributionTestNodes(t, legacy, "v2ray")
	newNodes := distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", loonTestUA, "", city), "v2ray")
	if len(oldNodes) != 1 || len(newNodes) != 1 || newNodes[0].Name != oldNodes[0].Name || newNodes[0].Server != oldNodes[0].Server {
		t.Fatal("Loon auto did not preserve the legacy native node list")
	}
}

func TestDistributionLoonAutoAndNativeOverridesEnforcePolicy(t *testing.T) {
	for _, client := range []string{"", "clash", "mihomo", "surge", "v2ray"} {
		t.Run("client="+client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			output := client
			if output == "" {
				output = "v2ray"
			}
			assertNodes := func(location geoip.CityLocation, allowed bool) {
				t.Helper()
				nodes := distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", loonTestUA, client, location), output)
				if len(nodes) == 0 {
					t.Fatal("no parsable nodes")
				}
				for _, node := range nodes {
					if allowed && node.Name != "Private node" {
						t.Fatal("valid refresh did not receive resource nodes")
					}
					if !allowed && node.Server != "placeholder.invalid" {
						t.Fatal("restricted refresh leaked real resources")
					}
				}
			}
			for _, ua := range []string{"Mozilla/5.0", "curl/8"} {
				want := http.StatusForbidden
				if ua == "Mozilla/5.0" {
					want = http.StatusFound
				}
				if res := pullDistribution(s, cred.Token, "GET", ua, client, city); res.Code != want {
					t.Fatalf("format override bypassed UA check: %d", res.Code)
				}
			}
			if res := pullDistribution(s, cred.Token, "HEAD", loonTestUA, client, city); res.Code != http.StatusOK || res.Body.Len() != 0 {
				t.Fatal("HEAD did not remain a non-delivery")
			}
			assertNodes(geoip.CityLocation{Country: "US"}, false)
			assertNodes(geoip.CityLocation{Country: "CN"}, false)
			stored, err := s.ByID(ctx, cred.ID)
			if err != nil || stored.ActivatedAt != nil || len(stored.Regions) != 0 {
				t.Fatal("non-delivery activated or reserved a city", err)
			}
			assertNodes(city, true)
			stored, err = s.ByID(ctx, cred.ID)
			if err != nil || stored.ActivatedAt == nil || stored.ExpiresAt == nil || len(stored.Regions) != 1 {
				t.Fatal("valid Loon refresh did not activate", err)
			}
			assertNodes(geoip.CityLocation{Key: "CN:2", Country: "CN", City: "苏州"}, true)
			assertNodes(geoip.CityLocation{Key: "CN:3", Country: "CN", City: "杭州"}, false)
			s.Now = func() time.Time { return stored.ExpiresAt.Add(time.Hour) }
			assertNodes(city, false)
			s.Now = time.Now
			status := "disabled"
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Status: &status}); err != nil {
				t.Fatal(err)
			}
			assertNodes(city, false)
			status = "revoked"
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Status: &status}); err != nil {
				t.Fatal(err)
			}
			assertNodes(city, false)
		})
	}
}
