package middleware

import (
	"net/http"
	"strings"

	"notion_pr5/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(secretKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		unauthorized := func() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		}
		header := c.GetHeader("Authorization")
		if len(secretKey) < 32 || !strings.HasPrefix(header, "Bearer ") {
			unauthorized()
			return
		}

		tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if tokenString == "" {
			unauthorized()
			return
		}

		claims := &auth.Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			return []byte(secretKey), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
		if err != nil || token == nil || !token.Valid || claims.UserID == 0 {
			unauthorized()
			return
		}

		c.Set("userID", claims.UserID)
		c.Next()
	}
}
