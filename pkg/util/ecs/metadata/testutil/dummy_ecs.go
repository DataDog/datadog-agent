// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
)

// DummyECS allows tests to mock ECS metadata server responses
type DummyECS struct {
	mux          *http.ServeMux
	fileHandlers map[string]string
	rawHandlers  map[string]string
	Requests     chan *http.Request
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

// RawHandlerOption allows returning the specified string to requests matching a
// pattern.
func RawHandlerOption(pattern, rawResponse string) Option {
	return func(d *DummyECS) {
		d.rawHandlers[pattern] = rawResponse
	}
}

// NewDummyECS create a mock of the ECS metadata API.
func NewDummyECS(ops ...Option) (*DummyECS, error) {
	d := &DummyECS{
		mux:          http.NewServeMux(),
		fileHandlers: make(map[string]string),
		rawHandlers:  make(map[string]string),
		Requests:     make(chan *http.Request, 3),
	}
	for _, o := range ops {
		o(d)
	}
	for pattern, testDataPath := range d.fileHandlers {
		raw, err := os.ReadFile(testDataPath)
		if err != nil {
			return nil, fmt.Errorf("failed to register handler for pattern %s: could not read test data file with path %s", pattern, testDataPath)
		}
		d.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Write(raw)
		})
	}
	for pattern, rawData := range d.rawHandlers {
		d.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(rawData))
		})
	}
	return d, nil
}

// ServeHTTP is used to handle HTTP requests.
func (d *DummyECS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("dummyECS received %s on %s\n", r.Method, r.URL.Path)
	d.Requests <- r
	d.mux.ServeHTTP(w, r)
}

// Start starts the HTTP server
func (d *DummyECS) Start() *httptest.Server {
	return httptest.NewServer(d)
}
