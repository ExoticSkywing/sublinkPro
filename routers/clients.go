package routers

import (
	"sublink/api"
	"sublink/middlewares"

	"github.com/gin-gonic/gin"
)

func Clients(r *gin.Engine) {
	ClientsGroup := r.Group("/c")
	ClientsGroup.Use(middlewares.GetIp)
	{
		ClientsGroup.GET("/", api.GetClient)
		ClientsGroup.HEAD("/", api.GetClient)
	}

	// 兼容旧服务订阅路径: /paraspace/*action (覆盖 /paraspace/elextest/:token, /paraspace/:token 等)
	r.GET("/paraspace/*action", middlewares.GetIp, api.GetLegacyParaspaceClient)
	r.HEAD("/paraspace/*action", middlewares.GetIp, api.GetLegacyParaspaceClient)
}
