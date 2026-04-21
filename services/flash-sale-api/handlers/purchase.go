package handlers

import (
	"errors"
	"log"
	"net/http"

	redisclient "flash-sale-api/redis"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type purchaseRequest struct {
	CustomerID string `json:"customer_id" binding:"required"`
	Quantity   int    `json:"quantity"    binding:"required,min=1"`
}

type purchaseResponse struct {
	OrderID            string `json:"order_id"`
	CustomerID         string `json:"customer_id"`
	Status             string `json:"status"`
	RemainingInventory int64  `json:"remaining_inventory"`
}

type soldOutResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Purchase handles POST /purchase.
// Critical path: atomic DECR on Redis → publish to SQS → 201 or 409.
func (h *Handler) Purchase(c *gin.Context) {
	var req purchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "message": err.Error()})
		return
	}

	ctx := c.Request.Context()
	const itemID = "flash-sale-item"

	// Step 1: Reserve inventory using configured strategy.
	remaining, err := redisclient.ReserveInventory(ctx, h.rdb, itemID, req.Quantity, h.inventoryMode)
	if err != nil {
		switch {
		case errors.Is(err, redisclient.ErrSoldOut):
			c.JSON(http.StatusConflict, soldOutResponse{
				Error:   "SOLD_OUT",
				Message: "No inventory remaining",
			})
			return
		case errors.Is(err, redisclient.ErrUnknownStrategy):
			log.Printf("Invalid inventory strategy configured: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid inventory strategy"})
			return
		default:
			log.Printf("Redis reserve error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "redis error"})
			return
		}
	}

	// Step 2: Inventory reserved — generate order ID and publish to SQS.
	orderID := uuid.New().String()

	if err := h.publisher.PublishOrder(ctx, orderID, req.CustomerID, itemID, req.Quantity); err != nil {
		// SQS publish failed — roll back reservation so inventory stays consistent.
		log.Printf("SQS publish error for order %s: %v — rolling back reservation", orderID, err)
		if incrErr := redisclient.IncrInventoryBy(ctx, h.rdb, itemID, req.Quantity); incrErr != nil {
			log.Printf("Redis rollback INCR error: %v", incrErr)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue order"})
		return
	}

	log.Printf("Purchase accepted: order_id=%s customer_id=%s remaining=%d", orderID, req.CustomerID, remaining)

	c.JSON(http.StatusCreated, purchaseResponse{
		OrderID:            orderID,
		CustomerID:         req.CustomerID,
		Status:             "accepted",
		RemainingInventory: remaining,
	})
}
