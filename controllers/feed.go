package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"streamly/catalog-service/cache"
	"streamly/catalog-service/config"
	"streamly/catalog-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FeedController struct {
	DB    *gorm.DB
	Cache *cache.CacheManager
	Cfg   config.Config
}

func NewFeedController(db *gorm.DB, cache *cache.CacheManager, cfg config.Config) *FeedController {
	return &FeedController{DB: db, Cache: cache, Cfg: cfg}
}

func (fc *FeedController) GetFeed(c *gin.Context) {
	mediaType := c.Query("type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 50 {
		limit = 50
	}
	offset := (page - 1) * limit

	cacheKey := fmt.Sprintf("catalog:feed:%s:p%d:l%d", mediaType, page, limit)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var cachedItems []models.CatalogItem
	if fc.Cache.Get(ctx, cacheKey, &cachedItems) {
		c.JSON(http.StatusOK, gin.H{
			"source": "cache",
			"items":  cachedItems,
			"page":   page,
			"limit":  limit,
		})
		return
	}

	query := fc.DB.Order("created_at DESC").Limit(limit).Offset(offset)
	if mediaType != "" && mediaType != "all" {
		query = query.Where("media_type = ?", mediaType)
	}

	var items []models.CatalogItem
	if err := query.Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query catalog"})
		return
	}

	// Cache result for 45 seconds
	fc.Cache.Set(ctx, cacheKey, items, 45*time.Second)

	c.JSON(http.StatusOK, gin.H{
		"source": "db",
		"items":  items,
		"page":   page,
		"limit":  limit,
	})
}

func (fc *FeedController) GetTrending(c *gin.Context) {
	mediaType := c.Query("type")
	cacheKey := fmt.Sprintf("catalog:trending:%s", mediaType)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var cachedItems []models.CatalogItem
	if fc.Cache.Get(ctx, cacheKey, &cachedItems) {
		c.JSON(http.StatusOK, gin.H{
			"source": "cache",
			"items":  cachedItems,
		})
		return
	}

	query := fc.DB.Order("views DESC, likes DESC").Limit(20)
	if mediaType != "" && mediaType != "all" {
		query = query.Where("media_type = ?", mediaType)
	}

	var items []models.CatalogItem
	if err := query.Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query trending items"})
		return
	}

	fc.Cache.Set(ctx, cacheKey, items, 60*time.Second)

	c.JSON(http.StatusOK, gin.H{
		"source": "db",
		"items":  items,
	})
}

func (fc *FeedController) GetItem(c *gin.Context) {
	id := c.Param("id")

	var item models.CatalogItem
	if err := fc.DB.First(&item, "id = ? OR media_id = ?", id, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media item not found in catalog"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// SyncMedia accepts an uploaded media payload or fetches all media from media-service and indexes them
func (fc *FeedController) SyncMedia(c *gin.Context) {
	var payload struct {
		MediaID      string  `json:"media_id"`
		Title        string  `json:"title"`
		Description  string  `json:"description"`
		MediaType    string  `json:"media_type"`
		Artist       string  `json:"artist"`
		Duration     float64 `json:"duration"`
		StreamURL    string  `json:"stream_url"`
		ThumbnailURL string  `json:"thumbnail_url"`
		UploaderID   string  `json:"uploader_id"`
		UploaderName string  `json:"uploader_name"`
		Tags         string  `json:"tags"`
	}

	if err := c.ShouldBindJSON(&payload); err == nil && payload.MediaID != "" {
		var existing models.CatalogItem
		if err := fc.DB.First(&existing, "media_id = ?", payload.MediaID).Error; err == nil {
			// update
			existing.Title = payload.Title
			existing.Description = payload.Description
			existing.ThumbnailURL = payload.ThumbnailURL
			fc.DB.Save(&existing)
			c.JSON(http.StatusOK, gin.H{"message": "Item updated", "item": existing})
			return
		}

		item := models.CatalogItem{
			ID:           uuid.New().String(),
			MediaID:      payload.MediaID,
			Title:        payload.Title,
			Description:  payload.Description,
			MediaType:    payload.MediaType,
			Artist:       payload.Artist,
			Duration:     payload.Duration,
			StreamURL:    payload.StreamURL,
			ThumbnailURL: payload.ThumbnailURL,
			UploaderID:   payload.UploaderID,
			UploaderName: payload.UploaderName,
			Tags:         payload.Tags,
			Views:        1,
			Likes:        0,
			CreatedAt:    time.Now(),
		}
		fc.DB.Create(&item)
		fc.Cache.Invalidate(context.Background(), "catalog:*")
		c.JSON(http.StatusCreated, gin.H{"message": "Item added to catalog", "item": item})
		return
	}

	// If no payload, trigger full sync from media-service
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/media", fc.Cfg.MediaServiceURL))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to media-service"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Items []struct {
			ID           string  `json:"id"`
			Title        string  `json:"title"`
			Description  string  `json:"description"`
			MediaType    string  `json:"media_type"`
			Artist       string  `json:"artist"`
			Duration     float64 `json:"duration"`
			StreamURL    string  `json:"stream_url"`
			ThumbnailURL string  `json:"thumbnail_url"`
			UploaderID   string  `json:"uploader_id"`
			UploaderName string  `json:"uploader_name"`
			Tags         string  `json:"tags"`
			CreatedAt    time.Time `json:"created_at"`
		} `json:"items"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse media-service response"})
		return
	}

	synced := 0
	for _, m := range result.Items {
		var count int64
		fc.DB.Model(&models.CatalogItem{}).Where("media_id = ?", m.ID).Count(&count)
		if count == 0 {
			item := models.CatalogItem{
				ID:           uuid.New().String(),
				MediaID:      m.ID,
				Title:        m.Title,
				Description:  m.Description,
				MediaType:    m.MediaType,
				Artist:       m.Artist,
				Duration:     m.Duration,
				StreamURL:    m.StreamURL,
				ThumbnailURL: m.ThumbnailURL,
				UploaderID:   m.UploaderID,
				UploaderName: m.UploaderName,
				Tags:         m.Tags,
				Views:        10,
				Likes:        2,
				CreatedAt:    m.CreatedAt,
			}
			fc.DB.Create(&item)
			synced++
		}
	}

	fc.Cache.Invalidate(context.Background(), "catalog:*")
	c.JSON(http.StatusOK, gin.H{"message": "Sync completed", "synced_count": synced})
}
