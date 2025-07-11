// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"net/http"
	"net/url"
	"time"
)

// Note: all of these are arbitrary defaults.

const (
	defaultMaxBatchItems     = 1024
	defaultMaxBatchSizeBytes = 1 * 1024 * 1024 // 1MB
	defaultMaxBufferDuration = 100 * time.Millisecond
)

type config struct {
	batcherConfig
	client *http.Client
	url    *url.URL
}

type batcherConfig struct {
	// The maximum number of items in a batch.
	maxBatchItems int
	// The maximum size of a batch in bytes in terms of the data of the items.
	maxBatchSizeBytes int
	// The maximum duration messages can sit in the buffer before being flushed.
	maxBufferDuration time.Duration
}

func defaultConfig() config {
	return config{
		client: http.DefaultClient,
		batcherConfig: batcherConfig{
			maxBatchItems:     defaultMaxBatchItems,
			maxBatchSizeBytes: defaultMaxBatchSizeBytes,
			maxBufferDuration: defaultMaxBufferDuration,
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
func WithURL(u *url.URL) Option {
	return func(c *config) {
		c.url = u
	}
}

// WithMaxBatchItems sets the maximum number of items in a batch.
func WithMaxBatchItems(maxItems int) Option {
	return func(c *config) {
		c.batcherConfig.maxBatchItems = maxItems
	}
}

// WithMaxBatchSizeBytes sets the maximum size of a batch in bytes.
func WithMaxBatchSizeBytes(maxSizeBytes int) Option {
	return func(c *config) {
		c.batcherConfig.maxBatchSizeBytes = maxSizeBytes
	}
}

// WithMaxBufferDuration sets the maximum buffer duration for the uploader.
func WithMaxBufferDuration(d time.Duration) Option {
	return func(c *config) {
		c.batcherConfig.maxBufferDuration = d
	}
}
