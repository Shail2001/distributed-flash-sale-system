// Experiment 1 — Waiting Room fairness harness.
//
// Fires N concurrent POST /queue/join requests with zero spawn delay, collects
// assigned positions and per-request latency, and writes a CSV summary.
//
// Env:
//   WAITING_ROOM_URL  (default http://localhost:8080)
//   USERS             (default 100)
//   STRATEGY_LABEL    (default "timestamp")   -- for the output filename
//   RESULTS_DIR       (default ./tests/results)
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

type joinReq struct {
	UserID string `json:"user_id"`
}

type joinResp struct {
	UserID   string `json:"user_id"`
	Position int64  `json:"position"`
	Strategy string `json:"strategy"`
}

type result struct {
	userID   string
	position int64
	latency  time.Duration
	status   int
	err      string
}

func main() {
	baseURL := getenv("WAITING_ROOM_URL", "http://localhost:8080")
	users := atoi(getenv("USERS", "100"))
	strategyLabel := getenv("STRATEGY_LABEL", "timestamp")
	resultsDir := getenv("RESULTS_DIR", "./tests/results")

	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", resultsDir, err)
		os.Exit(1)
	}

	// Reset queue state before the run.
	if err := resetQueue(baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "reset queue: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("==> Experiment 1 | strategy=%s users=%d\n", strategyLabel, users)

	// Pre-build the HTTP client shared across goroutines.
	client := &http.Client{Timeout: 30 * time.Second}
	results := make([]result, users)

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			userID := fmt.Sprintf("exp1-user-%d-%d", start.UnixNano(), i)
			t0 := time.Now()
			pos, status, err := join(client, baseURL, userID)
			results[i] = result{
				userID:   userID,
				position: pos,
				latency:  time.Since(t0),
				status:   status,
				err:      errString(err),
			}
		}(i)
	}
	wg.Wait()
	total := time.Since(start)

	// ---- Aggregate ----
	var accepted int
	latencies := make([]time.Duration, 0, users)
	positionCount := map[int64]int{}
	var maxPos int64
	for _, r := range results {
		if r.status == http.StatusCreated {
			accepted++
			latencies = append(latencies, r.latency)
			positionCount[r.position]++
			if r.position > maxPos {
				maxPos = r.position
			}
		}
	}

	// Collisions: users who received a position already assigned to someone else.
	collisions := 0
	for _, c := range positionCount {
		if c > 1 {
			collisions += c
		}
	}

	// Gaps: missing positions in [1..maxPos].
	gaps := 0
	if maxPos > 0 {
		for p := int64(1); p <= maxPos; p++ {
			if _, ok := positionCount[p]; !ok {
				gaps++
			}
		}
	}

	// Latency percentiles.
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p := func(q float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * q)
		return latencies[idx]
	}

	summary := map[string]any{
		"strategy":           strategyLabel,
		"users":              users,
		"accepted":           accepted,
		"unique_positions":   len(positionCount),
		"max_position":       maxPos,
		"collisions":         collisions,
		"collision_rate_pct": pct(collisions, accepted),
		"gaps":               gaps,
		"gap_rate_pct":       pct(gaps, int(maxPos)),
		"total_wallclock_ms": total.Milliseconds(),
		"throughput_rps":     float64(accepted) / total.Seconds(),
		"latency_p50_ms":     p(0.50).Milliseconds(),
		"latency_p95_ms":     p(0.95).Milliseconds(),
		"latency_p99_ms":     p(0.99).Milliseconds(),
		"latency_max_ms":     p(1.0).Milliseconds(),
	}

	// ---- Write per-user CSV ----
	perUserPath := fmt.Sprintf("%s/exp1_%s_n%d_positions.csv", resultsDir, strategyLabel, users)
	if err := writePerUserCSV(perUserPath, results); err != nil {
		fmt.Fprintf(os.Stderr, "write per-user csv: %v\n", err)
		os.Exit(1)
	}

	// ---- Write summary JSON ----
	summaryPath := fmt.Sprintf("%s/exp1_%s_n%d_summary.json", resultsDir, strategyLabel, users)
	if err := writeJSON(summaryPath, summary); err != nil {
		fmt.Fprintf(os.Stderr, "write summary: %v\n", err)
		os.Exit(1)
	}

	// ---- Append to cross-run CSV ----
	sweepPath := fmt.Sprintf("%s/exp1_sweep.csv", resultsDir)
	if err := appendSweepRow(sweepPath, summary); err != nil {
		fmt.Fprintf(os.Stderr, "append sweep: %v\n", err)
		os.Exit(1)
	}

	prettyPrint(summary)
}

func join(client *http.Client, baseURL, userID string) (int64, int, error) {
	body, _ := json.Marshal(joinReq{UserID: userID})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/queue/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return 0, resp.StatusCode, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var jr joinResp
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return 0, resp.StatusCode, err
	}
	return jr.Position, resp.StatusCode, nil
}

func resetQueue(baseURL string) error {
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/queue/reset", nil)
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

func writePerUserCSV(path string, rows []result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"user_id", "position", "latency_ms", "status", "error"})
	for _, r := range rows {
		_ = w.Write([]string{
			r.userID,
			strconv.FormatInt(r.position, 10),
			strconv.FormatInt(r.latency.Milliseconds(), 10),
			strconv.Itoa(r.status),
			r.err,
		})
	}
	return w.Error()
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
		"strategy", "users", "accepted", "unique_positions", "max_position",
		"collisions", "collision_rate_pct", "gaps", "gap_rate_pct",
		"total_wallclock_ms", "throughput_rps",
		"latency_p50_ms", "latency_p95_ms", "latency_p99_ms", "latency_max_ms",
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

func prettyPrint(summary map[string]any) {
	fmt.Println("----- Summary -----")
	order := []string{
		"strategy", "users", "accepted", "unique_positions", "max_position",
		"collisions", "collision_rate_pct", "gaps", "gap_rate_pct",
		"total_wallclock_ms", "throughput_rps",
		"latency_p50_ms", "latency_p95_ms", "latency_p99_ms", "latency_max_ms",
	}
	for _, k := range order {
		fmt.Printf("  %-22s %v\n", k, summary[k])
	}
	fmt.Println("-------------------")
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func pct(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return 100 * float64(num) / float64(denom)
}
