// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/cenkalti/backoff/v7"
)

// retry retries fn until it succeeds or opts exhaust it, bounding the retry loop to t's lifetime.
func retry(t testing.TB, fn func() error, opts ...backoff.RetryOption) error {
	t.Helper()
	_, err := backoff.Retry(t.Context(), func() (struct{}, error) {
		return struct{}{}, fn()
	}, opts...)
	return err
}
