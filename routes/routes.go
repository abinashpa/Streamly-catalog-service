package routes

import (
	"streamly/catalog-service/cache"
	"streamly/catalog-service/config"
	"streamly/catalog-service/controllers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, rdb *redis.Client, cfg config.Config) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "catalog-service"})
	})

	cacheMgr := cache.NewCacheManager(rdb)
	feedCtrl := controllers.NewFeedController(db, cacheMgr, cfg)
	interactCtrl := controllers.NewInteractionController(db, cacheMgr)

	api := r.Group("/api/v1/catalog")
	{
		// Feed and catalog endpoints
		api.GET("/feed", feedCtrl.GetFeed)
		api.GET("/trending", feedCtrl.GetTrending)
		api.GET("/media/:id", feedCtrl.GetItem)
		api.POST("/sync", feedCtrl.SyncMedia)

		// Interactions
		api.POST("/media/:id/view", interactCtrl.RecordView)
		api.POST("/media/:id/like", interactCtrl.ToggleLike)
		api.GET("/media/:id/comments", interactCtrl.GetComments)
		api.POST("/media/:id/comments", interactCtrl.AddComment)
	}

	return r
}
