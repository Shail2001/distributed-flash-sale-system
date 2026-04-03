package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"order-worker/internal/config"
	"order-worker/internal/metrics"
	"order-worker/internal/model"
	"order-worker/internal/retry"
	"order-worker/internal/store"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
)

// Service polls SQS, fans messages out to workers, and persists orders.
type Service struct {
	cfg     config.Config
	sqs     *sqs.Client
	store   store.OrderStore
	logger  *slog.Logger
	metrics *metrics.Collector
}

type job struct {
	message sqstypes.Message
}

func New(cfg config.Config, sqsClient *sqs.Client, orderStore store.OrderStore, logger *slog.Logger, collector *metrics.Collector) *Service {
	return &Service{
		cfg:     cfg,
		sqs:     sqsClient,
		store:   orderStore,
		logger:  logger,
		metrics: collector,
	}
}

// Run starts the polling loop and worker pool and blocks until shutdown.
func (s *Service) Run(ctx context.Context) error {
	jobs := make(chan job, s.cfg.NumWorkers*int(s.cfg.MaxMessagesPerReceive))

	var workersWG sync.WaitGroup
	for i := 0; i < s.cfg.NumWorkers; i++ {
		workersWG.Add(1)
		go func(workerID int) {
			defer workersWG.Done()
			s.runWorker(ctx, workerID, jobs)
		}(i + 1)
	}

	metricsDone := make(chan struct{})
	go func() {
		defer close(metricsDone)
		s.metrics.RunLogger(ctx, s.logger, s.cfg.MetricsInterval)
	}()

	pollErr := s.poll(ctx, jobs)
	close(jobs)
	workersWG.Wait()
	<-metricsDone

	if errors.Is(pollErr, context.Canceled) || errors.Is(pollErr, context.DeadlineExceeded) {
		return nil
	}

	return pollErr
}

func (s *Service) poll(ctx context.Context, jobs chan<- job) error {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stopping SQS poller")
			return ctx.Err()
		default:
		}

		resp, err := s.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(s.cfg.SQSQueueURL),
			MaxNumberOfMessages: s.cfg.MaxMessagesPerReceive,
			WaitTimeSeconds:     s.cfg.WaitTimeSeconds,
			VisibilityTimeout:   s.cfg.VisibilityTimeoutSeconds,
			AttributeNames:      []sqstypes.QueueAttributeName{},
			MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{
				sqstypes.MessageSystemAttributeNameApproximateReceiveCount,
			},
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			s.logger.Error("failed to receive SQS messages", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.cfg.IdleSleep):
			}
			continue
		}

		if len(resp.Messages) == 0 {
			continue
		}

		for _, msg := range resp.Messages {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case jobs <- job{message: msg}:
			}
		}
	}
}

func (s *Service) runWorker(ctx context.Context, workerID int, jobs <-chan job) {
	workerLogger := s.logger.With(slog.Int("worker_id", workerID))

	for {
		select {
		case <-ctx.Done():
			workerLogger.Info("worker shutting down")
			return
		case job, ok := <-jobs:
			if !ok {
				workerLogger.Info("worker drained")
				return
			}

			if err := s.handleMessage(ctx, workerLogger, job.message); err != nil {
				s.metrics.IncFailed()
				workerLogger.Error("message processing failed",
					slog.String("message_id", aws.ToString(job.message.MessageId)),
					slog.Any("error", err),
				)
			}
		}
	}
}

func (s *Service) handleMessage(ctx context.Context, logger *slog.Logger, msg sqstypes.Message) error {
	var order model.Order
	if err := json.Unmarshal([]byte(aws.ToString(msg.Body)), &order); err != nil {
		return fmt.Errorf("decode message body: %w", err)
	}

	if err := order.Validate(); err != nil {
		return fmt.Errorf("validate order: %w", err)
	}

	var saveResult store.SaveResult
	saveErr := retry.Do(ctx, s.cfg.RetryAttempts, s.cfg.RetryBaseDelay, func() error {
		result, err := s.store.SaveOrder(ctx, order)
		if err != nil && !isRetryable(err) {
			return retry.Permanent(err)
		}
		saveResult = result
		return err
	})
	if saveErr != nil {
		return saveErr
	}

	if saveResult.Inserted {
		s.metrics.IncConfirmed()
		logger.Info("order confirmed",
			slog.String("order_id", order.OrderID),
			slog.String("user_id", order.UserID),
			slog.String("item_id", order.ItemID),
			slog.Uint64("confirmed_orders", s.metrics.Confirmed()),
		)
	} else {
		s.metrics.IncDuplicate()
		logger.Info("duplicate order ignored",
			slog.String("order_id", order.OrderID),
			slog.String("user_id", order.UserID),
		)
	}

	if err := retry.Do(ctx, s.cfg.RetryAttempts, s.cfg.RetryBaseDelay, func() error {
		_, err := s.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(s.cfg.SQSQueueURL),
			ReceiptHandle: msg.ReceiptHandle,
		})
		return err
	}); err != nil {
		return fmt.Errorf("delete processed message: %w", err)
	}

	return nil
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorFault() == smithy.FaultServer
	}

	return true
}
