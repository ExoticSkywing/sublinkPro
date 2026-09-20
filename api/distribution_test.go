package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"sublink/database"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/services/distribution"
	"sublink/services/geoip"
)

func distributionTestNodes(t *testing.T, response *httptest.ResponseRecorder, client string) []protocol.Proxy {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("client %s: status %d: %s", client, response.Code, response.Body.String())
	}
	var nodes []protocol.Proxy
	switch client {
	case "clash", "mihomo":
		var output struct {
			Proxies []protocol.Proxy `yaml:"proxies"`
		}
		if err := yaml.Unmarshal(response.Body.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		nodes = output.Proxies
	case "v2ray":
		body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(response.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		for _, link := range strings.Split(strings.TrimSpace(string(body)), "\n") {
			node, err := protocol.LinkToProxy(protocol.Urls{Url: link}, protocol.OutputConfig{})
			if err != nil {
				t.Fatal(err)
			}
			nodes = append(nodes, node)
		}
	case "surge":
		for _, line := range strings.Split(response.Body.String(), "\n") {
			name, value, ok := strings.Cut(line, " = ss, ")
			if ok {
				nodes = append(nodes, protocol.Proxy{Name: name, Server: strings.Split(value, ",")[0]})
			}
		}
	}
	return nodes
}

func TestDistributionMultipleExpiryNoticesAndFallbackPolicy(t *testing.T) {
	for _, client := range []string{"clash", "mihomo", "surge", "v2ray"} {
		t.Run(client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			_ = pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			_ = pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, geoip.CityLocation{Key: "CN:2", Country: "CN", City: "苏州"})
			active, err := s.ByID(ctx, cred.ID)
			if err != nil || active.ExpiresAt == nil {
				t.Fatal("activation failed", err)
			}
			s.Now = func() time.Time { return active.ExpiresAt.Add(time.Hour) }
			cfg, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cfg.PortalURL = "https://renew.example/"
			cfg.Domain = "subscription.example"
			cfg.ExpiredMessages = []string{"订阅已到期", "请领取卡密续订", "{portal}"}
			if err := s.SaveSettings(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			response := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			nodes := distributionTestNodes(t, response, client)
			var names []string
			for _, node := range nodes {
				if node.Server != "placeholder.invalid" {
					t.Fatal("expiry leaked real resource", node.Server)
				}
				names = append(names, node.Name)
			}
			if !reflect.DeepEqual(names, []string{"订阅已到期", "请领取卡密续订", cfg.PortalURL}) {
				t.Fatalf("notice order/content: %q", names)
			}
			if response.Header().Get("profile-title") != "base64:"+base64.StdEncoding.EncodeToString([]byte(cred.Name)) {
				t.Fatal("profile identity changed")
			}
			// Even with selected fallback resources, denied IPs must get only the denial notice.
			createClientSubscriptionFixture(t, writeTestClashTemplate(t), writeTestSurgeTemplate(t), "fallback", "fallback-share", "FallbackNode")
			var fallback models.Subcription
			if err := database.DB.Where("name = ?", "fallback").First(&fallback).Error; err != nil {
				t.Fatal(err)
			}
			cfg.FallbackSubscriptionID = fallback.ID
			if err := s.SaveSettings(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			nodes = distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city), client)
			if len(nodes) != 4 || nodes[3].Name != "FallbackNode" {
				t.Fatalf("fallback must follow all notice nodes: %+v", nodes)
			}
			for _, denied := range []geoip.CityLocation{{Country: "US"}, {Country: "CN"}, {Key: "CN:3", Country: "CN", City: "杭州"}} {
				nodes = distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, denied), client)
				if len(nodes) < 2 || nodes[0].Name == cfg.ExpiredMessages[0] {
					t.Fatal("denied access bypassed expiry policy", nodes)
				}
				for _, node := range nodes {
					if node.Server != "placeholder.invalid" {
						t.Fatal("denied access leaked resources")
					}
				}
			}
			cards, err := s.CreateCards(ctx, distribution.CardInput{Name: "renewal", Kind: "quarter", Count: 1})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Redeem(ctx, cred.Token, cards[0].Code, "api"); err != nil {
				t.Fatal(err)
			}
			nodes = distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city), client)
			if len(nodes) != 1 || nodes[0].Name != "Private node" {
				t.Fatal("renewal did not remove notices and fallback", nodes)
			}
		})
	}
}

func TestDistributionExpiryLegacyAndSpecialNames(t *testing.T) {
	for _, client := range []string{"clash", "v2ray"} {
		t.Run(client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			_ = pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			active, _ := s.ByID(ctx, cred.ID)
			s.Now = func() time.Time { return active.ExpiresAt.Add(time.Hour) }
			cfg, _ := s.Settings(ctx)
			nodes := distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city), client)
			if len(nodes) != 2 || nodes[0].Name != cfg.ExpiredMessage || nodes[1].Name != "https://distribution.test/" {
				t.Fatal("legacy notice not separated", nodes)
			}
			for _, messages := range [][]string{{"只显示一条"}, {"续订, # + % ? & 中文", "同名", "同名"}} {
				cfg.ExpiredMessages = messages
				if err := s.SaveSettings(ctx, cfg); err != nil {
					t.Fatal(err)
				}
				nodes = distributionTestNodes(t, pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city), client)
				if len(nodes) != len(messages) || nodes[0].Name != messages[0] {
					t.Fatal("message was split or corrupted", nodes)
				}
				if len(nodes) == 3 && nodes[1].Name == nodes[2].Name {
					t.Fatal("duplicate node names")
				}
			}
		})
	}
}

func distributionFixture(t *testing.T) (*distribution.Store, distribution.Credential) {
	t.Helper()
	setupClientsAPITestDB(t)
	createClientSubscriptionFixture(t, writeTestClashTemplate(t), writeTestSurgeTemplate(t), "distribution", "legacy-test-token", "Private node")
	var sub models.Subcription
	if err := database.DB.Where("name = ?", "distribution").First(&sub).Error; err != nil {
		t.Fatal(err)
	}
	// GORM's true default overrides false during Create. Persist the intended
	// fixture switch explicitly, including its write-through subscription cache.
	sub.RefreshUsageOnRequest = false
	if err := sub.Update(); err != nil {
		t.Fatal(err)
	}
	s := distribution.New(database.DB, "test-key")
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Issue(context.Background(), distribution.IssueInput{Name: "Recipient", SubscriptionID: sub.ID, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	return s, rows[0]
}

func pullDistribution(s *distribution.Store, token, method, ua, client string, city geoip.CityLocation) *httptest.ResponseRecorder {
	r := gin.New()
	handler := func(c *gin.Context) {
		distributionClient(c, s, func(string) (geoip.CityLocation, error) { return city, nil })
	}
	r.GET("/d/:token", handler)
	r.HEAD("/d/:token", handler)
	request := httptest.NewRequestWithContext(context.Background(), method, "/d/"+token+"?client="+client, nil)
	request.Host = "distribution.test"
	request.Header.Set("User-Agent", ua)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}

func TestDistributionBrowserHeadAndDeniedPullDoNotActivate(t *testing.T) {
	s, cred := distributionFixture(t)
	city := geoip.CityLocation{Key: "CN:1", Country: "CN", Province: "江苏", City: "南京"}
	for _, test := range []struct {
		method, ua string
		city       geoip.CityLocation
		status     int
	}{
		{http.MethodGet, "Mozilla/5.0", city, 302},
		{http.MethodHead, "Clash/1.0", city, 200},
		{http.MethodGet, "curl/8", city, 403},
		{http.MethodGet, "Clash/1.0", geoip.CityLocation{Country: "US"}, 200},
		{http.MethodGet, "Clash/1.0", geoip.CityLocation{Country: "CN"}, 200},
	} {
		response := pullDistribution(s, cred.Token, test.method, test.ua, "clash", test.city)
		if response.Code != test.status || strings.Contains(response.Body.String(), "example.com") {
			t.Fatalf("%s %s: status %d, resource leak or wrong response", test.method, test.ua, response.Code)
		}
		stored, err := s.ByID(context.Background(), cred.ID)
		if err != nil || stored.ActivatedAt != nil || len(stored.Regions) != 0 {
			t.Fatalf("non-delivery activated: %+v %v", stored, err)
		}
	}
}

func TestDistributionClashMetaAndroidAutodetectionAndRestrictions(t *testing.T) {
	s, cred := distributionFixture(t)
	ctx := context.Background()
	const ua = "ClashMetaForAndroid/2.11.25.Meta"
	city := geoip.CityLocation{Key: "CN:1", Country: "CN", Province: "江苏", City: "南京"}
	// No client override: the application's real UA must select Clash YAML.
	for _, tc := range []struct {
		method, ua string
		location   geoip.CityLocation
		status     int
		yamlNotice bool
	}{
		{"GET", "Mozilla/5.0 " + ua, city, http.StatusFound, false},
		{"GET", "ClashMetaForAndroidOther/1", city, http.StatusForbidden, false},
		{"GET", "curl/8", city, http.StatusForbidden, false},
		{"HEAD", ua, city, http.StatusOK, false},
		{"GET", ua, geoip.CityLocation{Country: "US"}, http.StatusOK, true},
		{"GET", ua, geoip.CityLocation{Country: "CN"}, http.StatusOK, true},
	} {
		response := pullDistribution(s, cred.Token, tc.method, tc.ua, "", tc.location)
		if response.Code != tc.status || strings.Contains(response.Body.String(), "example.com") {
			t.Fatalf("unexpected response for %q: %d", tc.ua, response.Code)
		}
		if tc.yamlNotice && len(distributionTestNodes(t, response, "clash")) == 0 {
			t.Fatal("denial did not produce Clash notice nodes")
		}
	}
	stored, err := s.ByID(ctx, cred.ID)
	if err != nil || stored.ActivatedAt != nil || len(stored.Regions) != 0 {
		t.Fatal("rejected requests activated the subscription or consumed a city", err)
	}
	response := pullDistribution(s, cred.Token, "GET", ua, "", city)
	if len(distributionTestNodes(t, response, "clash")) == 0 || !strings.Contains(response.Body.String(), "example.com") {
		t.Fatal("Clash Meta for Android did not receive usable Clash YAML")
	}
	stored, err = s.ByID(ctx, cred.ID)
	if err != nil || stored.ActivatedAt == nil || len(stored.Regions) != 1 {
		t.Fatal("valid client did not activate normally", err)
	}
	second := geoip.CityLocation{Key: "CN:2", Country: "CN", City: "苏州"}
	third := geoip.CityLocation{Key: "CN:3", Country: "CN", City: "杭州"}
	_ = pullDistribution(s, cred.Token, "GET", ua, "", second)
	response = pullDistribution(s, cred.Token, "GET", ua, "", third)
	if len(distributionTestNodes(t, response, "clash")) == 0 || strings.Contains(response.Body.String(), "example.com") {
		t.Fatal("third-city request bypassed restrictions")
	}
	// An administrator's narrower allowlist still controls this client.
	cfg, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AllowedUA = `(?i)^clash/`
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	response = pullDistribution(s, cred.Token, "GET", ua, "", city)
	if response.Code != http.StatusForbidden {
		t.Fatal("client bypassed custom allowlist")
	}
}

func TestDistributionSkipsLocationForNonDeliveries(t *testing.T) {
	s, cred := distributionFixture(t)
	r := gin.New()
	handler := func(c *gin.Context) {
		distributionClient(c, s, func(string) (geoip.CityLocation, error) {
			t.Error("non-delivery spent location lookup quota")
			return geoip.CityLocation{}, nil
		})
	}
	r.GET("/d/:token", handler)
	r.HEAD("/d/:token", handler)
	for _, tc := range []struct {
		method, token, ua string
		status            int
	}{
		{"GET", cred.Token, "Mozilla/5.0", http.StatusFound},
		{"GET", cred.Token, "curl/8", http.StatusForbidden},
		{"HEAD", cred.Token, "Clash/1.0", http.StatusOK},
		{"HEAD", strings.Repeat("a", 64), "Clash/1.0", http.StatusNotFound},
		{"GET", strings.Repeat("b", 64), "Clash/1.0", http.StatusOK},
	} {
		request := httptest.NewRequestWithContext(context.Background(), tc.method, "/d/"+tc.token, nil)
		request.Host = "distribution.test"
		request.Header.Set("User-Agent", tc.ua)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s: status %d", tc.method, response.Code)
		}
	}
	status := "disabled"
	if _, err := s.UpdateCredential(context.Background(), cred.ID, distribution.CredentialPatch{Status: &status}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), "GET", "/d/"+cred.Token, nil)
	request.Host = "distribution.test"
	request.Header.Set("User-Agent", "Clash/1.0")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "example.com") {
		t.Fatal("disabled subscription leaked resources")
	}
}

func TestDistributionLocationErrorNeverActivates(t *testing.T) {
	s, cred := distributionFixture(t)
	r := gin.New()
	r.GET("/d/:token", func(c *gin.Context) {
		distributionClient(c, s, func(string) (geoip.CityLocation, error) {
			return geoip.CityLocation{Country: "CN", Province: "湖北", City: "武汉", Key: "CN:1791247"}, errors.New("provider unavailable")
		})
	})
	request := httptest.NewRequestWithContext(context.Background(), "GET", "/d/"+cred.Token, nil)
	request.Host = "distribution.test"
	request.Header.Set("User-Agent", "Clash/1.0")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "example.com") {
		t.Fatal("failed location leaked resources")
	}
	stored, err := s.ByID(context.Background(), cred.ID)
	if err != nil || stored.ActivatedAt != nil || len(stored.Regions) != 0 {
		t.Fatal("failed location activated or registered city")
	}
}

func TestDistributionDeliveryThirdCityAndRenewal(t *testing.T) {
	s, cred := distributionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	s.Now = func() time.Time { return now }
	cities := []geoip.CityLocation{
		{Key: "CN:1", Country: "CN", Province: "江苏", City: "南京"},
		{Key: "CN:2", Country: "CN", Province: "江苏", City: "苏州"},
		{Key: "CN:3", Country: "CN", Province: "浙江", City: "杭州"},
	}
	for i, city := range cities {
		response := pullDistribution(s, cred.Token, http.MethodGet, "Clash/1.0", "clash", city)
		if response.Code != 200 {
			t.Fatalf("pull %d failed: %d %s", i, response.Code, response.Body.String())
		}
		hasResource := strings.Contains(response.Body.String(), "example.com")
		if hasResource != (i < 2) {
			t.Fatalf("wrong city authorization for %d", i)
		}
	}
	status, err := s.PublicStatus(ctx, cred.Token)
	if err != nil || len(status.Regions) != 2 || len(status.Candidates) != 1 || status.ActivatedAt == nil {
		t.Fatalf("bad status: %+v %v", status, err)
	}
	encoded, _ := json.Marshal(status)
	if strings.Contains(string(encoded), cred.Token) || strings.Contains(string(encoded), "Clash/1.0") {
		t.Fatal("public status leaks secrets or UA")
	}
	now = now.Add(16 * 24 * time.Hour)
	response := pullDistribution(s, cred.Token, http.MethodGet, "Clash/1.0", "clash", cities[0])
	if strings.Contains(response.Body.String(), "example.com") || !strings.Contains(response.Body.String(), "placeholder.invalid") {
		t.Fatal("expiry did not replace resources")
	}
	cards, err := s.CreateCards(ctx, distribution.CardInput{Name: "Paid", Kind: "quarter", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Redeem(ctx, cred.Token, cards[0].Code, "api"); err != nil {
		t.Fatal(err)
	}
	response = pullDistribution(s, cred.Token, http.MethodGet, "Clash/1.0", "clash", cities[0])
	if !strings.Contains(response.Body.String(), "example.com") {
		t.Fatal("renewal did not restore resources")
	}
	// A new token must not work through the legacy share route.
	r := gin.New()
	r.GET("/c/:token", GetClient)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/c/"+cred.Token+"?client=clash", nil)
	req.Header.Set("User-Agent", "Clash/1.0")
	legacy := httptest.NewRecorder()
	r.ServeHTTP(legacy, req)
	if strings.Contains(legacy.Body.String(), "example.com") {
		t.Fatal("legacy route bypassed policy")
	}
}

func TestDistributionOutputValidationAndAliases(t *testing.T) {
	link := "ss://YWVzLTEyOC1nY206cGFzc0BleGFtcGxlLmNvbTo0NDM=#Node"
	for _, test := range []struct {
		client, body string
		valid        bool
	}{
		{"clash", "proxies: []\n", false},
		{"clash", "proxies:\n- {name: x, type: ss, server: example.com, port: 443}\n", true},
		{"surge", "[Proxy]\nDIRECT = direct\n", false},
		{"surge", "[Proxy]\nx = ss, example.com, 443, encrypt-method=aes-128-gcm, password=p\n", true},
		{"singbox", `{"outbounds":[{"type":"direct"}]}`, false},
		{"singbox", `{"outbounds":[{"type":"shadowsocks","server":"example.com"}]}`, true},
		{"qx", "shadowsocks=example.com:443, password=p, method=aes-128-gcm", true},
		{"v2ray", "", false},
		{"v2ray", base64.StdEncoding.EncodeToString([]byte(link)), true},
		{"clashmeta", "proxies: []", false},
	} {
		if validDistributionOutput([]byte(test.body), normalizeDistributionClient(test.client)) != test.valid {
			t.Errorf("validation %s %q", test.client, test.body)
		}
	}
}

func TestDistributionManagedURLAndDirectRule(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://request.example/d/token?client=surge", nil)
	cfg := distribution.Settings{Domain: "sub.example", PortalURL: "https://shop.example/renew"}
	body, err := distributionBodyPolicy(c, []byte("#!MANAGED-CONFIG https://legacy.example/c/old\n[Rule]\nFINAL,PROXY\n"), "surge", cfg)
	if err != nil || strings.Contains(string(body), "legacy.example") || strings.Contains(string(body), "shop.example") || !strings.HasPrefix(string(body), "#!MANAGED-CONFIG https://sub.example/d/token?client=surge interval=86400") || !strings.Contains(string(body), "[Rule]\nDOMAIN,sub.example,DIRECT") {
		t.Fatalf("unsafe body %s: %v", body, err)
	}
}

func TestDistributionEmptyConversionCannotActivate(t *testing.T) {
	s, cred := distributionFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"par_res":"[Proxy]\nDIRECT = direct\n"}}`))
	}))
	defer server.Close()
	saveSubStoreSettings(t, server.URL, []string{"loon"})
	response := pullDistribution(s, cred.Token, http.MethodGet, "Loon/1.0", "loon", geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"})
	if response.Code != 503 {
		t.Fatalf("empty conversion status=%d", response.Code)
	}
	stored, err := s.ByID(context.Background(), cred.ID)
	if err != nil || stored.ActivatedAt != nil || len(stored.Regions) > 0 {
		t.Fatal("empty conversion activated")
	}
}

func TestDistributionProfileNameSurvivesDenialAndResourceChange(t *testing.T) {
	for _, client := range []string{"clash", "mihomo", "surge", "v2ray"} {
		t.Run(client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			name := "9月 + 测试-003"
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Name: &name}); err != nil {
				t.Fatal(err)
			}
			city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			checkName := func(response *httptest.ResponseRecorder) {
				t.Helper()
				_, params, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
				if err != nil || !strings.HasPrefix(params["filename"], name+".") {
					t.Fatalf("wrong filename: %s (%v)", params["filename"], err)
				}
				wantTitle := "base64:" + base64.StdEncoding.EncodeToString([]byte(name))
				if response.Code != 200 || response.Header().Get("profile-title") != wantTitle {
					t.Fatalf("wrong status or profile title: %d", response.Code)
				}
			}
			body := func(response *httptest.ResponseRecorder) string {
				t.Helper()
				if client == "v2ray" {
					decoded, err := base64.StdEncoding.DecodeString(response.Body.String())
					if err != nil {
						t.Fatal(err)
					}
					var nodes []string
					for _, link := range strings.Split(strings.TrimSpace(string(decoded)), "\n") {
						proxy, err := protocol.LinkToProxy(protocol.Urls{Url: link}, protocol.OutputConfig{})
						if err != nil {
							t.Fatal(err)
						}
						nodes = append(nodes, proxy.Server+" "+proxy.Name)
					}
					return strings.Join(nodes, "\n")
				}
				return response.Body.String()
			}
			denied := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, geoip.CityLocation{Country: "US"})
			checkName(denied)
			if !strings.Contains(body(denied), "placeholder.invalid") || strings.Contains(body(denied), "example.com") {
				t.Fatal("denied response leaked real resources or lost its notice")
			}
			allowed := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			checkName(allowed)
			if !strings.Contains(body(allowed), "example.com") || strings.Contains(body(allowed), "placeholder.invalid") {
				t.Fatal("mainland refresh did not replace notice with real resources")
			}
			before, err := s.ByID(ctx, cred.ID)
			if err != nil {
				t.Fatal(err)
			}
			name = "已激活后改名 + 004"
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Name: &name}); err != nil {
				t.Fatal(err)
			}
			checkName(pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city))
			createClientSubscriptionFixture(t, writeTestClashTemplate(t), writeTestSurgeTemplate(t), "replacement", "replacement-share", "ReplacementNode")
			var replacement models.Subcription
			if err := database.DB.Where("name = ?", "replacement").First(&replacement).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{SubscriptionID: &replacement.ID}); err != nil {
				t.Fatal(err)
			}
			updated := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			checkName(updated)
			if !strings.Contains(body(updated), "ReplacementNode") || strings.Contains(body(updated), "Private node") {
				t.Fatal("refresh did not use the new resource pool")
			}
			after, err := s.ByID(ctx, cred.ID)
			if err != nil || !after.ExpiresAt.Equal(*before.ExpiresAt) || !after.ActivatedAt.Equal(*before.ActivatedAt) {
				t.Fatal("resource refresh reset the trial")
			}
			s.Now = func() time.Time { return after.ExpiresAt.Add(time.Hour) }
			checkName(pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city))
		})
	}
}

func TestDistributionOmitsUsageWhenResourceSwitchIsOff(t *testing.T) {
	for _, client := range []string{"clash", "mihomo", "surge", "v2ray"} {
		t.Run(client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			check := func(response *httptest.ResponseRecorder) {
				t.Helper()
				if response.Code != http.StatusOK || response.Header().Get("subscription-userinfo") != "" {
					t.Fatalf("status=%d, unexpected recipient usage=%q", response.Code, response.Header().Get("subscription-userinfo"))
				}
			}
			check(pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city))
			check(pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, geoip.CityLocation{Country: "US"}))
			active, err := s.ByID(ctx, cred.ID)
			if err != nil || active.ExpiresAt == nil {
				t.Fatal("missing activation expiry", err)
			}
			s.Now = func() time.Time { return active.ExpiresAt.Add(time.Hour) }
			check(pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city))
			permanent := true
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Permanent: &permanent}); err != nil {
				t.Fatal(err)
			}
			check(pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city))
			check(pullDistribution(s, strings.Repeat("a", 64), "GET", "Clash/1.0", client, city))
		})
	}
}

func TestDistributionUsageFollowsResourceSwitch(t *testing.T) {
	for _, client := range []string{"clash", "mihomo", "surge", "v2ray"} {
		t.Run(client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("subscription-userinfo", fmt.Sprintf("upload=3072; download=4096; total=8192; expire=%d", time.Now().AddDate(1, 0, 0).Unix()))
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()
			if err := s.DB.AutoMigrate(&models.Airport{}); err != nil {
				t.Fatal(err)
			}
			airport := models.Airport{Name: "Switch fixture", URL: upstream.URL, FetchUsageInfo: true, UsageUpload: 1, UsageDownload: 2, UsageTotal: 3}
			if err := s.DB.Create(&airport).Error; err != nil {
				t.Fatal(err)
			}
			if err := models.InitAirportCache(); err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Model(&models.Node{}).Where("name = ?", "distribution-node").Updates(map[string]any{"source": airport.Name, "source_id": airport.ID}).Error; err != nil {
				t.Fatal(err)
			}
			if err := models.InitNodeCache(); err != nil {
				t.Fatal(err)
			}
			sub := models.Subcription{ID: cred.SubscriptionID}
			if err := sub.Find(); err != nil {
				t.Fatal(err)
			}
			setUsage := func(enabled bool) {
				t.Helper()
				sub.RefreshUsageOnRequest = enabled
				if err := sub.Update(); err != nil {
					t.Fatal(err)
				}
			}
			city := geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"}
			var wantRequests int32
			for _, enabled := range []bool{false, true, true, false} {
				setUsage(enabled)
				response := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
				if response.Code != http.StatusOK || !validDistributionOutput(response.Body.Bytes(), client) {
					t.Fatalf("resource output failed: %d", response.Code)
				}
				want := ""
				if enabled {
					wantRequests++
					active, err := s.ByID(ctx, cred.ID)
					if err != nil || active.ExpiresAt == nil {
						t.Fatal("missing recipient expiry", err)
					}
					want = "upload=3072; download=4096; total=8192; expire=" + strconv.FormatInt(active.ExpiresAt.Unix(), 10)
				}
				if got := response.Header().Get("subscription-userinfo"); got != want {
					t.Fatalf("enabled=%t, header=%q, want=%q", enabled, got, want)
				}
				if requests.Load() != wantRequests {
					t.Fatalf("enabled=%t, upstream requests=%d, want=%d", enabled, requests.Load(), wantRequests)
				}
			}
			setUsage(true)
			// Turning usage on must not make denied/probe/expired requests fetch
			// airport usage or reveal it, even with fallback nodes selected.
			for _, test := range []struct {
				method, ua string
				city       geoip.CityLocation
			}{
				{"HEAD", "Clash/1.0", city},
				{"GET", "Mozilla/5.0", city},
				{"GET", "Clash/1.0", geoip.CityLocation{Country: "US"}},
			} {
				response := pullDistribution(s, cred.Token, test.method, test.ua, client, test.city)
				if response.Header().Get("subscription-userinfo") != "" || requests.Load() != wantRequests {
					t.Fatal("probe or denial refreshed/exposed usage")
				}
			}
			cfg, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cfg.FallbackSubscriptionID = cred.SubscriptionID
			if err := s.SaveSettings(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			active, err := s.ByID(ctx, cred.ID)
			if err != nil {
				t.Fatal(err)
			}
			s.Now = func() time.Time { return active.ExpiresAt.Add(time.Hour) }
			expired := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			if expired.Code != http.StatusOK || expired.Header().Get("subscription-userinfo") != "" || requests.Load() != wantRequests {
				t.Fatal("expiry/fallback refreshed/exposed usage")
			}
			permanent := true
			if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Permanent: &permanent}); err != nil {
				t.Fatal(err)
			}
			response := pullDistribution(s, cred.Token, "GET", "Clash/1.0", client, city)
			if response.Code != http.StatusOK || response.Header().Get("subscription-userinfo") != "upload=3072; download=4096; total=8192" || requests.Load() != wantRequests+1 {
				t.Fatal("permanent subscription inherited airport expiry or lost usage")
			}
		})
	}
}

func TestDistributionStripsCachedAirportUsageWithoutChangingLegacyRenderer(t *testing.T) {
	s, cred := distributionFixture(t)
	if err := s.DB.AutoMigrate(&models.Airport{}); err != nil {
		t.Fatal(err)
	}
	airport := models.Airport{Name: "Shared usage fixture", FetchUsageInfo: true, UsageUpload: 1024, UsageDownload: 2048, UsageTotal: 4096}
	if err := s.DB.Create(&airport).Error; err != nil {
		t.Fatal(err)
	}
	if err := models.InitAirportCache(); err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	prepared, ok := distributionPrepared(c, "clash", cred.SubscriptionID)
	if !ok {
		t.Fatal("missing test resources")
	}
	prepared.Subscription.Nodes[0].Source = airport.Name
	prepared.Subscription.Nodes[0].SourceID = airport.ID
	for _, want := range []string{"upload=1024; download=2048; total=4096; expire=0", "upload=0; download=0; total=0; expire=0"} {
		legacy, _ := gin.CreateTestContext(httptest.NewRecorder())
		legacy.Request = c.Request
		dispatchPreparedClientResponse(legacy, prepared)
		if got := legacy.Writer.Header().Get("subscription-userinfo"); got != want {
			t.Fatalf("legacy header=%q, want=%q", got, want)
		}
		buffer := renderDistribution(c, prepared)
		if buffer.failed || buffer.status != http.StatusOK || buffer.headers.Get("subscription-userinfo") != "" {
			t.Fatal("recipient rendering retained upstream quota")
		}
		prepared.Subscription.Nodes[0].Source = "manual"
		prepared.Subscription.Nodes[0].SourceID = 0
	}
}
