package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"sublink/middlewares"
	"sublink/services/distribution"
)

func TestDistributionRedeemReportsIdempotentOutcome(t *testing.T) {
	t.Setenv("SUBLINK_API_ENCRYPTION_KEY", "test-key")
	s, credential := distributionFixture(t)
	ctx := context.Background()
	if _, err := s.CommitDelivery(ctx, credential.Token, distribution.City{Key: "CN:1", Country: "CN", Name: "Fixture"}, credential.SubscriptionID); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if _, err := s.UpdateCredential(ctx, credential.ID, distribution.CredentialPatch{ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	card, err := s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"link": credential.Token, "code": card.Code})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/api/public/distribution/redeem", DistributionRedeem)
	var first distribution.Redemption
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(ctx, "POST", "/api/public/distribution/redeem", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("redeem status %d", w.Code)
		}
		var out struct {
			Data distribution.Redemption `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Data.AlreadyRedeemed != (i > 0) {
			t.Fatal("incorrect already_redeemed outcome")
		}
		if i == 0 {
			first = out.Data
		} else if out.Data.ID != first.ID || !out.Data.After.Equal(*first.After) {
			t.Fatal("retry altered original outcome")
		}
	}
}

func TestDistributionCardManagementHandlers(t *testing.T) {
	t.Setenv("SUBLINK_API_ENCRYPTION_KEY", "test-key")
	s, _ := distributionFixture(t)
	rows, err := s.CreateCards(context.Background(), distribution.CardInput{Name: "HTTP", Count: 1, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/cards/:id", DistributionCard)
	r.PATCH("/cards/:id", DistributionUpdateCard)
	r.DELETE("/cards/:id", DistributionDeleteCard)
	r.POST("/cards/batch-delete", DistributionDeleteCards)
	r.POST("/cards/free-cycle", DistributionFreeCard)
	r.POST("/cards/free-cycle/reissue", DistributionReissueFreeCard)
	call := func(method, path, body string, status int, code string) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("HTTP status %d, expected %d", w.Code, status)
		}
		if code != "" {
			var response struct {
				Key string `json:"i18nKey"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Key != "distribution.errors."+code {
				t.Fatal("incorrect localized error", response.Key)
			}
		}
	}
	path := "/cards/" + strconv.Itoa(int(rows[0].ID))
	call("GET", path, "", 200, "")
	call("PATCH", path, `{"name":"corrected","kind":"year","ends_at":null}`, 200, "")
	call("PATCH", path, `{"enabled":false}`, 200, "")
	call("PATCH", path, `{}`, 400, "invalid_input")
	call("PATCH", path, `{"ends_at":42}`, 400, "invalid_input")
	call("PATCH", "/cards/0", `{"enabled":true}`, 400, "invalid_input")
	call("POST", "/cards/batch-delete", `{"ids":[]}`, 400, "invalid_input")
	call("DELETE", path, "", 200, "")
	call("GET", path, "", 200, "")
	call("DELETE", path, "", 200, "")
	call("PATCH", path, `{"enabled":true}`, 400, "not_found")
	free, err := s.CurrentFreeCard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	call("DELETE", "/cards/"+strconv.Itoa(int(free.ID)), "", 200, "")
	call("POST", "/cards/free-cycle", "", 400, "card_deleted")
	call("POST", "/cards/free-cycle/reissue", "", 200, "")
	call("POST", "/cards/free-cycle", "", 200, "")
}

func TestDistributionCardManagementRequiresAuthentication(t *testing.T) {
	r := gin.New()
	group := r.Group("/api/v1/distribution", DistributionPrivateHeaders, middlewares.AuthToken)
	group.GET("/cards/:id", DistributionCard)
	group.PATCH("/cards/:id", DistributionUpdateCard)
	group.DELETE("/cards/:id", DistributionDeleteCard)
	group.POST("/cards/batch-delete", DistributionDeleteCards)
	group.POST("/cards/free-cycle/reissue", DistributionReissueFreeCard)
	for _, entry := range []struct{ method, path string }{{"GET", "/cards/1"}, {"PATCH", "/cards/1"}, {"DELETE", "/cards/1"}, {"POST", "/cards/batch-delete"}, {"POST", "/cards/free-cycle/reissue"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), entry.method, "/api/v1/distribution"+entry.path, strings.NewReader(`{}`)))
		if w.Code != 401 && w.Code != 403 {
			t.Fatal("management accepted unauthenticated request", w.Code)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("missing private headers")
		}
	}
}
