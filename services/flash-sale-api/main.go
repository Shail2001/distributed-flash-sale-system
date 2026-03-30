package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"flash-sale-api/handlers"
	redisclient "flash-sale-api/redis"
	sqsclient "flash-sale-api/sqs"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load env vars
	redisEndpoint := getEnv("REDIS_ENDPOINT", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	sqsQueueURL := getEnv("SQS_QUEUE_URL", "")
	inventoryCount := getEnvInt("INVENTORY_COUNT", 100)
	awsRegion := getEnv("AWS_REGION", "us-east-1")
	appPort := getEnv("APP_PORT", "8080")

	// Init Redis client
	rdb := redisclient.NewClient(redisEndpoint, redisPort)

	// Seed inventory on startup (NX = only if not already set)
	ctx := context.Background()
	err := rdb.SetNX(ctx, "inventory:flash-sale-item", inventoryCount, 0).Err()
	if err != nil {
		log.Fatalf("Failed to initialize Redis inventory: %v", err)
	}
	log.Printf("Redis connected. Inventory key initialized (NX) to %d", inventoryCount)

	// Init AWS config + SQS client
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion))
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	sqsSvc := sqs.NewFromConfig(cfg)
	publisher := sqsclient.NewPublisher(sqsSvc, sqsQueueURL)

	// Build handler dependencies
	h := handlers.NewHandler(rdb, publisher, inventoryCount)

	// Router
	r := gin.Default()
	r.GET("/health", h.Health)
	r.GET("/inventory", h.GetInventory)
	r.POST("/purchase", h.Purchase)
	r.POST("/reset", h.Reset)

	log.Printf("Flash Sale API starting on port %s", appPort)
	if err := r.Run(fmt.Sprintf(":%s", appPort)); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		n, err := strconv.Atoi(val)
		if err == nil {
			return n
		}
	}
	return fallback
}
