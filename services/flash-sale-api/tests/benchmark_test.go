package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func BenchmarkPurchase(b *testing.B) {
	const baseURL = "http://localhost:8080"

	// Reset before benchmark
	http.Post(baseURL+"/reset", "application/json", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(map[string]interface{}{
			"customer_id": fmt.Sprintf("bench-customer-%d", i),
			"quantity":    1,
		})
		resp, err := http.Post(baseURL+"/purchase", "application/json", bytes.NewBuffer(body))
		if err == nil {
			resp.Body.Close()
		}
		// Reset every 90 requests to avoid all sold-out
		if i%90 == 0 {
			http.Post(baseURL+"/reset", "application/json", nil)
		}
	}
}

func BenchmarkInventory(b *testing.B) {
	const baseURL = "http://localhost:8080"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Get(baseURL + "/inventory")
		if err == nil {
			resp.Body.Close()
		}
	}
}
