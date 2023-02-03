// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fakeintake

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type payload struct {
	timestamp time.Time
	data      []byte
}

type FakeIntake struct {
	mu     sync.RWMutex
	server http.Server

	payloadStore map[string][]payload
}

// NewFakeIntake creates a new fake intake and starts it on localhost:5000
func NewFakeIntake() *FakeIntake {
	fi := &FakeIntake{
		mu:           sync.RWMutex{},
		payloadStore: map[string][]payload{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", fi.postPayload)
	mux.HandleFunc("/fakeintake/payloads/", fi.getPayloads)

	fi.server = http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fi.start()

	return fi
}

func (fi *FakeIntake) start() {
	go func() {
		err := fi.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("Error creating fake intake server at %s: %v", fi.server.Addr, err)
		}
	}()
}

// Stop Gracefully stop the http server
func (fi *FakeIntake) Stop() error {
	return fi.server.Shutdown(context.Background())
}

type postPayloadResponse struct {
	Errors []string `json:"errors"`
}

func (fi *FakeIntake) postPayload(w http.ResponseWriter, req *http.Request) {
	fmt.Println("Handling PostPayload request")
	if req == nil {
		writePostPayloadResponse(w, []string{"invalid request, nil request"})
		return
	}

	if req.Method != http.MethodPost {
		writePostPayloadResponse(w, []string{fmt.Sprintf("invalid request with route %s and method %s", req.URL.Path, req.Method)})
		return
	}

	if req.Body == nil {
		writePostPayloadResponse(w, []string{"invalid request, nil body"})
		return
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		writePostPayloadResponse(w, []string{fmt.Sprintf("%v", err)})
		return
	}

	fi.safeAppendPayload(req.URL.Path, payload)
	writePostPayloadResponse(w, []string{})
}

func writePostPayloadResponse(w http.ResponseWriter, errors []string) {
	// build response
	resp := postPayloadResponse{
		Errors: errors,
	}
	jsonResp, err := json.Marshal(resp)

	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	statusCode := http.StatusAccepted
	if len(errors) > 0 {
		statusCode = http.StatusBadRequest
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(jsonResp)
}

type getPayloadResponse struct {
	Payloads [][]byte `json:"payloads"`
}

func (fi *FakeIntake) getPayloads(w http.ResponseWriter, req *http.Request) {
	fmt.Println("Handling GetPayload request")
	routes := req.URL.Query()["endpoint"]
	if len(routes) == 0 {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing endpoint query parameter"))
		return
	}
	// we could support multiple endpoints in the future
	route := routes[0]
	payloads := fi.safeGetPayloads(route)

	// build response
	resp := getPayloadResponse{
		Payloads: payloads,
	}
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// send response
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (fi *FakeIntake) safeAppendPayload(route string, data []byte) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	if _, found := fi.payloadStore[route]; !found {
		fi.payloadStore[route] = []payload{}
	}
	fi.payloadStore[route] = append(fi.payloadStore[route], payload{
		timestamp: time.Now(),
		data:      data,
	})
}

func (fi *FakeIntake) safeGetPayloads(route string) [][]byte {
	payloads := [][]byte{}
	fi.mu.Lock()
	defer fi.mu.Unlock()
	for _, p := range fi.payloadStore[route] {
		payloads = append(payloads, p.data)
	}
	return payloads
}
