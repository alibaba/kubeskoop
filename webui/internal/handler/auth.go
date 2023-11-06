package handler

import (
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/config"
	"time"
)

type loginParams struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func RegisterAuthHandler(g *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	g.POST("/login", auth.LoginHandler)
	g.GET("/info", auth.MiddlewareFunc(), userInfoHandler)
}

func GetAuthMiddleware() (*jwt.GinJWTMiddleware, error) {
	return jwt.New(&jwt.GinJWTMiddleware{
		Realm:       "kubeskoop webconsole",
		Key:         []byte(config.Global.Auth.JWTKey),
		Timeout:     24 * time.Hour,
		MaxRefresh:  24 * time.Hour,
		IdentityKey: "user",
		PayloadFunc: func(data interface{}) jwt.MapClaims {
			return data.(jwt.MapClaims)
		},
		Authenticator: func(ctx *gin.Context) (interface{}, error) {
			var params loginParams
			if err := ctx.ShouldBindJSON(&params); err != nil {
				return nil, err
			}
			if params.Username != config.Global.Auth.Username ||
				params.Password != config.Global.Auth.Password {
				return nil, jwt.ErrFailedAuthentication
			}
			return jwt.MapClaims{
				"user": params.Username,
				"role": "admin",
			}, nil
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.JSON(code, gin.H{
				"error": message,
			})
		},
		SendCookie:  true,
		TokenLookup: "cookie: jwt",
	})
}

func userInfoHandler(ctx *gin.Context) {
	claims := jwt.ExtractClaims(ctx)
	if len(claims) != 0 {
		ctx.JSON(200, claims)
		//ctx.JSON(200, gin.H{
		//	"user": claims["user"],
		//	"role": claims["role"],
		//})
		return
	}
	ctx.JSON(401, gin.H{
		"error": "unauthorized",
	})
}
