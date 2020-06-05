// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package http

import (
	"net/http"
	"sync"
	"time"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Client wraps (http.Client).Do and resets the underlying connections at the
// configured interval
type Client struct {
	httpClient    *http.Client
	timeout       time.Duration
	resetInterval time.Duration

	lastReset time.Time
	mutex     sync.RWMutex
}

// NewClient returns an initialized Client
func NewClient(timeout, resetInterval time.Duration) *Client {
	return &Client{
		httpClient:    newHTTPClient(timeout),
		timeout:       timeout,
		resetInterval: resetInterval,
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		// reusing core agent HTTP transport to benefit from proxy settings.
		Transport: httputils.CreateHTTPTransport(),
	}
}

// Do wraps (http.Client).Do. Thread safe.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.mutex.RLock()
	httpClient := c.httpClient
	c.mutex.RUnlock()
	if c.shouldReset() {
		log.Debug("Resetting client's connections")
		c.resetHTTPClient()
	}

	return httpClient.Do(req)
}

func (c *Client) shouldReset() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.resetInterval == 0 {
		return false
	}

	if c.lastReset.IsZero() {
		c.lastReset = time.Now()
	}

	if time.Since(c.lastReset) >= c.resetInterval {
		c.lastReset = time.Now()
		return true
	}

	return false
}

func (c *Client) resetHTTPClient() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// close idle connections. Thread-safe, but best effort
	// (if other goroutine(s) are currently using the client, the related open connection(s)
	// will remain open until the client is GC'ed)
	c.httpClient.CloseIdleConnections()
	c.httpClient = newHTTPClient(c.timeout)
}
