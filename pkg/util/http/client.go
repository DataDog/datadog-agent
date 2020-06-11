// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package http

import (
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	apiKeyReplacement = "api_key=*************************$1"
)

var apiKeyRegExp = regexp.MustCompile("api_key=*\\w+(\\w{5})")

// SanitizeURL sanitizes credentials from a message containing a URL, and returns
// a string that can be logged safely.
// For now, it obfuscates the API key.
func SanitizeURL(message string) string {
	return apiKeyRegExp.ReplaceAllString(message, apiKeyReplacement)
}

// Client wraps (http.Client).Do and resets the underlying connections at the
// configured interval
type Client struct {
	httpClient    *http.Client
	newHTTPClient func() *http.Client
	resetInterval time.Duration

	lastReset time.Time
	mutex     sync.RWMutex
}

// NewClient returns an initialized Client resetting connections at the passed resetInterval.
// The underlying http.Client used will be created using the passed http client factory.
func NewClient(resetInterval time.Duration, newHTTPClient func() *http.Client) *Client {
	return &Client{
		resetInterval: resetInterval,
		httpClient:    newHTTPClient(),
		newHTTPClient: newHTTPClient,
	}
}

// Do wraps (http.Client).Do. Thread safe.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.mutex.RLock()
	httpClient := c.httpClient
	c.mutex.RUnlock()
	if c.shouldReset() {
		log.Debug("Resetting HTTP client's connections")
		c.resetHTTPClient()
	}

	return httpClient.Do(req)
}

// shouldReset returns whether the http.Client should be reset
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

// resetHTTPClient resets the underlying *http.Client
func (c *Client) resetHTTPClient() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// close idle connections. Thread-safe.
	// This is a best effort: if other goroutine(s) are currently using the client,
	// the related open connection(s) will remain open until the client is GC'ed)
	c.httpClient.CloseIdleConnections()
	c.httpClient = c.newHTTPClient()
}
