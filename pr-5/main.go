package main

import (
	"log"
	"net/http"
	"os"

	"notion_pr5/database"
	_ "notion_pr5/docs"
	"notion_pr5/handlers"
	"notion_pr5/middleware"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// @title Incident Management API
// @version 1.0
// @description Incident tracking API with JWT authentication. In Authorize enter: Bearer <token>.
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	secretKey := os.Getenv("JWT_SECRET")
	if len(secretKey) < 32 {
		log.Fatal("JWT_SECRET must contain at least 32 bytes")
	}
	dsn := "host=127.0.0.1 user=pr5 password=pr5_local dbname=pr5 port=5432 sslmode=disable"
	db, err := database.StartDB(dsn)
	if err != nil {
		log.Fatal(err)
	}

	if err := newRouter(db, secretKey).Run(); err != nil {
		log.Fatal(err)
	}
}

func newRouter(db *gorm.DB, secretKey string) *gin.Engine {
	r := gin.Default()
	v1 := r.Group("/api/v1")
	routeForIncident(v1, db, secretKey)
	routeForAuth(v1, db)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	return r
}

func routeForIncident(v1 *gin.RouterGroup, db *gorm.DB, secretKey string) {
	v1.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	v1.GET("/incidents", func(ctx *gin.Context) {
		handlers.FindAll(ctx, db)
	})
	v1.GET("/incidents/:id", func(ctx *gin.Context) {
		handlers.FindById(ctx, db)
	})
	private := v1.Group("")
	private.Use(middleware.AuthMiddleware(secretKey))
	private.PATCH("/incidents/:id/status", func(ctx *gin.Context) {
		handlers.UpdateById(ctx, db)
	})
	private.POST("/incidents", func(ctx *gin.Context) {
		handlers.CreateIncidentHandler(ctx, db)
	})
	private.DELETE("/incidents/:id", func(ctx *gin.Context) {
		handlers.DeleteById(ctx, db)
	})
}

func routeForAuth(v1 *gin.RouterGroup, db *gorm.DB) {
	v1.POST("/auth/login", func(ctx *gin.Context) {
		handlers.Authorization(ctx, db)
	})
	v1.POST("/auth/register", func(ctx *gin.Context) {
		handlers.Register(ctx, db)
	})
}
