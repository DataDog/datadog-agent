// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package utils

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithConcurrencyLimit(t *testing.T) {
	const concurrentRequests = 5

	var (
		recorders []*httptest.ResponseRecorder
		wg        sync.WaitGroup
		wait      = make(chan struct{})
	)

	handler := WithConcurrencyLimit(concurrentRequests, func(w http.ResponseWriter, r *http.Request) {
		<-wait
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		request := httptest.NewRequest("GET", "http://example.com", nil)
		responseRecorder := httptest.NewRecorder()
		recorders = append(recorders, responseRecorder)

		go func(w http.ResponseWriter, r *http.Request) {
			handler(w, r)
			wg.Done()
		}(responseRecorder, request)
	}

	// Time to ensure that all requests are busy being processed
	time.Sleep(100 * time.Millisecond)

	// The requests above should all hang and the next ones should return
	// immediately with a `StatusTooManyRequests` as they're being limitted
	for i := 0; i < concurrentRequests; i++ {
		r := httptest.NewRequest("GET", "http://example.com", nil)
		w := httptest.NewRecorder()
		handler(w, r)
		resp := w.Result()
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	}

	// Unlock the in-flight requests
	close(wait)
	wg.Wait()

	// Verify that they were processed by the original handler
	for _, recorder := range recorders {
		resp := recorder.Result()
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}
