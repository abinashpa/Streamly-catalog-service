package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"streamly/catalog-service/models"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Port                 string
	DBHost               string
	DBPort               string
	DBUser               string
	DBPass               string
	DBName               string
	RedisAddr            string
	RedisPass            string
	MediaServiceURL      string
	MediaServiceGRPCAddr string
	KafkaBrokers         string
	GRPCPort             string
}

func GetEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func LoadConfig() Config {
	return Config{
		Port:                 GetEnv("PORT", "8082"),
		DBHost:               GetEnv("DB_HOST", "localhost"),
		DBPort:               GetEnv("DB_PORT", "5432"),
		DBUser:               GetEnv("DB_USER", "streamly"),
		DBPass:               GetEnv("DB_PASSWORD", "streamly_password"),
		DBName:               GetEnv("DB_NAME", "streamly_catalog"),
		RedisAddr:            GetEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:            GetEnv("REDIS_PASSWORD", ""),
		MediaServiceURL:      GetEnv("MEDIA_SERVICE_URL", "http://media-service:8081"),
		MediaServiceGRPCAddr: GetEnv("MEDIA_SERVICE_GRPC_ADDR", "media-service:50051"),
		KafkaBrokers:         GetEnv("KAFKA_BROKERS", "kafka:9092"),
		GRPCPort:             GetEnv("GRPC_PORT", "50052"),
	}
}

func InitDB(cfg Config) *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		cfg.DBHost, cfg.DBUser, cfg.DBPass, cfg.DBName, cfg.DBPort,
	)

	var db *gorm.DB
	var err error

	for i := 0; i < 15; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
		if err == nil {
			break
		}
		log.Printf("Connecting to DB (%s:%s/%s)... retry %d/15", cfg.DBHost, cfg.DBPort, cfg.DBName, i+1)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to catalog database: %v", err)
	}

	log.Println("Database connection established for catalog-service")

	if err := db.AutoMigrate(&models.CatalogItem{}, &models.Comment{}, &models.MediaLike{}); err != nil {
		log.Fatalf("Catalog auto-migration failed: %v", err)
	}

	return db
}

func InitRedis(cfg Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Printf("Warning: Redis ping failed (%v). Continuing without active cache.", err)
	} else {
		log.Println("Redis connected successfully in catalog-service")
	}

	return rdb
}
