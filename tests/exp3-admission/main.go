// Experiment 3 — Admission-rate tuning.
//
// Full path: N users join the waiting room, the admission ticker releases
// users at a configured rate, each admitted user attempts a purchase.
// We measure time-to-sellout, peak SQS depth, and wasted admissions
// (users admitted after inventory is exhausted).
//
// Prerequisites:
//   - waiting-room running on WAITING_ROOM_URL
//   - flash-sale-api running on API_BASE_URL with REQUIRE_ADMISSION_TOKEN=true
//   - order-worker consuming SQS in background
//
// Env knobs:
//   WAITING_ROOM_URL       (default http://localhost:8080)
//   API_BASE_URL           (default http://localhost:8081)
//   USERS                  (default 500)
//   INVENTORY              (default 100)
//   ADMISSION_RATE_LABEL   (default "unknown") — for output filename
//   RUN_DURATION_SECONDS   (default 60)
//   RESULTS_DIR            (default ./tests/results)
//   SQS_POLL_COMMAND       (optional) — shell cmd that prints SQS depth (int)
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type joinReq struct {
	UserID string `json:"user_id"`
}
type joinResp struct {
	UserID   string `json:"user_id"`
	Position int64  `json:"position"`
	Admitted bool   `json:"admitted"`
	Token    string `json:"token"`
	Strategy string `json:"strategy"`
}
type purchaseReq struct {
	CustomerID string `json:"customer_id"`
	Quantity   int    `json:"quantity"`
}
type positionResp struct {
	UserID        string `json:"user_id"`
	Position      int64  `json:"position"`
	Admitted      bool   `json:"admitted"`
	Token         string `json:"token"`
	AdmittedCount int64  `json:"admitted_count"`
}

func main() {
	wrURL := getenv("WAITING_ROOM_URL", "http://localhost:8080")
	apiURL := getenv("API_BASE_URL", "http://localhost:8081")
	users := atoi(getenv("USERS", "500"))
	inventory := atoi(getenv("INVENTORY", "100"))
	label := getenv("ADMISSION_RATE_LABEL", "unknown")
	durSec := atoi(getenv("RUN_DURATION_SECONDS", "60"))
	resultsDir := getenv("RESULTS_DIR", "./tests/results")
	sqsCmd := os.Getenv("SQS_POLL_COMMAND")
	_ = os.MkdirAll(resultsDir, 0o755)

	fmt.Printf("==> Experiment 3 | label=%s users=%d inventory=%d dur=%ds\n", label, users, inventory, durSec)

	// Reset waiting room and inventory so each run starts from a clean slate.
	_ = post(wrURL+"/queue/reset", nil)
	_ = post(apiURL+"/reset", nil)
	time.Sleep(500 * time.Millisecond)

	client := &http.Client{Timeout: 30 * time.Second}

	// Phase 1: all users join the waiting room (no delay).
	type user struct {
		id    string
		pos   int64
		token string
	}
	joined := make([]user, users)
	var wg sync.WaitGroup
	nowNano := time.Now().UnixNano()
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid := fmt.Sprintf("exp3-%s-%d-%d", label, nowNano, i)
			pos, tok, _ := join(client, wrURL, uid)
			joined[i] = user{id: uid, pos: pos, token: tok}
		}(i)
	}
	wg.Wait()

	// Phase 2: buyer loop — poll each user's position until admitted, then purchase.
	// We run until inventory sells out OR duration elapses.
	var admittedCnt, purchasedCnt, soldOutCnt, wastedCnt, errCnt int64
	var firstSellOut time.Time
	var peakSQS int64
	startExp := time.Now()
	deadline := startExp.Add(time.Duration(durSec) * time.Second)

	// SQS-depth sampler (best-effort). Runs in background.
	stopSample := make(chan struct{})
	if sqsCmd != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopSample:
					return
				case <-ticker.C:
					if depth, err := sampleSQS(sqsCmd); err == nil {
						if depth > atomic.LoadInt64(&peakSQS) {
							atomic.StoreInt64(&peakSQS, depth)
						}
					}
				}
			}
		}()
	}

	// Spawn a goroutine per user that retries /queue/position with backoff,
	// then POST /purchase once admitted.
	wg2 := sync.WaitGroup{}
	for i := 0; i < users; i++ {
		wg2.Add(1)
		go func(u user) {
			defer wg2.Done()
			// Use the join token if already admitted; otherwise poll position.
			token := u.token
			for token == "" {
				if time.Now().After(deadline) {
					return
				}
				tok, admitted, err := position(client, wrURL, u.id)
				if err == nil && admitted && tok != "" {
					token = tok
					atomic.AddInt64(&admittedCnt, 1)
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			if token == "" {
				return
			}
			// Try to purchase.
			if !firstSellOut.IsZero() {
				// Inventory already exhausted — count as wasted admission.
				atomic.AddInt64(&wastedCnt, 1)
				return
			}
			status, err := purchase(client, apiURL, u.id, token, 1)
			if err != nil {
				atomic.AddInt64(&errCnt, 1)
				return
			}
			switch status {
			case http.StatusCreated:
				atomic.AddInt64(&purchasedCnt, 1)
			case http.StatusConflict:
				atomic.AddInt64(&soldOutCnt, 1)
				// Sentinel: record the moment inventory first ran out.
				// Not strictly atomic but best-effort is fine for a single timestamp.
				if firstSellOut.IsZero() {
					firstSellOut = time.Now()
				}
				atomic.AddInt64(&wastedCnt, 1)
			default:
				atomic.AddInt64(&errCnt, 1)
			}
		}(joined[i])
	}
	wg2.Wait()
	close(stopSample)
	wg.Wait()
	totalDur := time.Since(startExp)

	timeToSellOutMs := int64(-1)
	if !firstSellOut.IsZero() {
		timeToSellOutMs = firstSellOut.Sub(startExp).Milliseconds()
	}

	finalRemaining, _ := getRemaining(apiURL)
	summary := map[string]any{
		"label":                label,
		"users":                users,
		"inventory":            inventory,
		"admitted":             atomic.LoadInt64(&admittedCnt),
		"purchased":            atomic.LoadInt64(&purchasedCnt),
		"sold_out_rejections":  atomic.LoadInt64(&soldOutCnt),
		"wasted_admissions":    atomic.LoadInt64(&wastedCnt),
		"errors":               atomic.LoadInt64(&errCnt),
		"time_to_sellout_ms":   timeToSellOutMs,
		"total_wallclock_ms":   totalDur.Milliseconds(),
		"final_remaining":      finalRemaining,
		"peak_sqs_depth":       atomic.LoadInt64(&peakSQS),
	}

	_ = writeJSON(fmt.Sprintf("%s/exp3_%s_u%d_summary.json", resultsDir, label, users), summary)
	_ = appendSweepRow(fmt.Sprintf("%s/exp3_sweep.csv", resultsDir), summary)
	prettyPrint(summary)
}

// ---- HTTP helpers ----

func join(c *http.Client, baseURL, userID string) (int64, string, error) {
	body, _ := json.Marshal(joinReq{UserID: userID})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/queue/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return 0, "", fmt.Errorf("status=%d", resp.StatusCode)
	}
	var jr joinResp
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return 0, "", err
	}
	return jr.Position, jr.Token, nil
}

func position(c *http.Client, baseURL, userID string) (string, bool, error) {
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/queue/position?user_id="+userID, nil)
	resp, err := c.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("status=%d", resp.StatusCode)
	}
	var pr positionResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", false, err
	}
	return pr.Token, pr.Admitted, nil
}

func purchase(c *http.Client, baseURL, customerID, token string, qty int) (int, error) {
	body, _ := json.Marshal(purchaseReq{CustomerID: customerID, Quantity: qty})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/purchase", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admission-Token", token)
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func post(url string, body []byte) error {
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func getRemaining(baseURL string) (int64, error) {
	resp, err := http.Get(baseURL + "/inventory")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var v struct {
		Remaining int64 `json:"remaining"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return 0, err
	}
	return v.Remaining, nil
}

func sampleSQS(cmd string) (int64, error) {
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// ---- CSV/JSON ----

func writeJSON(path string, v any) error {
	b, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

func appendSweepRow(path string, s map[string]any) error {
	cols := []string{
		"label", "users", "inventory",
		"admitted", "purchased", "sold_out_rejections", "wasted_admissions", "errors",
		"time_to_sellout_ms", "total_wallclock_ms", "final_remaining", "peak_sqs_depth",
	}
	needHeader := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		needHeader = true
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if needHeader {
		_ = w.Write(cols)
	}
	row := make([]string, len(cols))
	for i, c := range cols {
		row[i] = fmt.Sprintf("%v", s[c])
	}
	return w.Write(row)
}

func prettyPrint(s map[string]any) {
	order := []string{
		"label", "users", "inventory",
		"admitted", "purchased", "sold_out_rejections", "wasted_admissions", "errors",
		"time_to_sellout_ms", "total_wallclock_ms", "final_remaining", "peak_sqs_depth",
	}
	fmt.Println("----- Summary -----")
	for _, k := range order {
		fmt.Printf("  %-24s %v\n", k, s[k])
	}
	fmt.Println("-------------------")
}

// ---- misc ----

func getenv(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}
func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
