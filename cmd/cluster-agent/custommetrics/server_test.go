// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package custommetrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// TestRetrySetup_SucceedsImmediately tests that the retrySetup function succeeds immediately.
func TestRetrySetup_SucceedsImmediately(t *testing.T) {
	backoff := wait.Backoff{Steps: 3, Duration: time.Millisecond, Factor: 2.0, Jitter: 0.0}
	calls := 0
	err := retrySetup(context.Background(), backoff, func(_ context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

// TestRetrySetup_SucceedsAfterTransientFailures tests that the retrySetup function succeeds after transient failures.
func TestRetrySetup_SucceedsAfterTransientFailures(t *testing.T) {
	backoff := wait.Backoff{Steps: 5, Duration: time.Millisecond, Factor: 2.0, Jitter: 0.0}
	transient := errors.New("apiserver unavailable")
	calls := 0
	err := retrySetup(context.Background(), backoff, func(_ context.Context) error {
		calls++
		if calls < 3 {
			return transient
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

// TestRetrySetup_ExhaustsRetries tests that the retrySetup function exhausts retries.
func TestRetrySetup_ExhaustsRetries(t *testing.T) {
	backoff := wait.Backoff{Steps: 3, Duration: time.Millisecond, Factor: 2.0, Jitter: 0.0}
	setupErr := errors.New("permanent-looking apiserver failure")
	calls := 0
	err := retrySetup(context.Background(), backoff, func(_ context.Context) error {
		calls++
		return setupErr
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
	if !errors.Is(err, setupErr) {
		t.Fatalf("expected wrapped setup error, got %v", err)
	}
}
