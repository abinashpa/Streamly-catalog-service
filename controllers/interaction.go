package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"streamly/catalog-service/cache"
	"streamly/catalog-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InteractionController struct {
	DB    *gorm.DB
	Cache *cache.CacheManager
}

func NewInteractionController(db *gorm.DB, cache *cache.CacheManager) *InteractionController {
	return &InteractionController{DB: db, Cache: cache}
}

func (ic *InteractionController) RecordView(c *gin.Context) {
	id := c.Param("id")

	var item models.CatalogItem
	if err := ic.DB.First(&item, "id = ? OR media_id = ?", id, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media item not found"})
		return
	}

	// Increment in Redis for high performance
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = ic.Cache.IncrementView(ctx, item.ID)

	// Increment in Postgres
	ic.DB.Model(&item).UpdateColumn("views", gorm.Expr("views + 1"))

	c.JSON(http.StatusOK, gin.H{
		"message": "View recorded",
		"id":      item.ID,
		"views":   item.Views + 1,
	})
}

type LikeRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

func (ic *InteractionController) ToggleLike(c *gin.Context) {
	id := c.Param("id")
	var req LikeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var item models.CatalogItem
	if err := ic.DB.First(&item, "id = ? OR media_id = ?", id, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media item not found"})
		return
	}

	var existingLike models.MediaLike
	err := ic.DB.Where("catalog_id = ? AND user_id = ?", item.ID, req.UserID).First(&existingLike).Error

	liked := false
	if err == nil {
		// Already liked -> Unlike
		ic.DB.Delete(&existingLike)
		ic.DB.Model(&item).UpdateColumn("likes", gorm.Expr("GREATEST(likes - 1, 0)"))
		liked = false
	} else {
		// Not liked yet -> Add like
		newLike := models.MediaLike{
			CatalogID: item.ID,
			UserID:    req.UserID,
		}
		ic.DB.Create(&newLike)
		ic.DB.Model(&item).UpdateColumn("likes", gorm.Expr("likes + 1"))
		liked = true
	}

	ic.Cache.Invalidate(context.Background(), "catalog:*")

	c.JSON(http.StatusOK, gin.H{
		"liked": liked,
	})
}

func (ic *InteractionController) GetComments(c *gin.Context) {
	id := c.Param("id")

	var item models.CatalogItem
	if err := ic.DB.First(&item, "id = ? OR media_id = ?", id, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media item not found"})
		return
	}

	var comments []models.Comment
	if err := ic.DB.Where("catalog_id = ?", item.ID).Order("created_at DESC").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load comments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"comments": comments,
		"count":    len(comments),
	})
}

type AddCommentRequest struct {
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	UserAvatar string `json:"user_avatar"`
	Content    string `json:"content" binding:"required"`
}

func (ic *InteractionController) AddComment(c *gin.Context) {
	id := c.Param("id")

	var req AddCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comment content cannot be empty"})
		return
	}

	var item models.CatalogItem
	if err := ic.DB.First(&item, "id = ? OR media_id = ?", id, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media item not found"})
		return
	}

	userName := req.UserName
	if userName == "" {
		userName = "Streamly Viewer"
	}
	userAvatar := req.UserAvatar
	if userAvatar == "" {
		userAvatar = "https://api.dicebear.com/7.x/identicon/svg?seed=" + userName
	}
	userID := req.UserID
	if userID == "" {
		userID = uuid.New().String()
	}

	comment := models.Comment{
		CatalogID:  item.ID,
		UserID:     userID,
		UserName:   userName,
		UserAvatar: userAvatar,
		Content:    content,
		CreatedAt:  time.Now(),
	}

	if err := ic.DB.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Comment posted",
		"comment": comment,
	})
}
