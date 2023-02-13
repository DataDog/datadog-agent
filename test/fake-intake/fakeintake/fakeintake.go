// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fakeintake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
func NewFakeIntake(port int) *FakeIntake {
	fi := &FakeIntake{
		mu:           sync.RWMutex{},
		payloadStore: map[string][]payload{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", fi.handleDatadogRequest)
	mux.HandleFunc("/fakeintake/payloads/", fi.getPayloads)

	fi.server = http.Server{
		Addr:    fmt.Sprintf(":%d", port),
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

func (fi *FakeIntake) handleDatadogRequest(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		response := buildPostResponse(errors.New("invalid request, nil request"))
		writeHttpResponse(w, response)
		return
	}

	log.Printf("Handling Datadog %s request to %s", req.Method, req.URL.Path)

	if req.Method == http.MethodGet {
		writeHttpResponse(w, httpResponse{
			statusCode: http.StatusOK,
		})
		return
	}

	// from now on accept only POST requests
	if req.Method != http.MethodPost {
		response := buildPostResponse(fmt.Errorf("invalid request with route %s and method %s", req.URL.Path, req.Method))
		writeHttpResponse(w, response)
		return
	}

	if req.Body == nil {
		response := buildPostResponse(errors.New("invalid request, nil body"))
		writeHttpResponse(w, response)
		return
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		response := buildPostResponse(err)
		writeHttpResponse(w, response)
		return
	}

	fi.safeAppendPayload(req.URL.Path, payload)
	response := buildPostResponse(nil)
	writeHttpResponse(w, response)
}

func buildPostResponse(responseError error) httpResponse {
	ret := httpResponse{}

	ret.contentType = "application/json"
	ret.statusCode = http.StatusAccepted

	resp := postPayloadResponse{}
	if responseError != nil {
		ret.statusCode = http.StatusBadRequest
		resp.Errors = []string{responseError.Error()}
	}
	body, err := json.Marshal(resp)

	if err != nil {
		return httpResponse{
			statusCode:  http.StatusInternalServerError,
			contentType: "text/plain",
			body:        []byte(err.Error()),
		}
	}

	ret.body = body

	return ret
}

type getPayloadResponse struct {
	Payloads [][]byte `json:"payloads"`
}

func (fi *FakeIntake) getPayloads(w http.ResponseWriter, req *http.Request) {
	routes := req.URL.Query()["endpoint"]
	if len(routes) == 0 {
		writeHttpResponse(w, httpResponse{
			contentType: "text/plain",
			statusCode:  http.StatusBadRequest,
			body:        []byte("missing endpoint query parameter"),
		})
		return
	}
	// we could support multiple endpoints in the future
	route := routes[0]
	log.Printf("Handling GetPayload request for %s payloads", route)
	payloads := fi.safeGetPayloads(route)

	// build response
	resp := getPayloadResponse{
		Payloads: payloads,
	}
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		writeHttpResponse(w, httpResponse{
			contentType: "text/plain",
			statusCode:  http.StatusInternalServerError,
			body:        []byte(err.Error()),
		})
		return
	}

	// send response
	writeHttpResponse(w, httpResponse{
		contentType: "application/json",
		statusCode:  http.StatusOK,
		body:        jsonResp,
	})
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
