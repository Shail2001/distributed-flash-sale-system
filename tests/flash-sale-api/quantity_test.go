package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flash-sale-api/handlers"
	redisclient "flash-sale-api/redis"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

type mockPublisher struct {
	called     bool
	customerID string
	itemID     string
	quantity   int
	err        error
}

func (m *mockPublisher) PublishOrder(_ context.Context, _ string, customerID, itemID string, quantity int) error {
	m.called = true
	m.customerID = customerID
	m.itemID = itemID
	m.quantity = quantity
	return m.err
}

func TestPurchaseDecrementsInventoryByQuantity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redisclient.NewClient(mr.Host(), mr.Port())
	t.Cleanup(func() { _ = rdb.Close() })

	if err := redisclient.SetInventory(context.Background(), rdb, "flash-sale-item", 10); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	publisher := &mockPublisher{}
	handler := handlers.NewHandler(rdb, publisher, 10)
	router := gin.New()
	router.POST("/purchase", handler.Purchase)

	reqBody := []byte(`{"customer_id":"user-1","quantity":3}`)
	req := httptest.NewRequest(http.MethodPost, "/purchase", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		RemainingInventory int64  `json:"remaining_inventory"`
		CustomerID         string `json:"customer_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RemainingInventory != 7 {
		t.Fatalf("expected remaining inventory 7, got %d", resp.RemainingInventory)
	}
	if !publisher.called || publisher.quantity != 3 || publisher.customerID != "user-1" || publisher.itemID != "flash-sale-item" {
		t.Fatalf("publisher received wrong payload: %+v", publisher)
	}

	remaining, err := redisclient.GetInventory(context.Background(), rdb, "flash-sale-item")
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if remaining != 7 {
		t.Fatalf("expected redis inventory 7, got %d", remaining)
	}
}

func TestPurchaseRollsBackEntireQuantityWhenPublishFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redisclient.NewClient(mr.Host(), mr.Port())
	t.Cleanup(func() { _ = rdb.Close() })

	if err := redisclient.SetInventory(context.Background(), rdb, "flash-sale-item", 10); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	publisher := &mockPublisher{err: context.DeadlineExceeded}
	handler := handlers.NewHandler(rdb, publisher, 10)
	router := gin.New()
	router.POST("/purchase", handler.Purchase)

	req := httptest.NewRequest(http.MethodPost, "/purchase", bytes.NewReader([]byte(`{"customer_id":"user-2","quantity":4}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}

	remaining, err := redisclient.GetInventory(context.Background(), rdb, "flash-sale-item")
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if remaining != 10 {
		t.Fatalf("expected redis inventory rollback to 10, got %d", remaining)
	}
}

func TestPurchaseRejectsWhenQuantityExceedsInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redisclient.NewClient(mr.Host(), mr.Port())
	t.Cleanup(func() { _ = rdb.Close() })

	if err := redisclient.SetInventory(context.Background(), rdb, "flash-sale-item", 2); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	publisher := &mockPublisher{}
	handler := handlers.NewHandler(rdb, publisher, 2)
	router := gin.New()
	router.POST("/purchase", handler.Purchase)

	req := httptest.NewRequest(http.MethodPost, "/purchase", bytes.NewReader([]byte(`{"customer_id":"user-3","quantity":3}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	if publisher.called {
		t.Fatal("publisher should not be called when inventory is insufficient")
	}

	remaining, err := redisclient.GetInventory(context.Background(), rdb, "flash-sale-item")
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("expected redis inventory to remain 2, got %d", remaining)
	}
}
