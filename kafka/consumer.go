package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"streamly/catalog-service/cache"
	"streamly/catalog-service/models"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type Consumer struct {
	reader *kafka.Reader
	db     *gorm.DB
	cache  *cache.CacheManager
}

type MediaUploadedEvent struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	MediaType    string    `json:"media_type"`
	Artist       string    `json:"artist"`
	Duration     float64   `json:"duration"`
	StreamURL    string    `json:"stream_url"`
	ThumbnailURL string    `json:"thumbnail_url"`
	UploaderID   string    `json:"uploader_id"`
	UploaderName string    `json:"uploader_name"`
	Tags         string    `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
}

func NewConsumer(brokers string, db *gorm.DB, cache *cache.CacheManager) *Consumer {
	brokerList := strings.Split(brokers, ",")
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokerList,
		Topic:          "media.uploaded",
		GroupID:        "catalog-service-syncer",
		MinBytes:       10e3,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})
	log.Printf("Kafka consumer initialized for topic 'media.uploaded' (group: catalog-service-syncer)")
	return &Consumer{reader: reader, db: db, cache: cache}
}

func (c *Consumer) Start(ctx context.Context) {
	log.Println("Catalog Kafka consumer listening for media upload events...")
	for {
		select {
		case <-ctx.Done():
			log.Println("Catalog Kafka consumer stopped")
			return
		default:
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				time.Sleep(2 * time.Second)
				continue
			}

			var event MediaUploadedEvent
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Failed to unmarshal media.uploaded event: %v", err)
				continue
			}

			var existing models.CatalogItem
			if err := c.db.First(&existing, "media_id = ?", event.ID).Error; err == nil {
				// Update
				existing.Title = event.Title
				existing.Description = event.Description
				existing.ThumbnailURL = event.ThumbnailURL
				existing.StreamURL = event.StreamURL
				c.db.Save(&existing)
			} else {
				// Create new catalog item
				item := models.CatalogItem{
					ID:           uuid.New().String(),
					MediaID:      event.ID,
					Title:        event.Title,
					Description:  event.Description,
					MediaType:    event.MediaType,
					Artist:       event.Artist,
					Duration:     event.Duration,
					StreamURL:    event.StreamURL,
					ThumbnailURL: event.ThumbnailURL,
					UploaderID:   event.UploaderID,
					UploaderName: event.UploaderName,
					Tags:         event.Tags,
					Views:        1,
					Likes:        0,
					CreatedAt:    event.CreatedAt,
				}
				if err := c.db.Create(&item).Error; err != nil {
					log.Printf("Failed to insert catalog item from Kafka event: %v", err)
					continue
				}
			}

			// Invalidate Redis feed caches so new media appears instantly
			c.cache.Invalidate(ctx, "catalog:*")
			log.Printf("Kafka consumer: synchronized media %s (%s) into catalog and invalidated Redis cache", event.ID, event.Title)
		}
	}
}

func (c *Consumer) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}
