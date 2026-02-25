package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// Simple program that makes HTTP/2 requests to k8s API using Go's crypto/tls
// This will be captured by USM's Go TLS uprobes

func main() {
	count := flag.Int("count", 50, "number of requests to make")
	url := flag.String("url", "https://172.17.0.1:443/api/v1/persistentvolumes", "k8s API URL")
	interval := flag.Duration("interval", 100*time.Millisecond, "interval between requests")
	flag.Parse()

	// Create HTTP/2 client with Go's crypto/tls (this is what USM captures!)
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // k8s API uses self-signed cert
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	fmt.Printf("Making %d HTTP/2 requests to: %s\n", *count, *url)
	fmt.Printf("Using Go's crypto/tls (will be captured by USM Go TLS uprobes)\n")
	fmt.Printf("Started at: %s\n\n", time.Now().Format("15:04:05"))

	successCount := 0
	errorCount := 0

	for i := 1; i <= *count; i++ {
		startTime := time.Now()

		req, err := http.NewRequest("GET", *url, nil)
		if err != nil {
			fmt.Printf("[%d] Error creating request: %v\n", i, err)
			errorCount++
			continue
		}

		resp, err := client.Do(req)
		duration := time.Since(startTime)

		if err != nil {
			fmt.Printf("[%d] Error: %v (%.3fs)\n", i, err, duration.Seconds())
			errorCount++
		} else {
			// Read and discard body
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			fmt.Printf("[%d] HTTP %d - %.3fs - Proto: %s\n",
				i, resp.StatusCode, duration.Seconds(), resp.Proto)

			if resp.StatusCode < 500 {
				successCount++
			} else {
				errorCount++
			}
		}

		if i < *count {
			time.Sleep(*interval)
		}
	}

	fmt.Printf("\nFinished at: %s\n", time.Now().Format("15:04:05"))
	fmt.Printf("Results: %d successful, %d errors\n", successCount, errorCount)
	fmt.Printf("\nThese requests should now appear in USM metrics!\n")
	fmt.Printf("Check universal.http.server.hits or universal.http.client.hits\n")
	fmt.Printf("with resource_name:*persistentvolumes* to see which service they're attributed to.\n")
}
