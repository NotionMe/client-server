package auth

import "github.com/golang-jwt/jwt/v5"

// Claims is shared by token creation and verification.
type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}
