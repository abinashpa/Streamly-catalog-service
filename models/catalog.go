package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CatalogItem struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	MediaID      string    `gorm:"type:uuid;uniqueIndex;not null" json:"media_id"`
	Title        string    `gorm:"type:varchar(255);not null;index" json:"title"`
	Description  string    `gorm:"type:text" json:"description"`
	MediaType    string    `gorm:"type:varchar(20);not null;index" json:"media_type"` // video, short, music
	Artist       string    `gorm:"type:varchar(100);index" json:"artist,omitempty"`
	Duration     float64   `gorm:"default:0" json:"duration"`
	StreamURL    string    `gorm:"type:text;not null" json:"stream_url"`
	ThumbnailURL string    `gorm:"type:text" json:"thumbnail_url"`
	UploaderID   string    `gorm:"type:uuid;index" json:"uploader_id"`
	UploaderName string    `gorm:"type:varchar(100)" json:"uploader_name"`
	Tags         string    `gorm:"type:text" json:"tags"`
	Views        int64     `gorm:"default:0;index" json:"views"`
	Likes        int64     `gorm:"default:0" json:"likes"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Comment struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	CatalogID    string    `gorm:"type:uuid;index;not null" json:"catalog_id"`
	UserID       string    `gorm:"type:uuid;index;not null" json:"user_id"`
	UserName     string    `gorm:"type:varchar(100);not null" json:"user_name"`
	UserAvatar   string    `gorm:"type:text" json:"user_avatar"`
	Content      string    `gorm:"type:text;not null" json:"content"`
	Likes        int64     `gorm:"default:0" json:"likes"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MediaLike struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	CatalogID string    `gorm:"type:uuid;uniqueIndex:idx_catalog_user;not null" json:"catalog_id"`
	UserID    string    `gorm:"type:uuid;uniqueIndex:idx_catalog_user;not null" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *CatalogItem) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return
}

func (c *Comment) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return
}

func (l *MediaLike) BeforeCreate(tx *gorm.DB) (err error) {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return
}
