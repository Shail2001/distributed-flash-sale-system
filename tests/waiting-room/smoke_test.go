package waitingroomtests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const defaultBaseURL = "http://localhost:8080"

func baseURL() string {
	if u := os.Getenv("WAITING_ROOM_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBaseURL
}

// strategy returns the QUEUE_STRATEGY env var, defaulting to "timestamp".
func strategy() string {
	if s := os.Getenv("QUEUE_STRATEGY"); s != "" {
		return s
	}
	return "timestamp"
}

type joinResponse struct {
	UserID        string `json:"user_id"`
	Position      int64  `json:"position"`
	AdmittedCount int64  `json:"admitted_count"`
	Admitted      bool   `json:"admitted"`
	Token         string `json:"token"`
	Strategy      string `json:"strategy"`
}

type positionResponse struct {
	UserID        string `json:"user_id"`
	Position      int64  `json:"position"`
	AdmittedCount int64  `json:"admitted_count"`
	Admitted      bool   `json:"admitted"`
	Token         string `json:"token"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func postReset(t *testing.T) {
	t.Helper()
	resp, err := http.Post(baseURL()+"/queue/reset", "application/json", nil)
	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset returned %d", resp.StatusCode)
	}
}

func postJoin(t *testing.T, userID string) joinResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"user_id": userID})
	resp, err := http.Post(baseURL()+"/queue/join", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("join failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("join returned %d", resp.StatusCode)
	}
	var r joinResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatalf("decode join response: %v", err)
	}
	return r
}

func getPosition(t *testing.T, userID string) positionResponse {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/queue/position?user_id=%s", baseURL(), userID))
	if err != nil {
		t.Fatalf("position failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("position returned %d", resp.StatusCode)
	}
	var r positionResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatalf("decode position response: %v", err)
	}
	return r
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	resp, err := http.Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "healthy" {
		t.Fatalf("expected status=healthy, got %q", body["status"])
	}
}

func TestJoinAssignsPosition(t *testing.T) {
	postReset(t)
	r := postJoin(t, "user-smoke-1")
	if r.Position != 1 {
		t.Fatalf("expected position 1, got %d", r.Position)
	}
	if r.UserID != "user-smoke-1" {
		t.Fatalf("expected user_id user-smoke-1, got %q", r.UserID)
	}
}

func TestJoinIdempotent(t *testing.T) {
	postReset(t)
	r1 := postJoin(t, "user-idem-1")
	r2 := postJoin(t, "user-idem-1")
	if r1.Position != r2.Position {
		t.Fatalf("idempotency failed: first=%d second=%d", r1.Position, r2.Position)
	}
}

func TestSequentialPositions(t *testing.T) {
	postReset(t)
	for i := 1; i <= 5; i++ {
		r := postJoin(t, fmt.Sprintf("user-seq-%d", i))
		if r.Position != int64(i) {
			t.Fatalf("user %d got position %d, expected %d", i, r.Position, i)
		}
	}
}

func TestPositionNotFound(t *testing.T) {
	postReset(t)
	resp, err := http.Get(baseURL() + "/queue/position?user_id=nobody")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPositionMissingParam(t *testing.T) {
	resp, err := http.Get(baseURL() + "/queue/position")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestJoinBadRequest(t *testing.T) {
	body, _ := json.Marshal(map[string]string{})
	resp, err := http.Post(baseURL()+"/queue/join", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAdmissionToken(t *testing.T) {
	postReset(t)
	postJoin(t, "user-token-1")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r := getPosition(t, "user-token-1")
		if r.Admitted {
			if r.Token == "" {
				t.Fatal("admitted=true but token is empty")
			}
			t.Logf("admitted after polling, token=%s...", r.Token[:8])
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("user was not admitted within 5 seconds — check ADMISSION_RATE")
}

func TestReset(t *testing.T) {
	postReset(t)
	postJoin(t, "user-reset-1")
	postReset(t)

	resp, err := http.Get(baseURL() + "/queue/position?user_id=user-reset-1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after reset, got %d", resp.StatusCode)
	}
}

// ── Concurrency Test — Experiment 1 proxy ─────────────────────────────────────

// TestConcurrentJoins fires N simultaneous joins and measures collision rate.
//
// Strategy A (timestamp):     collisions are EXPECTED — this is what Experiment 1 measures.
//                              The test logs the collision rate but does NOT fail.
// Strategy B (timestamp_incr): collisions must be ZERO — the INCR tiebreaker
//                              guarantees unique scores. The test fails if any occur.
//
// Run with Strategy A: go test -v ./...
// Run with Strategy B: QUEUE_STRATEGY=timestamp_incr go test -v ./...
func TestConcurrentJoins(t *testing.T) {
	const N = 100
	postReset(t)

	type result struct {
		userID   string
		position int64
	}

	results := make([]result, N)
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := fmt.Sprintf("concurrent-user-%d", id)
			r := postJoin(t, userID)
			results[id] = result{userID: userID, position: r.Position}
		}(i)
	}
	wg.Wait()

	// Count collisions: positions assigned to more than one user.
	seen := make(map[int64]string) // position → first userID
	collisions := 0
	for _, r := range results {
		if existing, dup := seen[r.position]; dup {
			collisions++
			t.Logf("collision: position %d assigned to %q and %q", r.position, existing, r.userID)
		} else {
			seen[r.position] = r.userID
		}
	}

	collisionRate := float64(collisions) / float64(N) * 100

	t.Logf("=== Concurrent Join Results ===")
	t.Logf("Strategy:       %s", strategy())
	t.Logf("Total users:    %d", N)
	t.Logf("Unique positions assigned: %d", len(seen))
	t.Logf("Collisions:     %d (%.1f%%)", collisions, collisionRate)

	// Strategy B must have zero collisions — INCR tiebreaker guarantees uniqueness.
	// Strategy A collisions are expected and just logged for Experiment 1 data.
	if strategy() == "timestamp_incr" && collisions > 0 {
		t.Errorf("FAIL: strategy=timestamp_incr must have zero collisions, got %d", collisions)
	}
}
