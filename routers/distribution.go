package routers

import (
	"github.com/gin-gonic/gin"
	"sublink/api"
	"sublink/middlewares"
)

func Distribution(r *gin.Engine) {
	admin := r.Group("/api/v1/distribution", api.DistributionPrivateHeaders, middlewares.AuthToken)
	admin.GET("/settings", api.DistributionSettings)
	admin.PUT("/settings", api.DistributionSaveSettings)
	admin.GET("/credentials", api.DistributionCredentials)
	admin.POST("/credentials/search", api.DistributionSearchCredentials)
	admin.GET("/credentials/:id", api.DistributionCredential)
	admin.GET("/credentials/:id/access-grants", api.DistributionAccessGrants)
	admin.POST("/credentials/:id/access-grants", api.DistributionCreateAccessGrant)
	admin.POST("/credentials/:id/access-grants/:grantId/revoke", api.DistributionRevokeAccessGrant)
	admin.POST("/credentials", api.DistributionIssue)
	admin.PATCH("/credentials/:id", api.DistributionUpdateCredential)
	admin.DELETE("/credentials/:id", api.DistributionDeleteCredential)
	admin.POST("/credentials/batch-delete", api.DistributionDeleteCredentials)
	admin.GET("/cards", api.DistributionCards)
	admin.POST("/cards/search", api.DistributionSearchCards)
	admin.GET("/cards/:id", api.DistributionCard)
	admin.POST("/cards", api.DistributionCreateCards)
	admin.POST("/cards/free-cycle", api.DistributionFreeCard)
	admin.POST("/cards/free-cycle/reissue", api.DistributionReissueFreeCard)
	admin.POST("/cards/batch-delete", api.DistributionDeleteCards)
	admin.PATCH("/cards/:id", api.DistributionUpdateCard)
	admin.DELETE("/cards/:id", api.DistributionDeleteCard)
	admin.POST("/status", api.DistributionStatus)
	admin.POST("/redeem", api.DistributionRedeem)
	admin.GET("/redemptions", api.DistributionRedemptions)
	admin.GET("/visits", api.DistributionVisits)
	admin.GET("/region-requests", api.DistributionRequests)
	admin.POST("/region-requests", api.DistributionApply)
	admin.POST("/region-requests/:id/review", api.DistributionReview)
	public := r.Group("/api/public/distribution", api.DistributionPublicHeaders)
	public.GET("/settings", api.DistributionPublicSettings)
	public.POST("/status", api.DistributionStatus)
	public.POST("/redeem", api.DistributionRedeem)
	public.POST("/region-requests", api.DistributionApply)
	r.GET("/d/:token", api.DistributionPublicHeaders, api.DistributionClient)
	r.HEAD("/d/:token", api.DistributionPublicHeaders, api.DistributionClient)
}
