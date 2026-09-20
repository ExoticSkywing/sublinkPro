package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"sublink/middlewares"
	"sublink/services/distribution"
	"sublink/services/geoip"
)

func TestDistributionDisabledAndDeletedNoticeNodes(t *testing.T) {
	for _, client := range []string{"clash", "mihomo", "surge", "v2ray"} {
		t.Run(client, func(t *testing.T) {
			s, cred := distributionFixture(t)
			ctx := context.Background()
			cfg, _ := s.Settings(ctx)
			cfg.FallbackSubscriptionID = cred.SubscriptionID
			if err := s.SaveSettings(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			for _, status := range []string{"disabled", "enabled", "revoked"} {
				if _, err := s.UpdateCredential(ctx, cred.ID, distribution.CredentialPatch{Status: &status}); err != nil {
					t.Fatal(err)
				}
				response := pullDistribution(s, cred.Token, "GET", "ClashMetaForAndroid/2.11.25.Meta", client, geoip.CityLocation{Key: "CN:1", Country: "CN", City: "南京"})
				nodes := distributionTestNodes(t, response, client)
				if status == "enabled" {
					if len(nodes) != 1 || nodes[0].Name != "Private node" {
						t.Fatal("re-enable did not restore resources")
					}
					continue
				}
				want := []string{"订阅已停用", "请联系管理员恢复", "https://distribution.test/"}
				if status == "revoked" {
					want = []string{"订阅链接已失效", "请联系管理员", "https://distribution.test/"}
				}
				var names []string
				for _, n := range nodes {
					names = append(names, n.Name)
					if n.Server != "placeholder.invalid" {
						t.Fatal("disabled/deleted link leaked fallback resources")
					}
				}
				if !reflect.DeepEqual(names, want) {
					t.Fatalf("notice labels: %q", names)
				}
				if response.Header().Get("subscription-userinfo") != "" {
					t.Fatal("notice leaked usage")
				}
				row, _ := s.ByID(ctx, cred.ID)
				if status == "disabled" && row.ActivatedAt != nil {
					t.Fatal("disabled link activated")
				}
			}
		})
	}
}

func TestDistributionDeletionHandlersAndAuthentication(t *testing.T) {
	t.Setenv("SUBLINK_API_ENCRYPTION_KEY", "test-key")
	s, cred := distributionFixture(t)
	r := gin.New()
	r.DELETE("/credentials/:id", DistributionDeleteCredential)
	r.POST("/credentials/batch-delete", DistributionDeleteCredentials)
	path := "/credentials/" + strconv.Itoa(int(cred.ID))
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"DELETE", "/credentials/0", "", 400},
		{"POST", "/credentials/batch-delete", `{"ids":[]}`, 400},
		{"POST", "/credentials/batch-delete", `{"ids":[-1]}`, 400},
		{"POST", "/credentials/batch-delete", `{"ids":[` + strconv.Itoa(int(cred.ID)) + `,999999]}`, 400},
		{"DELETE", path, "", 200},
		{"DELETE", path, "", 200},
		{"POST", "/credentials/batch-delete", `{"ids":[` + strconv.Itoa(int(cred.ID)) + `]}`, 200},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s: status %d", tc.path, w.Code)
		}
	}
	detail, err := s.ByID(context.Background(), cred.ID)
	if err != nil || detail.Status != "revoked" {
		t.Fatal("delete handler did not invalidate", err)
	}
	encoded, err := json.Marshal(detail)
	if err != nil || strings.Contains(string(encoded), cred.Token) {
		t.Fatal("deleted metadata leaked token", err)
	}
	secured := gin.New()
	group := secured.Group("/credentials", DistributionPrivateHeaders, middlewares.AuthToken)
	group.DELETE("/:id", DistributionDeleteCredential)
	group.POST("/batch-delete", DistributionDeleteCredentials)
	for _, tc := range []struct{ method, path string }{{"DELETE", path}, {"POST", "/credentials/batch-delete"}} {
		w := httptest.NewRecorder()
		secured.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(`{}`)))
		if w.Code != 401 && w.Code != 403 {
			t.Fatal("unauthenticated deletion allowed", w.Code)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("missing private headers")
		}
	}
}
