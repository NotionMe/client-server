package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"notion_pr5/auth"
	"notion_pr5/entity"
	"notion_pr5/entity/dto"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Register godoc
// @Summary Register a user
// @Tags auth
// @Accept json
// @Produce json
// @Param request body dto.RegisterDTO true "Registration credentials"
// @Success 201 {object} entity.User
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/register [post]
func Register(ctx *gin.Context, db *gorm.DB) {
	var user dto.RegisterDTO

	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error:": err.Error()})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)

	result := entity.User{
		Email:        user.Email,
		PasswordHash: string(hash),
	}

	db.Create(&result)
	ctx.JSON(http.StatusCreated, &result)
}

// Authorization godoc
// @Summary Log in and obtain a JWT valid for 24 hours
// @Tags auth
// @Accept json
// @Produce json
// @Param request body dto.LoginDTO true "Login credentials"
// @Success 200 {object} map[string]string "JWT in the token field"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/login [post]
func Authorization(ctx *gin.Context, db *gorm.DB) {
	var loginUser dto.LoginDTO
	var user entity.User

	if err := ctx.ShouldBindJSON(&loginUser); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error:": err.Error()})
		return
	}

	if err := db.Where("email = ?", loginUser.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Неправильний email або пароль"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Не вдалося отримати користувача"})
		}
		return
	}

	err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(loginUser.Password),
	)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Неправильний email або пароль"})
		return
	}

	tokenString, err := createAccessToken(user.ID, []byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Не вдалося створити токен"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"token": tokenString})
}

func createAccessToken(userID uint, secret []byte) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("JWT_SECRET must contain at least 32 bytes")
	}

	now := time.Now()
	claims := auth.Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
