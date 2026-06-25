package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/http2"
)

var (
	targetURL = flag.String("url", "https://127.0.0.1:8443", "Target HTTPS server URL")
	interval  = flag.Duration("interval", 2*time.Second, "Interval between requests")
	count     = flag.Int("count", 0, "Number of requests (0 = infinite)")
)

func main() {
	flag.Parse()

	log.Printf("Starting TLS client (PID: %d)", os.Getpid())
	log.Printf("Target: %s", *targetURL)
	log.Printf("Interval: %v", *interval)
	if *count == 0 {
		log.Printf("Mode: Infinite requests (press Ctrl+C to stop)")
	} else {
		log.Printf("Mode: %d requests", *count)
	}

	// Create HTTP client with TLS config that accepts self-signed certs
	// Force HTTP/2 transport
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Accept self-signed certificates
		},
		AllowHTTP: false, // Only HTTPS
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	log.Printf("Using HTTP/2 transport")

	// List of k8s API endpoints to simulate kafka-admin-daemon behavior
	endpoints := []string{
		"/api/v1/persistentvolumes/pvc-63958627-daf8-4858-bf79-3818733c87b4",
		"/api/v1/persistentvolumes/pvc-33641375-8db3-4ff3-b172-2ba76ce132fe",
		"/api/v1/persistentvolumes/pvc-12345678-1234-1234-1234-123456789abc",
		"/api/v1/namespaces/orgstore-maze/configmaps",
		"/api/v1/namespaces/postgres-orgstore-proposals/configmaps",
		"/api/v1/namespaces/datadog-agent/configmaps",
		"/api/v1/namespaces/zookeeper-lockness/configmaps",
		"/api/v1/namespaces/kube-system/pods",
	}

	requestCount := 0
	successCount := 0
	errorCount := 0

	for {
		if *count > 0 && requestCount >= *count {
			break
		}

		// Rotate through endpoints
		endpoint := endpoints[requestCount%len(endpoints)]
		fullURL := *targetURL + endpoint

		// Make GET request
		log.Printf("[Request #%d] GET %s", requestCount+1, endpoint)
		start := time.Now()
		resp, err := client.Get(fullURL)
		elapsed := time.Since(start)

		if err != nil {
			log.Printf("  ✗ ERROR: %v (elapsed: %v)", err, elapsed)
			errorCount++
		} else {
			// Read response body
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			log.Printf("  ✓ Status: %d, Size: %d bytes (elapsed: %v)",
				resp.StatusCode, len(body), elapsed)
			successCount++

			if resp.StatusCode != http.StatusOK {
				log.Printf("  ⚠ Unexpected status code: %d", resp.StatusCode)
			}
		}

		requestCount++

		// Print summary every 10 requests
		if requestCount%10 == 0 {
			log.Printf("--- Summary after %d requests: %d success, %d errors ---",
				requestCount, successCount, errorCount)
		}

		// Wait before next request
		if *count == 0 || requestCount < *count {
			time.Sleep(*interval)
		}
	}

	// Final summary
	log.Printf("=== Final Summary ===")
	log.Printf("Total requests: %d", requestCount)
	log.Printf("Successful: %d", successCount)
	log.Printf("Errors: %d", errorCount)
	log.Printf("Success rate: %.2f%%", float64(successCount)/float64(requestCount)*100)
}
