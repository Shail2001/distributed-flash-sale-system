// Experiment 2 — Inventory correctness harness.
//
// Fires N concurrent POST /purchase requests against a seeded inventory of
// INVENTORY units. Collects accepted/rejected counts and per-request latency
// so we can detect oversell (accepted > inventory) under each strategy.
//
// Env:
//   API_BASE_URL       (default http://localhost:8081)
//   BUYERS             (default 200)
//   INVENTORY          (default 100)
//   STRATEGY_LABEL     (default "atomic")  -- used only for the output filename
//   RESULTS_DIR        (default ./tests/results)
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type purchaseReq struct {
	CustomerID string `json:"customer_id"`
	Quantity   int    `json:"quantity"`
}

type result struct {
	customer string
	status   int
	latency  time.Duration
	err      string
}

func main() {
	baseURL := getenv("API_BASE_URL", "http://localhost:8081")
	buyers := atoi(getenv("BUYERS", "200"))
	inventory := atoi(getenv("INVENTORY", "100"))
	label := getenv("STRATEGY_LABEL", "atomic")
	resultsDir := getenv("RESULTS_DIR", "./tests/results")

	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := resetInventory(baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "reset: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("==> Experiment 2 | strategy=%s buyers=%d inventory=%d\n", label, buyers, inventory)

	client := &http.Client{Timeout: 30 * time.Second}
	results := make([]result, buyers)

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			customer := fmt.Sprintf("exp2-%s-%d-%d", label, start.UnixNano(), i)
			t0 := time.Now()
			status, err := purchase(client, baseURL, customer, 1)
			results[i] = result{
				customer: customer,
				status:   status,
				latency:  time.Since(t0),
				err:      errString(err),
			}
		}(i)
	}
	wg.Wait()
	total := time.Since(start)

	var accepted, rejected, errors int
	lats := make([]time.Duration, 0, buyers)
	for _, r := range results {
		switch r.status {
		case http.StatusCreated:
			accepted++
		case http.StatusConflict:
			rejected++
		default:
			errors++
		}
		if r.status != 0 {
			lats = append(lats, r.latency)
		}
	}

	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	p := func(q float64) time.Duration {
		if len(lats) == 0 {
			return 0
		}
		return lats[int(float64(len(lats)-1)*q)]
	}

	// Verify final inventory; if accepted > inventory, we have oversell.
	finalRemaining, _ := getRemaining(baseURL)
	expectedRemaining := inventory - accepted
	oversell := accepted - inventory
	if oversell < 0 {
		oversell = 0
	}

	summary := map[string]any{
		"strategy":           label,
		"buyers":             buyers,
		"inventory":          inventory,
		"accepted":           accepted,
		"rejected":           rejected,
		"errors":             errors,
		"final_remaining":    finalRemaining,
		"expected_remaining": expectedRemaining,
		"oversell":           oversell,
		"total_wallclock_ms": total.Milliseconds(),
		"throughput_rps":     float64(buyers) / total.Seconds(),
		"latency_p50_ms":     p(0.50).Milliseconds(),
		"latency_p95_ms":     p(0.95).Milliseconds(),
		"latency_p99_ms":     p(0.99).Milliseconds(),
	}

	if err := writeJSON(fmt.Sprintf("%s/exp2_%s_b%d_summary.json", resultsDir, label, buyers), summary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := appendSweepRow(fmt.Sprintf("%s/exp2_sweep.csv", resultsDir), summary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	prettyPrint(summary)
}

func purchase(client *http.Client, baseURL, customer string, qty int) (int, error) {
	body, _ := json.Marshal(purchaseReq{CustomerID: customer, Quantity: qty})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/purchase", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func resetInventory(baseURL string) error {
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/reset", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reset status=%d", resp.StatusCode)
	}
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

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func appendSweepRow(path string, summary map[string]any) error {
	cols := []string{
		"strategy", "buyers", "inventory",
		"accepted", "rejected", "errors",
		"final_remaining", "expected_remaining", "oversell",
		"total_wallclock_ms", "throughput_rps",
		"latency_p50_ms", "latency_p95_ms", "latency_p99_ms",
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
		row[i] = fmt.Sprintf("%v", summary[c])
	}
	return w.Write(row)
}

func prettyPrint(s map[string]any) {
	order := []string{
		"strategy", "buyers", "inventory",
		"accepted", "rejected", "errors",
		"final_remaining", "expected_remaining", "oversell",
		"total_wallclock_ms", "throughput_rps",
		"latency_p50_ms", "latency_p95_ms", "latency_p99_ms",
	}
	fmt.Println("----- Summary -----")
	for _, k := range order {
		fmt.Printf("  %-22s %v\n", k, s[k])
	}
	fmt.Println("-------------------")
}

func getenv(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}
func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
