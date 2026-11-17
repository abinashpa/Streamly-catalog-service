package main

import (
	"context"
	"log"

	"streamly/catalog-service/cache"
	"streamly/catalog-service/config"
	cataloggrpc "streamly/catalog-service/grpc"
	catalogkafka "streamly/catalog-service/kafka"
	"streamly/catalog-service/routes"
)

func main() {
	cfg := config.LoadConfig()
	db := config.InitDB(cfg)
	rdb := config.InitRedis(cfg)

	cacheMgr := cache.NewCacheManager(rdb)

	// Start Kafka consumer in background
	consumer := catalogkafka.NewConsumer(cfg.KafkaBrokers, db, cacheMgr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer consumer.Close()
	go consumer.Start(ctx)

	// Start gRPC server in background
	grpcServer := cataloggrpc.NewServer(db, cacheMgr, cfg)
	go func() {
		if err := grpcServer.Start(cfg.GRPCPort); err != nil {
			log.Printf("catalog-service gRPC server error: %v", err)
		}
	}()

	// Start Gin HTTP server
	r := routes.SetupRouter(db, rdb, cfg)

	log.Printf("catalog-service HTTP running on port %s, gRPC on port %s", cfg.Port, cfg.GRPCPort)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
