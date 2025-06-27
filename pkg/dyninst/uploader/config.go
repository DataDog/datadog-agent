// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"net/http"
	"time"
)

// Note: all of these are arbitrary defaults.

const (
	defaultMaxItems      = 1024
	defaultMaxSizeBytes  = 1 * 1024 * 1024 // 1MB
	defaultIdleTimeout   = 250 * time.Millisecond
	defaultMaxBufferTime = 2 * time.Second
)

type config struct {
	batcherConfig
	client *http.Client
	url    string
}

type batcherConfig struct {
	maxItems          int
	maxSizeBytes      int
	idleFlushDuration time.Duration
	maxBufferDuration time.Duration
}

func defaultConfig() *config {
	return &config{
		client: http.DefaultClient,
		batcherConfig: batcherConfig{
			maxItems:          defaultMaxItems,
			maxSizeBytes:      defaultMaxSizeBytes,
			idleFlushDuration: defaultIdleTimeout,
			maxBufferDuration: defaultMaxBufferTime,
		},
	}
}

// Option is a functional option for configuring an uploader.
type Option func(*config)

// WithClient sets the http client for the uploader.
func WithClient(client *http.Client) Option {
	return func(c *config) {
		c.client = client
	}
}

// WithURL sets the URL for the uploader.
func WithURL(url string) Option {
	return func(c *config) {
		c.url = url
	}
}

// WithMaxItems sets the maximum number of items in a batch.
func WithMaxItems(maxItems int) Option {
	return func(c *config) {
		c.maxItems = maxItems
	}
}

// WithMaxSizeBytes sets the maximum size of a batch in bytes.
func WithMaxSizeBytes(maxSizeBytes int) Option {
	return func(c *config) {
		c.maxSizeBytes = maxSizeBytes
	}
}

// WithIdleFlushDuration sets the idle flush duration for the uploader.
func WithIdleFlushDuration(d time.Duration) Option {
	return func(c *config) {
		c.idleFlushDuration = d
	}
}

// WithMaxBufferDuration sets the maximum buffer duration for the uploader.
func WithMaxBufferDuration(d time.Duration) Option {
	return func(c *config) {
		c.maxBufferDuration = d
	}
}
