package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"sublink/middlewares"
	"sublink/services/distribution"
)

func TestDistributionCredentialSearchBodyAndPrivacy(t *testing.T) {
	t.Setenv("SUBLINK_API_ENCRYPTION_KEY", "test-key")
	s, cred := distributionFixture(t)
	r := gin.New()
	r.POST("/credentials/search", DistributionPrivateHeaders, DistributionSearchCredentials)
	link := "https://long-subscription-domain.example/d/" + cred.Token + "?client=mihomo#client-profile"
	for _, keyword := range []string{link, " \n" + cred.Token + "\t", cred.Name} {
		body, err := json.Marshal(distribution.ListFilter{Keyword: keyword, Page: 1, Size: 20})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/credentials/search", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || req.URL.RawQuery != "" {
			t.Fatal("body search failed or lost private response headers")
		}
		var response struct {
			Data distribution.Page[distribution.Credential] `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Data.Total != 1 || len(response.Data.Items) != 1 || response.Data.Items[0].ID != cred.ID {
			t.Fatal("complete link was truncated or did not match", err)
		}
		if strings.Contains(w.Body.String(), "token_hash") || strings.Contains(w.Body.String(), `"secret"`) {
			t.Fatal("search exposed storage secrets")
		}
	}
	for _, body := range []string{`{"keyword":`, `{"keyword":"` + strings.Repeat("x", 2049) + `"}`, `{"keyword":"` + strings.Repeat("x", 17000) + `"}`, `{"keyword":"test","status":"invalid"}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/credentials/search", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatal("invalid search input accepted", w.Code)
		}
	}
	stored, err := s.ByID(context.Background(), cred.ID)
	if err != nil || stored.ActivatedAt != nil || stored.AccessCount != 0 || len(stored.Regions) != 0 {
		t.Fatal("administrative search activated recipient", err)
	}
	secured := gin.New()
	secured.POST("/credentials/search", DistributionPrivateHeaders, middlewares.AuthToken, DistributionSearchCredentials)
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/credentials/search", strings.NewReader(`{"keyword":"`+cred.Token+`"}`))
	req.Header.Set("Content-Type", "application/json")
	secured.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatal("unauthenticated search exposed recipients", w.Code)
	}
	if strings.Contains(w.Body.String(), cred.Token) || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("unauthenticated response leaked token or lost private headers")
	}
}

func TestDistributionCardSearchBodyAndPrivacy(t *testing.T) {
	t.Setenv("SUBLINK_API_ENCRYPTION_KEY", "test-key")
	s, _ := distributionFixture(t)
	ctx := context.Background()
	cards, err := s.CreateCards(ctx, distribution.CardInput{Name: "Search fixture", Count: 1, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/cards/search", DistributionPrivateHeaders, DistributionSearchCards)
	for _, keyword := range []string{cards[0].Code, strings.ToLower(cards[0].Code), strings.ReplaceAll(cards[0].Code, "-", ""), cards[0].Name} {
		body, _ := json.Marshal(distribution.ListFilter{Keyword: keyword, Page: 1, Size: 20})
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/cards/search", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || req.URL.RawQuery != "" {
			t.Fatal("card search failed or leaked code into URL", w.Code)
		}
		var response struct {
			Data distribution.Page[distribution.Card] `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Data.Total != 1 || len(response.Data.Items) != 1 || response.Data.Items[0].ID != cards[0].ID {
			t.Fatal("card body lookup failed", err)
		}
		if strings.Contains(w.Body.String(), "code_hash") || strings.Contains(w.Body.String(), `"secret"`) {
			t.Fatal("search exposed stored hash or ciphertext")
		}
	}
	for _, body := range []string{`{"keyword":`, `{"keyword":"` + strings.Repeat("a", 2049) + `"}`, `{"keyword":"` + strings.Repeat("a", 17000) + `"}`, `{"status":"invalid"}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/cards/search", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatal("invalid card search accepted", w.Code)
		}
	}
	secured := gin.New()
	secured.POST("/cards/search", DistributionPrivateHeaders, middlewares.AuthToken, DistributionSearchCards)
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/cards/search", strings.NewReader(`{"keyword":"`+cards[0].Code+`"}`))
	req.Header.Set("Content-Type", "application/json")
	secured.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatal("unauthenticated card search allowed", w.Code)
	}
	if strings.Contains(w.Body.String(), cards[0].Code) {
		t.Fatal("unauthenticated response leaked code")
	}
}

func TestDistributionRegionLimitSettingsResponses(t *testing.T) {
	t.Setenv("SUBLINK_API_ENCRYPTION_KEY", "test-key")
	s, cred := distributionFixture(t)
	cfg, _ := s.Settings(context.Background())
	cfg.RegionLimit = 1
	if err := s.SaveSettings(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/settings", DistributionPublicHeaders, DistributionPublicSettings)
	r.POST("/status", DistributionPublicHeaders, DistributionStatus)
	for _, path := range []string{"/settings", "/status"} {
		method := http.MethodGet
		if path == "/status" {
			method = http.MethodPost
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(`{"link":"`+cred.Token+`"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		var response struct {
			Data struct {
				RegionLimit int `json:"region_limit"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.Data.RegionLimit != 1 {
			t.Fatal("public API reports wrong city limit", err)
		}
		if strings.Contains(w.Body.String(), cred.Token) || strings.Contains(w.Body.String(), "allowed_ua") {
			t.Fatal("public settings leaked private fields")
		}
	}
}
