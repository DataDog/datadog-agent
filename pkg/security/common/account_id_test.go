// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&buf, log.DebugLvl)
	require.NoError(t, err)
	prev := log.Default()
	t.Cleanup(func() { log.SetupLogger(prev, "debug") })
	log.SetupLogger(logger, "debug")
	return &buf
}

func TestQueryAccountIDTagSuccess(t *testing.T) {
	logs := captureLogs(t)

	var detectCalls, accountIDCalls int
	detectors := []cloudProviderDetector{
		{
			name:          "AWS",
			accountIDName: "account_id",
			callback: func(context.Context) bool {
				return false
			},
			accountIDCallback: func(context.Context) (string, error) {
				t.Fatal("unexpected call to non-matching detector accountIDCallback")
				return "", nil
			},
		},
		{
			name:          "GCP",
			accountIDName: "project_id",
			callback: func(context.Context) bool {
				detectCalls++
				return true
			},
			accountIDCallback: func(context.Context) (string, error) {
				accountIDCalls++
				return "my-gcp-project", nil
			},
		},
	}

	resolver := newAccountIDTagResolver(detectors, maxAccountIDAttempts, accountIDRetryBackoff)
	assert.Equal(t, "project_id:my-gcp-project", resolver.queryAccountIDTag())
	assert.Equal(t, "project_id:my-gcp-project", resolver.queryAccountIDTag())
	assert.Equal(t, 1, detectCalls)
	assert.Equal(t, 1, accountIDCalls)
	assert.NotContains(t, logs.String(), "[ERROR]")
	assert.NotContains(t, logs.String(), "[WARN]")
}

func TestQueryAccountIDTagNoCloudProvider(t *testing.T) {
	logs := captureLogs(t)

	synctest.Test(t, func(t *testing.T) {
		var detectCalls int
		detectors := []cloudProviderDetector{
			{
				name:          "AWS",
				accountIDName: "account_id",
				callback: func(context.Context) bool {
					detectCalls++
					return false
				},
				accountIDCallback: func(context.Context) (string, error) {
					t.Fatal("unexpected call to accountIDCallback when provider is not detected")
					return "", nil
				},
			},
		}

		resolver := newAccountIDTagResolver(detectors, maxAccountIDAttempts, accountIDRetryBackoff)
		assert.Empty(t, resolver.queryAccountIDTag())

		time.Sleep(accountIDRetryBackoff)
		assert.Empty(t, resolver.queryAccountIDTag())
		assert.Equal(t, 1, detectCalls)
	})

	output := logs.String()
	assert.Contains(t, output, "[DEBUG] failed to query account id: no cloud provider detected")
	assert.NotContains(t, output, "[ERROR]")
	assert.NotContains(t, output, "[WARN]")
}

func TestQueryAccountIDTagTransientFailureThenSuccess(t *testing.T) {
	logs := captureLogs(t)

	synctest.Test(t, func(t *testing.T) {
		var accountIDCalls atomic.Int32
		detectors := []cloudProviderDetector{
			{
				name:          "AWS",
				accountIDName: "account_id",
				callback: func(context.Context) bool {
					return true
				},
				accountIDCallback: func(context.Context) (string, error) {
					if accountIDCalls.Add(1) < 3 {
						return "", errors.New("metadata timeout")
					}
					return "123456789012", nil
				},
			},
		}

		resolver := newAccountIDTagResolver(detectors, maxAccountIDAttempts, accountIDRetryBackoff)

		assert.Empty(t, resolver.queryAccountIDTag())
		assert.Equal(t, int32(1), accountIDCalls.Load())

		resolver.waitForInFlight()
		assert.Equal(t, int32(3), accountIDCalls.Load())
		assert.Equal(t, "account_id:123456789012", resolver.queryAccountIDTag())

		time.Sleep(accountIDRetryBackoff)
		assert.Equal(t, "account_id:123456789012", resolver.queryAccountIDTag())
		assert.Equal(t, int32(3), accountIDCalls.Load())
	})

	output := logs.String()
	assert.Contains(t, output, "[DEBUG] failed to query account id (attempt 1/3): could not detect cloud provider account ID: metadata timeout")
	assert.Contains(t, output, "[DEBUG] failed to query account id (attempt 2/3): could not detect cloud provider account ID: metadata timeout")
	assert.NotContains(t, output, "[ERROR]")
	assert.NotContains(t, output, "[WARN]")
}

func TestQueryAccountIDTagRetryExhaustion(t *testing.T) {
	logs := captureLogs(t)

	synctest.Test(t, func(t *testing.T) {
		var accountIDCalls atomic.Int32
		detectors := []cloudProviderDetector{
			{
				name:          "Azure",
				accountIDName: "subscription_id",
				callback: func(context.Context) bool {
					return true
				},
				accountIDCallback: func(context.Context) (string, error) {
					accountIDCalls.Add(1)
					return "", errors.New("HTTP 503")
				},
			},
		}

		resolver := newAccountIDTagResolver(detectors, maxAccountIDAttempts, accountIDRetryBackoff)

		assert.Empty(t, resolver.queryAccountIDTag())
		assert.Equal(t, int32(1), accountIDCalls.Load())

		resolver.waitForInFlight()
		assert.Equal(t, int32(3), accountIDCalls.Load())
		assert.Empty(t, resolver.queryAccountIDTag())

		time.Sleep(accountIDRetryBackoff)
		assert.Empty(t, resolver.queryAccountIDTag())
		assert.Equal(t, int32(3), accountIDCalls.Load())
	})

	output := logs.String()
	assert.Contains(t, output, "[DEBUG] failed to query account id (attempt 1/3): could not detect cloud provider account ID: HTTP 503")
	assert.Contains(t, output, "[DEBUG] failed to query account id (attempt 2/3): could not detect cloud provider account ID: HTTP 503")
	assert.Contains(t, output, "[WARN] failed to query account id after 3 attempts: could not detect cloud provider account ID: HTTP 503")
	assert.NotContains(t, output, "[ERROR]")
}

func TestQueryAccountIDTagAsyncRetryAndInFlightNonBlocking(t *testing.T) {
	_ = captureLogs(t)

	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		initialEntered := make(chan struct{})
		releaseInitial := make(chan struct{})
		retryEntered := make(chan struct{})
		releaseRetry := make(chan struct{})

		detectors := []cloudProviderDetector{
			{
				name:          "AWS",
				accountIDName: "account_id",
				callback: func(context.Context) bool {
					return true
				},
				accountIDCallback: func(context.Context) (string, error) {
					callNum := calls.Add(1)
					if callNum == 1 {
						close(initialEntered)
						<-releaseInitial
						return "", errors.New("transient error")
					}
					close(retryEntered)
					<-releaseRetry
					return "123456789012", nil
				},
			},
		}

		resolver := newAccountIDTagResolver(detectors, maxAccountIDAttempts, accountIDRetryBackoff)

		initialDone := make(chan string, 1)
		concurrentInitialDone := make(chan string, 1)
		go func() {
			initialDone <- resolver.queryAccountIDTag()
		}()

		<-initialEntered
		go func() {
			concurrentInitialDone <- resolver.queryAccountIDTag()
		}()
		assert.Equal(t, int32(1), calls.Load())

		close(releaseInitial)
		assert.Empty(t, <-initialDone)
		assert.Empty(t, <-concurrentInitialDone)

		<-retryEntered
		assert.Empty(t, resolver.queryAccountIDTag())
		assert.Equal(t, int32(2), calls.Load())

		close(releaseRetry)
		resolver.waitForInFlight()

		assert.Equal(t, "account_id:123456789012", resolver.queryAccountIDTag())
		assert.Equal(t, int32(2), calls.Load())
	})
}
