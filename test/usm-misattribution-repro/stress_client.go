// Stress test client for misattribution reproduction
// Creates rapid TLS connections to multiple servers to trigger pointer reuse
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	server1     = flag.String("server1", "localhost:8443", "First server address")
	server2     = flag.String("server2", "localhost:9443", "Second server address")
	duration    = flag.Duration("duration", 60*time.Second, "Test duration")
	concurrency = flag.Int("concurrency", 50, "Number of concurrent workers")
	skipClose   = flag.Float64("skip-close", 0.1, "Fraction of connections to skip Close() (0.0-1.0)")
	requestRate = flag.Duration("rate", 10*time.Millisecond, "Delay between requests per worker")
)

type stats struct {
	server1Requests int64
	server2Requests int64
	errors          int64
	skippedClose    int64
}

func main() {
	flag.Parse()

	log.Printf("Starting stress test:")
	log.Printf("  Server 1: %s", *server1)
	log.Printf("  Server 2: %s", *server2)
	log.Printf("  Duration: %s", *duration)
	log.Printf("  Concurrency: %d", *concurrency)
	log.Printf("  Skip Close rate: %.1f%%", *skipClose*100)

	// Create TLS config that skips verification (for self-signed certs)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var s stats
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker(ctx, workerID, tlsConfig, &s)
		}(i)
	}

	// Progress reporter
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Printf("Progress: server1=%d, server2=%d, errors=%d, skipped_close=%d",
					atomic.LoadInt64(&s.server1Requests),
					atomic.LoadInt64(&s.server2Requests),
					atomic.LoadInt64(&s.errors),
					atomic.LoadInt64(&s.skippedClose))
			}
		}
	}()

	wg.Wait()

	log.Printf("Test complete:")
	log.Printf("  Server 1 requests: %d", s.server1Requests)
	log.Printf("  Server 2 requests: %d", s.server2Requests)
	log.Printf("  Errors: %d", s.errors)
	log.Printf("  Skipped Close: %d", s.skippedClose)
}

func worker(ctx context.Context, id int, tlsConfig *tls.Config, s *stats) {
	servers := []string{*server1, *server2}
	paths := [][]string{
		{"/", "/health", "/server8443/identify", "/api/v1/data"},
		{"/", "/health", "/server9443/identify", "/api/v1/users"},
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Alternate between servers, with some randomness
		serverIdx := rand.Intn(2)
		server := servers[serverIdx]
		path := paths[serverIdx][rand.Intn(len(paths[serverIdx]))]

		// Create a NEW transport for each request to force new TLS connections
		// This maximizes memory churn and pointer reuse potential
		transport := &http.Transport{
			TLSClientConfig:     tlsConfig,
			DisableKeepAlives:   true, // Force new connection each time
			MaxIdleConns:        1,
			IdleConnTimeout:     1 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
		}

		client := &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		}

		url := fmt.Sprintf("https://%s%s", server, path)
		resp, err := client.Get(url)
		if err != nil {
			atomic.AddInt64(&s.errors, 1)
			// Don't close transport properly on error - simulates cleanup failure
			continue
		}

		// Read and discard body
		io.Copy(io.Discard, resp.Body)

		// Randomly skip Close() to simulate GC-dependent cleanup
		if rand.Float64() < *skipClose {
			atomic.AddInt64(&s.skippedClose, 1)
			// Don't close! Let GC handle it (or not)
			// This can leave stale entries in the eBPF maps
		} else {
			resp.Body.Close()
		}

		// Close the transport to release the TLS connection
		// But sometimes skip this too
		if rand.Float64() >= *skipClose {
			transport.CloseIdleConnections()
		}

		if serverIdx == 0 {
			atomic.AddInt64(&s.server1Requests, 1)
		} else {
			atomic.AddInt64(&s.server2Requests, 1)
		}

		// Small delay to control rate
		time.Sleep(*requestRate)

		// Occasionally force GC to trigger memory reuse
		if rand.Intn(100) < 5 {
			runtime.GC()
		}
	}
}
