package orderflowtests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const defaultItemID = "flash-sale-item"

type testConfig struct {
	AWSRegion       string
	APIBaseURL      string
	OrdersTable     string
	InventoryTable  string
	ItemID          string
	ResetBeforeTest bool
	PollTimeout     time.Duration
	PollInterval    time.Duration
}

type purchaseResponse struct {
	OrderID            string `json:"order_id"`
	CustomerID         string `json:"customer_id"`
	Status             string `json:"status"`
	RemainingInventory int64  `json:"remaining_inventory"`
}

type inventoryResponse struct {
	ItemID    string `json:"item_id"`
	Remaining int64  `json:"remaining"`
	SoldOut   bool   `json:"sold_out"`
}

type orderRecord struct {
	OrderID    string `dynamodbav:"order_id"`
	CustomerID string `dynamodbav:"customer_id"`
	ItemID     string `dynamodbav:"item_id"`
	Quantity   int    `dynamodbav:"quantity"`
	Status     string `dynamodbav:"status"`
}

type inventoryRecord struct {
	ItemID          string `dynamodbav:"item_id"`
	ConfirmedOrders int64  `dynamodbav:"confirmed_orders"`
	LastOrderID     string `dynamodbav:"last_order_id"`
}

func TestAWSOrderFlowSmoke(t *testing.T) {
	cfg, ok := loadTestConfig(t)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.PollTimeout)
	defer cancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		t.Fatalf("load AWS config: %v", err)
	}

	ddb := dynamodb.NewFromConfig(awsCfg)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	if err := waitForHealthy(ctx, httpClient, cfg.APIBaseURL); err != nil {
		t.Fatalf("API health check failed: %v", err)
	}

	if cfg.ResetBeforeTest {
		if err := postReset(ctx, httpClient, cfg.APIBaseURL); err != nil {
			t.Fatalf("reset inventory: %v", err)
		}
	}

	beforeInventory, err := getInventory(ctx, httpClient, cfg.APIBaseURL)
	if err != nil {
		t.Fatalf("get inventory before purchase: %v", err)
	}
	if beforeInventory.Remaining < 1 {
		t.Fatalf("inventory is empty before test; remaining=%d", beforeInventory.Remaining)
	}

	beforeConfirmed, err := getConfirmedOrders(ctx, ddb, cfg.InventoryTable, cfg.ItemID)
	if err != nil {
		t.Fatalf("get confirmed_orders before purchase: %v", err)
	}

	customerID := fmt.Sprintf("aws-smoke-%d", time.Now().UnixNano())
	purchase, err := postPurchase(ctx, httpClient, cfg.APIBaseURL, customerID, 1)
	if err != nil {
		t.Fatalf("purchase request failed: %v", err)
	}

	t.Logf("purchase accepted: order_id=%s customer_id=%s remaining=%d", purchase.OrderID, purchase.CustomerID, purchase.RemainingInventory)

	record, err := waitForOrderRecord(ctx, ddb, cfg.OrdersTable, purchase.OrderID, cfg.PollInterval)
	if err != nil {
		t.Fatalf("order record was not persisted: %v", err)
	}

	if record.CustomerID != customerID {
		t.Fatalf("unexpected customer_id in order record: got %q want %q", record.CustomerID, customerID)
	}
	if record.ItemID != cfg.ItemID {
		t.Fatalf("unexpected item_id in order record: got %q want %q", record.ItemID, cfg.ItemID)
	}
	if record.Quantity != 1 {
		t.Fatalf("unexpected quantity in order record: got %d want 1", record.Quantity)
	}
	if record.Status != "confirmed" {
		t.Fatalf("unexpected order status: got %q want confirmed", record.Status)
	}

	expectedConfirmed := beforeConfirmed + 1
	if err := waitForConfirmedOrders(ctx, ddb, cfg.InventoryTable, cfg.ItemID, expectedConfirmed, purchase.OrderID, cfg.PollInterval); err != nil {
		t.Fatalf("inventory audit table was not updated: %v", err)
	}

	afterInventory, err := getInventory(ctx, httpClient, cfg.APIBaseURL)
	if err != nil {
		t.Fatalf("get inventory after purchase: %v", err)
	}
	if afterInventory.Remaining != purchase.RemainingInventory {
		t.Fatalf("inventory mismatch: GET /inventory=%d purchase_response=%d", afterInventory.Remaining, purchase.RemainingInventory)
	}
}

func loadTestConfig(t *testing.T) (testConfig, bool) {
	t.Helper()

	cfg := testConfig{
		AWSRegion:       os.Getenv("AWS_REGION"),
		APIBaseURL:      strings.TrimRight(firstNonEmpty(os.Getenv("API_BASE_URL"), prefixedHTTP(os.Getenv("ALB_DNS_NAME"))), "/"),
		OrdersTable:     os.Getenv("DYNAMODB_TABLE"),
		InventoryTable:  os.Getenv("INVENTORY_TABLE"),
		ItemID:          firstNonEmpty(os.Getenv("ITEM_ID"), defaultItemID),
		ResetBeforeTest: boolFromEnv("RESET_BEFORE_TEST", true),
		PollTimeout:     durationFromEnv("ORDER_FLOW_TIMEOUT_SECONDS", 90*time.Second),
		PollInterval:    durationFromEnv("ORDER_FLOW_POLL_INTERVAL_MS", 2*time.Second),
	}

	var missing []string
	if cfg.AWSRegion == "" {
		missing = append(missing, "AWS_REGION")
	}
	if cfg.APIBaseURL == "" {
		missing = append(missing, "API_BASE_URL or ALB_DNS_NAME")
	}
	if cfg.OrdersTable == "" {
		missing = append(missing, "DYNAMODB_TABLE")
	}
	if cfg.InventoryTable == "" {
		missing = append(missing, "INVENTORY_TABLE")
	}

	if len(missing) > 0 {
		t.Skipf("skipping live AWS order-flow smoke test; missing env vars: %s", strings.Join(missing, ", "))
		return testConfig{}, false
	}

	return cfg, true
}

func waitForHealthy(ctx context.Context, client *http.Client, baseURL string) error {
	return poll(ctx, 2*time.Second, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			return false, err
		}

		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		return resp.StatusCode == http.StatusOK, nil
	})
}

func postReset(ctx context.Context, client *http.Client, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/reset", nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected reset status code: %d", resp.StatusCode)
	}

	return nil
}

func postPurchase(ctx context.Context, client *http.Client, baseURL, customerID string, quantity int) (purchaseResponse, error) {
	payload, err := json.Marshal(map[string]any{
		"customer_id": customerID,
		"quantity":    quantity,
	})
	if err != nil {
		return purchaseResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/purchase", bytes.NewReader(payload))
	if err != nil {
		return purchaseResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return purchaseResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return purchaseResponse{}, fmt.Errorf("unexpected purchase status code: %d", resp.StatusCode)
	}

	var parsed purchaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return purchaseResponse{}, err
	}

	return parsed, nil
}

func getInventory(ctx context.Context, client *http.Client, baseURL string) (inventoryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/inventory", nil)
	if err != nil {
		return inventoryResponse{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return inventoryResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return inventoryResponse{}, fmt.Errorf("unexpected inventory status code: %d", resp.StatusCode)
	}

	var parsed inventoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return inventoryResponse{}, err
	}

	return parsed, nil
}

func waitForOrderRecord(ctx context.Context, client *dynamodb.Client, tableName, orderID string, interval time.Duration) (orderRecord, error) {
	var record orderRecord
	err := poll(ctx, interval, func() (bool, error) {
		out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: &tableName,
			Key: map[string]types.AttributeValue{
				"order_id": &types.AttributeValueMemberS{Value: orderID},
			},
		})
		if err != nil {
			return false, err
		}
		if len(out.Item) == 0 {
			return false, nil
		}
		if err := attributevalue.UnmarshalMap(out.Item, &record); err != nil {
			return false, err
		}
		return true, nil
	})

	return record, err
}

func getConfirmedOrders(ctx context.Context, client *dynamodb.Client, tableName, itemID string) (int64, error) {
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &tableName,
		Key: map[string]types.AttributeValue{
			"item_id": &types.AttributeValueMemberS{Value: itemID},
		},
	})
	if err != nil {
		return 0, err
	}
	if len(out.Item) == 0 {
		return 0, nil
	}

	var record inventoryRecord
	if err := attributevalue.UnmarshalMap(out.Item, &record); err != nil {
		return 0, err
	}

	return record.ConfirmedOrders, nil
}

func waitForConfirmedOrders(ctx context.Context, client *dynamodb.Client, tableName, itemID string, minConfirmed int64, lastOrderID string, interval time.Duration) error {
	return poll(ctx, interval, func() (bool, error) {
		out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: &tableName,
			Key: map[string]types.AttributeValue{
				"item_id": &types.AttributeValueMemberS{Value: itemID},
			},
		})
		if err != nil {
			return false, err
		}
		if len(out.Item) == 0 {
			return false, nil
		}

		var record inventoryRecord
		if err := attributevalue.UnmarshalMap(out.Item, &record); err != nil {
			return false, err
		}

		return record.ConfirmedOrders >= minConfirmed && record.LastOrderID == lastOrderID, nil
	})
}

func poll(ctx context.Context, interval time.Duration, fn func() (bool, error)) error {
	for {
		done, err := fn()
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}

	if strings.HasSuffix(key, "_MS") {
		return time.Duration(n) * time.Millisecond
	}

	return time.Duration(n) * time.Second
}

func boolFromEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}

	switch raw {
	case "1", "true", "yes", "y":
		return true
	case "0", "false", "no", "n":
		return false
	default:
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func prefixedHTTP(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	return "http://" + host
}
