package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sublink/config"
	"sublink/database"
	"sublink/services/distribution"
	"sublink/utils"

	"github.com/gin-gonic/gin"
)

func distributionStore() *distribution.Store {
	return distribution.New(database.DB, config.GetAPIEncryptionKey())
}
func distributionResponse(c *gin.Context, data any, err error) {
	if err != nil {
		var p distribution.Problem
		status := http.StatusBadRequest
		code := "server_error"
		if errors.As(err, &p) {
			code = string(p)
		} else {
			status = http.StatusInternalServerError
			utils.Error("分发业务操作失败: %v", err)
		}
		c.JSON(status, gin.H{"code": status, "msg": code, "i18nKey": "distribution.errors." + code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": data})
}
func distributionBody(c *gin.Context, in any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := c.ShouldBindJSON(in); err != nil {
		distributionResponse(c, nil, distribution.Invalid)
		return false
	}
	return true
}
func distributionID(c *gin.Context) uint {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		distributionResponse(c, nil, distribution.Invalid)
		return 0
	}
	return uint(id)
}
func distributionFilter(c *gin.Context) distribution.ListFilter {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))
	id, _ := strconv.ParseUint(c.Query("credential_id"), 10, 32)
	keyword := c.Query("keyword")
	if len(keyword) > 100 {
		keyword = keyword[:100]
	}
	return distribution.ListFilter{Page: page, Size: size, Keyword: keyword, Status: c.Query("status"), CredentialID: uint(id)}
}

type distributionRateEntry struct {
	Until time.Time
	Count int
}

var distributionRate = struct {
	sync.Mutex
	Entries map[string]distributionRateEntry
	CleanAt time.Time
}{Entries: make(map[string]distributionRateEntry)}

// DistributionPublicHeaders also bounds anonymous request cost. Keys expire and
// the map is capped; URLs, card codes and subscription tokens are never logged.
func DistributionPrivateHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Next()
}

func DistributionPublicHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	limit := 30
	prefix := "portal:"
	if strings.HasPrefix(c.Request.URL.Path, "/d/") {
		limit = 120
		prefix = "pull:"
	}
	key := prefix + c.ClientIP()
	now := time.Now()
	distributionRate.Lock()
	if now.After(distributionRate.CleanAt) {
		for k, v := range distributionRate.Entries {
			if now.After(v.Until) {
				delete(distributionRate.Entries, k)
			}
		}
		distributionRate.CleanAt = now.Add(time.Minute)
	}
	entry, exists := distributionRate.Entries[key]
	blocked := !exists && len(distributionRate.Entries) >= 10000
	if now.After(entry.Until) {
		entry = distributionRateEntry{Until: now.Add(time.Minute)}
	}
	entry.Count++
	if !blocked {
		distributionRate.Entries[key] = entry
	}
	distributionRate.Unlock()
	if blocked || entry.Count > limit {
		c.Header("Retry-After", "60")
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "i18nKey": "distribution.errors.too_frequent", "msg": "too_frequent"})
		return
	}
	c.Next()
}
func DistributionSettings(c *gin.Context) {
	out, err := distributionStore().Settings(c.Request.Context())
	distributionResponse(c, out, err)
}
func DistributionSaveSettings(c *gin.Context) {
	var in distribution.Settings
	if !distributionBody(c, &in) {
		return
	}
	distributionResponse(c, nil, distributionStore().SaveSettings(c.Request.Context(), in))
}
func DistributionPublicSettings(c *gin.Context) {
	cfg, err := distributionStore().Settings(c.Request.Context())
	distributionResponse(c, gin.H{"portal_url": cfg.PortalURL, "trial_days": cfg.TrialDays, "free_days": cfg.FreeDays, "region_limit": cfg.RegionLimit}, err)
}
func DistributionCredentials(c *gin.Context) {
	out, err := distributionStore().Credentials(c.Request.Context(), distributionFilter(c))
	distributionResponse(c, out, err)
}

// Keep recipient links out of request URLs/access logs during admin searches.
func DistributionSearchCredentials(c *gin.Context) {
	var in distribution.ListFilter
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().Credentials(c.Request.Context(), in)
	distributionResponse(c, out, err)
}

func DistributionCredential(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	out, err := distributionStore().ByID(c.Request.Context(), id)
	distributionResponse(c, out, err)
}

func DistributionIssue(c *gin.Context) {
	var in distribution.IssueInput
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().Issue(c.Request.Context(), in)
	distributionResponse(c, out, err)
}
func DistributionUpdateCredential(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	var in distribution.CredentialPatch
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().UpdateCredential(c.Request.Context(), id, in)
	distributionResponse(c, out, err)
}
func DistributionCards(c *gin.Context) {
	out, err := distributionStore().Cards(c.Request.Context(), distributionFilter(c))
	distributionResponse(c, out, err)
}

// Card codes belong in the authenticated request body, not access-log URLs.
func DistributionSearchCards(c *gin.Context) {
	var in distribution.ListFilter
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().Cards(c.Request.Context(), in)
	distributionResponse(c, out, err)
}
func DistributionDeleteCredential(c *gin.Context) {
	if id := distributionID(c); id != 0 {
		distributionResponse(c, nil, distributionStore().DeleteCredentials(c.Request.Context(), []uint{id}))
	}
}
func DistributionDeleteCredentials(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
	}
	if distributionBody(c, &in) {
		distributionResponse(c, nil, distributionStore().DeleteCredentials(c.Request.Context(), in.IDs))
	}
}
func DistributionCard(c *gin.Context) {
	if id := distributionID(c); id != 0 {
		out, err := distributionStore().CardByID(c.Request.Context(), id)
		distributionResponse(c, out, err)
	}
}
func DistributionCreateCards(c *gin.Context) {
	var in distribution.CardInput
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().CreateCards(c.Request.Context(), in)
	distributionResponse(c, out, err)
}
func DistributionFreeCard(c *gin.Context) {
	out, err := distributionStore().CurrentFreeCard(c.Request.Context())
	distributionResponse(c, out, err)
}
func DistributionReissueFreeCard(c *gin.Context) {
	out, err := distributionStore().ReissueFreeCard(c.Request.Context())
	distributionResponse(c, out, err)
}
func DistributionUpdateCard(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	var in distribution.CardPatch
	if !distributionBody(c, &in) {
		return
	}
	distributionResponse(c, nil, distributionStore().UpdateCard(c.Request.Context(), id, in))
}
func DistributionDeleteCard(c *gin.Context) {
	if id := distributionID(c); id != 0 {
		distributionResponse(c, nil, distributionStore().DeleteCards(c.Request.Context(), []uint{id}))
	}
}
func DistributionDeleteCards(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
	}
	if distributionBody(c, &in) {
		distributionResponse(c, nil, distributionStore().DeleteCards(c.Request.Context(), in.IDs))
	}
}
func DistributionRedeem(c *gin.Context) {
	var in struct {
		Link string `json:"link"`
		Code string `json:"code"`
	}
	if !distributionBody(c, &in) {
		return
	}
	source := "web"
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
		source = "api"
	}
	out, err := distributionStore().Redeem(c.Request.Context(), in.Link, in.Code, source)
	distributionResponse(c, out, err)
}
func DistributionStatus(c *gin.Context) {
	var in struct {
		Link string `json:"link"`
	}
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().PublicStatus(c.Request.Context(), in.Link)
	distributionResponse(c, out, err)
}
func DistributionApply(c *gin.Context) {
	var in distribution.ApplyInput
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().ApplyRegion(c.Request.Context(), in)
	distributionResponse(c, out, err)
}
func DistributionRequests(c *gin.Context) {
	out, err := distributionStore().Requests(c.Request.Context(), distributionFilter(c))
	distributionResponse(c, out, err)
}
func DistributionReview(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	var in distribution.ReviewInput
	if !distributionBody(c, &in) {
		return
	}
	distributionResponse(c, nil, distributionStore().ReviewRegion(c.Request.Context(), id, in, c.GetString("username")))
}
func DistributionRedemptions(c *gin.Context) {
	out, err := distributionStore().Redemptions(c.Request.Context(), distributionFilter(c))
	distributionResponse(c, out, err)
}
func DistributionVisits(c *gin.Context) {
	out, err := distributionStore().Visits(c.Request.Context(), distributionFilter(c))
	distributionResponse(c, out, err)
}

func distributionGrantID(c *gin.Context) uint {
	id, err := strconv.ParseUint(c.Param("grantId"), 10, 32)
	if err != nil || id == 0 {
		distributionResponse(c, nil, distribution.Invalid)
		return 0
	}
	return uint(id)
}

func DistributionAccessGrants(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	out, err := distributionStore().ListAccessGrants(c.Request.Context(), id)
	distributionResponse(c, gin.H{"items": out, "total": len(out)}, err)
}

func DistributionCreateAccessGrant(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	var in distribution.AccessGrantInput
	if !distributionBody(c, &in) {
		return
	}
	out, err := distributionStore().CreateAccessGrant(c.Request.Context(), id, in)
	distributionResponse(c, out, err)
}

func DistributionRevokeAccessGrant(c *gin.Context) {
	id := distributionID(c)
	if id == 0 {
		return
	}
	grantID := distributionGrantID(c)
	if grantID == 0 {
		return
	}
	distributionResponse(c, nil, distributionStore().RevokeAccessGrant(c.Request.Context(), id, grantID))
}
