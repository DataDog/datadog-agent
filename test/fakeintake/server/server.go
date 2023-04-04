// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/benbjohnson/clock"
)

type Server struct {
	mu     sync.RWMutex
	server http.Server
	ready  chan bool
	clock  clock.Clock

	url string

	payloadStore map[string][]api.Payload
}

// NewServer creates a new fake intake server and starts it on localhost:port
// options accept WithPort and WithReadyChan.
// Call Server.Start() to start the server in a separate go-routine
// If the port is 0, a port number is automatically chosen
func NewServer(options ...func(*Server)) *Server {
	fi := &Server{
		mu:           sync.RWMutex{},
		payloadStore: map[string][]api.Payload{},
		clock:        clock.New(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", fi.handleDatadogRequest)
	mux.HandleFunc("/fakeintake/payloads/", fi.getPayloads)
	mux.HandleFunc("/fakeintake/health/", fi.getFakeHealth)
	mux.HandleFunc("/fakeintake/routestats/", fi.getRouteStats)

	fi.server = http.Server{
		Handler: mux,
		Addr:    ":0",
	}

	for _, opt := range options {
		opt(fi)
	}

	return fi
}

// WithPort changes the server port.
// If the port is 0, a port number is automatically chosen
func WithPort(port int) func(*Server) {
	return func(fi *Server) {
		if fi.URL() != "" {
			log.Println("Fake intake is already running. Stop it and try again to change the port.")
			return
		}
		fi.server.Addr = fmt.Sprintf(":%d", port)
	}
}

// WithReadyChannel assign a boolean channel to get notified when the server is ready.
func WithReadyChannel(ready chan bool) func(*Server) {
	return func(fi *Server) {
		if fi.URL() != "" {
			log.Println("Fake intake is already running. Stop it and try again to change the ready channel.")
			return
		}
		fi.ready = ready
	}
}

func WithClock(clock clock.Clock) func(*Server) {
	return func(fi *Server) {
		if fi.URL() != "" {
			log.Println("Fake intake is already running. Stop it and try again to change the clock.")
			return
		}
		fi.clock = clock
	}
}

// Start Starts a fake intake server in a separate go-routine
// Notifies when ready to the ready channel
func (fi *Server) Start() {
	if fi.URL() != "" {
		log.Printf("Fake intake alredy running at %s", fi.URL())
		if fi.ready != nil {
			fi.ready <- true
		}
		return
	}
	go func() {
		// explicitly creating a listener to get the actual port
		// as http.Server.ListenAndServe hides this information
		// https://github.com/golang/go/blob/go1.19.6/src/net/http/server.go#L2987-L3000
		listener, err := net.Listen("tcp", fi.server.Addr)
		if err != nil {
			log.Printf("Error creating fake intake server at %s: %v", fi.server.Addr, err)

			if fi.ready != nil {
				fi.ready <- false
			}

			return
		}
		fi.url = "http://" + listener.Addr().String()
		// notify server is ready, if anybody is listening
		if fi.ready != nil {
			fi.ready <- true
		}
		// server.Serve blocks and listens to requests
		err = fi.server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Error creating fake intake server at %s: %v", listener.Addr().String(), err)
			return
		}
	}()
}

func (fi *Server) URL() string {
	return fi.url
}

// Stop Gracefully stop the http server
func (fi *Server) Stop() error {
	if fi.URL() == "" {
		return fmt.Errorf("server not running")
	}
	err := fi.server.Shutdown(context.Background())
	if err != nil {
		return err
	}
	fi.url = ""
	return nil
}

type postPayloadResponse struct {
	Errors []string `json:"errors"`
}

func (fi *Server) handleDatadogRequest(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		response := buildPostResponse(errors.New("invalid request, nil request"))
		writeHttpResponse(w, response)
		return
	}

	log.Printf("Handling Datadog %s request to %s, header %v", req.Method, req.URL.Path, req.Header)

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

	fi.safeAppendPayload(req.URL.Path, payload, req.Header.Get("Content-Encoding"))
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

func (fi *Server) getPayloads(w http.ResponseWriter, req *http.Request) {
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
	resp := api.APIFakeIntakePayloadsGETResponse{
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

func (fi *Server) getFakeHealth(w http.ResponseWriter, req *http.Request) {
	writeHttpResponse(w, httpResponse{
		statusCode: http.StatusOK,
	})
}

func (fi *Server) safeAppendPayload(route string, data []byte, encoding string) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	if _, found := fi.payloadStore[route]; !found {
		fi.payloadStore[route] = []api.Payload{}
	}
	fi.payloadStore[route] = append(fi.payloadStore[route], api.Payload{
		Timestamp: fi.clock.Now(),
		Data:      data,
		Encoding:  encoding,
	})
}

func (fi *Server) safeGetPayloads(route string) []api.Payload {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	payloads := make([]api.Payload, 0, len(fi.payloadStore[route]))
	payloads = append(payloads, fi.payloadStore[route]...)
	return payloads
}

func (fi *Server) getRouteStats(w http.ResponseWriter, req *http.Request) {
	log.Print("Handling getRouteStats request")
	routes := fi.safeGetRouteStats()
	// build response
	resp := api.APIFakeIntakeRouteStatsGETResponse{
		Routes: map[string]api.RouteStat{},
	}
	for route, count := range routes {
		resp.Routes[route] = api.RouteStat{ID: route, Count: count}
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

func (fi *Server) safeGetRouteStats() map[string]int {
	routes := map[string]int{}
	fi.mu.Lock()
	defer fi.mu.Unlock()
	for route, payloads := range fi.payloadStore {
		routes[route] = len(payloads)
	}
	return routes
}
