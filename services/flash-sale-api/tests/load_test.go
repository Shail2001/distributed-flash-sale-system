package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
)

func TestConcurrentPurchases(t *testing.T) {
	const baseURL = "http://localhost:8080"
	const totalRequests = 200
	const inventoryCount = 100

	// Reset first
	http.Post(baseURL+"/reset", "application/json", nil)

	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	rejected := 0
	errors := 0

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]interface{}{
				"customer_id": fmt.Sprintf("concurrent-customer-%d", id),
				"quantity":    1,
			})
			resp, err := http.Post(baseURL+"/purchase", "application/json", bytes.NewBuffer(body))
			if err != nil {
				mu.Lock()
				errors++
				mu.Unlock()
				return
			}
			defer resp.Body.Close()
			mu.Lock()
			if resp.StatusCode == 201 {
				accepted++
			} else if resp.StatusCode == 409 {
				rejected++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	fmt.Printf("\n=== CONCURRENCY TEST RESULTS ===\n")
	fmt.Printf("Total requests:  %d\n", totalRequests)
	fmt.Printf("Accepted (201):  %d\n", accepted)
	fmt.Printf("Rejected (409):  %d\n", rejected)
	fmt.Printf("Errors:          %d\n", errors)
	fmt.Printf("Oversell:        %v\n", accepted > inventoryCount)
	fmt.Printf("================================\n\n")

	if accepted > inventoryCount {
		t.Errorf("OVERSELL DETECTED: %d orders accepted but only %d items in inventory", accepted, inventoryCount)
	}
	if accepted+rejected+errors != totalRequests {
		t.Errorf("Request count mismatch: %d+%d+%d != %d", accepted, rejected, errors, totalRequests)
	}
}
