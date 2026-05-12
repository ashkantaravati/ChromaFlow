package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:8080", "ChromaFlow base URL")
	targetURL := flag.String("target-url", "https://example.com", "URL to render")
	requests := flag.Int("requests", 20, "number of jobs to submit")
	concurrency := flag.Int("concurrency", 4, "concurrent submitters")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	jobs := make(chan int)
	var ok, failed atomic.Int64
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 15 * time.Second}
	start := time.Now()

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := range jobs {
				if err := submit(ctx, client, *baseURL, *targetURL, n); err != nil {
					failed.Add(1)
					fmt.Printf("submitter=%d request=%d error=%v\n", worker, n, err)
					continue
				}
				ok.Add(1)
			}
		}(i)
	}

	for i := 0; i < *requests; i++ {
		select {
		case <-ctx.Done():
			break
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("submitted=%d failed=%d duration=%s rate=%.2f/s\n", ok.Load(), failed.Load(), time.Since(start).Round(time.Millisecond), float64(ok.Load())/time.Since(start).Seconds())
	if failed.Load() > 0 {
		panic("load test had failed submissions")
	}
}

func submit(ctx context.Context, client *http.Client, baseURL, targetURL string, n int) error {
	body, _ := json.Marshal(map[string]string{"url": targetURL, "idempotency_key": fmt.Sprintf("load-%d-%d", time.Now().UnixNano(), n)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/pdf", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}
	return nil
}
