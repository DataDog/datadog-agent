// Write a loadtest for sending metric payloads to the cluster agent endpoint: http://localhost:5050/api/v2/series

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
)

const (
	authorizationHeaderKey = "Authorization"
	authToken              = "cc066c496df1406dfb81a6b667e0c444d3b89885439dcbfcee2a5c3227c3abaf"
)

func createSeriesPayload(i int) *gogen.MetricPayload {
	container_id := fmt.Sprintf("container_id:%d", i)
	display_container_name := fmt.Sprintf("display_container_name:%d", i)
	namespace := fmt.Sprintf("kube_namespace:test")
	payload := gogen.MetricPayload{
		Series: []*gogen.MetricPayload_MetricSeries{
			{
				Metric: "datadog.test.run",
				Type:   3, // Gauge
				Points: []*gogen.MetricPayload_MetricPoint{
					{
						Timestamp: time.Now().Unix(),
						Value:     1.0,
					},
				},
				Tags: []string{container_id, display_container_name, namespace},
				Resources: []*gogen.MetricPayload_Resource{
					{
						Type: "host", Name: "localHost",
					},
				},
			},
		},
	}
	return &payload
}

func worker(_ context.Context, numRequests int) {
	// Create a client
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
				KeepAlive: 20 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout:   5 * time.Second,
			MaxConnsPerHost:       1,
			MaxIdleConnsPerHost:   1,
			IdleConnTimeout:       60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	// Send requests
	for i := 0; i < numRequests; i++ {
		// Create a gzip writer
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)

		// Write the payload to the gzip writer
		payload, _ := createSeriesPayload(i).Marshal()
		_, err := gz.Write(payload)
		if err != nil {
			fmt.Println("Error writing payload to gzip writer:", err)
			continue
		}

		// Close the gzip writer
		err = gz.Close()
		if err != nil {
			fmt.Println("Error closing gzip writer:", err)
			continue
		}

		// Create a request
		req, err := http.NewRequest("POST", "https://localhost:5005/api/v2/series", &buf)
		if err != nil {
			fmt.Println("Error creating request:", err)
			continue
		}
		req.Header.Set("Content-Encoding", "gzip")
		keyStr := fmt.Sprintf("Bearer %s", authToken)
		req.Header.Set(authorizationHeaderKey, keyStr)

		// Send the request
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			continue
		}

		// Check the response
		if resp.StatusCode != http.StatusOK {
			fmt.Println("Unexpected response status code:", resp.StatusCode)
			continue
		} else {
			//msg := fmt.Sprintf("Response status code: %d, request id = %d", resp.StatusCode, i)
			//fmt.Println(msg)
		}

		// Close the response body
		_, err = io.Copy(io.Discard, resp.Body)
		if err != nil {
			fmt.Println("Error reading response body:", err)
			continue
		}
		resp.Body.Close()
	}
}

func main() {
	// Number of requests to send
	numRequests := 100000

	// Create a context
	ctx, _ := context.WithCancel(context.Background())

	// Start the workers
	worker(ctx, numRequests)

}
