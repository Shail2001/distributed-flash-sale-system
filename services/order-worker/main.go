package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	appconfig "order-worker/internal/config"
	"order-worker/internal/metrics"
	"order-worker/internal/service"
	"order-worker/internal/store"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := appconfig.Load()
	if err != nil {
		logger.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		logger.Error("failed to load AWS config", slog.Any("error", err))
		os.Exit(1)
	}

	sqsClient := sqs.NewFromConfig(awsCfg)
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	orderStore := store.NewDynamoOrderStore(dynamoClient, cfg.DynamoDBTable, cfg.InventoryTable)
	collector := &metrics.Collector{}
	svc := service.New(cfg, sqsClient, orderStore, logger, collector)

	logger.Info("order worker starting",
		slog.String("aws_region", cfg.AWSRegion),
		slog.String("dynamodb_table", cfg.DynamoDBTable),
		slog.String("inventory_table", cfg.InventoryTable),
		slog.Int("num_workers", cfg.NumWorkers),
	)

	if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("order worker stopped with error", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("order worker stopped cleanly")
}
