// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

// Package testutil implements a fake ECS client to be used in tests.
package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"
)

// DummyECS allows tests to mock ECS metadata server responses
type DummyECS struct {
	mux               *http.ServeMux
	fileHandlers      map[string]string
	fileHandlersDelay map[string]time.Duration
	rawHandlers       map[string]string
	rawHandlersDelay  map[string]time.Duration
	Requests          chan *http.Request
	RequestCount      atomic.Uint64
}

// Option represents an option used to create a new mock of the ECS metadata
// server.
type Option func(*DummyECS)

// FileHandlerOption allows returning the content of a file to requests matching
// a pattern.
func FileHandlerOption(pattern, testDataFile string) Option {
	return func(d *DummyECS) {
		d.fileHandlers[pattern] = testDataFile
	}
}

// FileHandlerDelayOption allows returning to requests matching a pattern with a delay.
func FileHandlerDelayOption(pattern string, delay time.Duration) Option {
	return func(d *DummyECS) {
		d.fileHandlersDelay[pattern] = delay
	}
}

// RawHandlerOption allows returning the specified string to requests matching a
// pattern.
func RawHandlerOption(pattern, rawResponse string) Option {
	return func(d *DummyECS) {
		d.rawHandlers[pattern] = rawResponse
	}
}

// RawHandlerDelayOption allows returning to requests matching a pattern with a delay.
func RawHandlerDelayOption(pattern string, delay time.Duration) Option {
	return func(d *DummyECS) {
		d.rawHandlersDelay[pattern] = delay
	}
}

// NewDummyECS create a mock of the ECS metadata API.
func NewDummyECS(ops ...Option) (*DummyECS, error) {
	d := &DummyECS{
		mux:               http.NewServeMux(),
		fileHandlers:      make(map[string]string),
		fileHandlersDelay: make(map[string]time.Duration),
		rawHandlers:       make(map[string]string),
		rawHandlersDelay:  make(map[string]time.Duration),
		Requests:          make(chan *http.Request, 10),
	}
	for _, o := range ops {
		o(d)
	}
	for pattern, testDataPath := range d.fileHandlers {
		raw, err := os.ReadFile(testDataPath)
		if err != nil {
			return nil, fmt.Errorf("failed to register handler for pattern %s: could not read test data file with path %s", pattern, testDataPath)
		}
		d.mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			if delay, ok := d.fileHandlersDelay[pattern]; ok {
				time.Sleep(delay)
			}
			w.Write(raw)
		})
	}
	for pattern, rawData := range d.rawHandlers {
		d.mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			if delay, ok := d.rawHandlersDelay[pattern]; ok {
				time.Sleep(delay)
			}
			w.Write([]byte(rawData))
		})
	}
	return d, nil
}

// ServeHTTP is used to handle HTTP requests.
func (d *DummyECS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("dummyECS received %s on %s\n", r.Method, r.URL.Path)
	d.RequestCount.Add(1)
	d.Requests <- r
	d.mux.ServeHTTP(w, r)
}

// Start starts the HTTP server
func (d *DummyECS) Start() *httptest.Server {
	return httptest.NewServer(d)
}
