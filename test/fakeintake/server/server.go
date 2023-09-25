// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a dummy http Datadog intake, meant to be used with integration and e2e tests.
// It runs an catch-all http server that stores submitted payloads into a dictionary of [api.Payloads], indexed by the route
// It implements 3 testing endpoints:
//   - /fakeintake/payloads/<payload_route> returns any received payloads on the specified route as [api.Payload]s
//   - /fakeintake/health returns current fakeintake server health
//   - /fakeintake/routestats returns stats for collected payloads, by route
//   - /fakeintake/flushPayloads returns all stored payloads and clear them up
//
// [api.Payloads]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/fakeintake/api#Payload
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
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/benbjohnson/clock"
)

type Server struct {
	server    http.Server
	ready     chan bool
	clock     clock.Clock
	retention time.Duration
	shutdown  chan struct{}

	urlMutex sync.RWMutex
	url      string

	storeMutex         sync.RWMutex
	payloadStore       map[string][]api.Payload
	payloadParsedStore PayloadParsedStore
}

// NewServer creates a new fake intake server and starts it on localhost:port
// options accept WithPort and WithReadyChan.
// Call Server.Start() to start the server in a separate go-routine
// If the port is 0, a port number is automatically chosen
func NewServer(options ...func(*Server)) *Server {
	fi := &Server{
		storeMutex:         sync.RWMutex{},
		urlMutex:           sync.RWMutex{},
		payloadStore:       map[string][]api.Payload{},
		clock:              clock.New(),
		payloadParsedStore: NewPayloadParsedStore(),
		retention:          15 * time.Minute,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", fi.handleDatadogRequest)
	mux.HandleFunc("/fakeintake/payloads/", fi.handleGetPayloads)
	mux.HandleFunc("/fakeintake/health/", fi.handleFakeHealth)
	mux.HandleFunc("/fakeintake/routestats/", fi.handleGetRouteStats)
	mux.HandleFunc("/fakeintake/flushPayloads/", fi.handleFlushPayloads)

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
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the port.")
			return
		}
		fi.server.Addr = fmt.Sprintf(":%d", port)
	}
}

// WithReadyChannel assign a boolean channel to get notified when the server is ready.
func WithReadyChannel(ready chan bool) func(*Server) {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the ready channel.")
			return
		}
		fi.ready = ready
	}
}

func WithClock(clock clock.Clock) func(*Server) {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the clock.")
			return
		}
		fi.clock = clock
	}
}

func WithRetention(retention time.Duration) func(*Server) {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the ready channel.")
			return
		}
		fi.retention = retention
	}
}

// Start Starts a fake intake server in a separate go-routine
// Notifies when ready to the ready channel
func (fi *Server) Start() {
	if fi.IsRunning() {
		log.Printf("Fake intake already running at %s", fi.URL())
		if fi.ready != nil {
			fi.ready <- true
		}
		return
	}
	fi.shutdown = make(chan struct{})
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
		fi.setURL("http://" + listener.Addr().String())
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
	go fi.cleanUpPayloadsRoutine()
}

func (fi *Server) URL() string {
	fi.urlMutex.RLock()
	defer fi.urlMutex.RUnlock()
	return fi.url
}

func (fi *Server) setURL(url string) {
	fi.urlMutex.Lock()
	defer fi.urlMutex.Unlock()
	fi.url = url
}

func (fi *Server) IsRunning() bool {
	return fi.URL() != ""
}

// Stop Gracefully stop the http server
func (fi *Server) Stop() error {
	if !fi.IsRunning() {
		return fmt.Errorf("server not running")
	}
	defer close(fi.shutdown)
	err := fi.server.Shutdown(context.Background())
	if err != nil {
		return err
	}

	fi.setURL("")
	return nil
}

func (fi *Server) cleanUpPayloadsRoutine() {
	ticker := fi.clock.Ticker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-fi.shutdown:
			return
		case <-ticker.C:
			fi.cleanUpPayloads()
		}
	}
}

func (fi *Server) cleanUpPayloads() {
	now := fi.clock.Now().UTC()
	fi.storeMutex.Lock()
	defer fi.storeMutex.Unlock()
	for route, payloads := range fi.payloadStore {
		n := 0
		for _, payload := range payloads {
			if now.Before(payload.Timestamp.Add(fi.retention)) {
				fi.payloadStore[route][n] = payload
				n++
			}
		}
		fi.payloadStore[route] = fi.payloadStore[route][:n]
	}

}
func (fi *Server) handleDatadogRequest(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		response := buildErrorResponse(errors.New("invalid request, nil request"))
		writeHTTPResponse(w, response)
		return
	}

	log.Printf("Handling Datadog %s request to %s, header %v", req.Method, req.URL.Path, req.Header)

	if req.Method == http.MethodGet {
		writeHTTPResponse(w, httpResponse{
			statusCode: http.StatusOK,
		})
		return
	}

	// Datadog Agent sends a HEAD request to avoid redirect issue before sending the actual flare
	if req.Method == http.MethodHead && req.URL.Path == "/support/flare" {
		writeHTTPResponse(w, httpResponse{
			statusCode: http.StatusOK,
		})
		return
	}

	// Datadog Agent sends a HEAD request to avoid redirect issue before sending the actual flare
	if req.Method == http.MethodHead && req.URL.Path == "/support/flare" {
		writeHTTPResponse(w, httpResponse{
			statusCode: http.StatusOK,
		})
		return
	}

	// from now on accept only POST requests
	if req.Method != http.MethodPost {
		response := buildErrorResponse(fmt.Errorf("invalid request with route %s and method %s", req.URL.Path, req.Method))
		writeHTTPResponse(w, response)
		return
	}

	if req.Body == nil {
		response := buildErrorResponse(errors.New("invalid request, nil body"))
		writeHTTPResponse(w, response)
		return
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err.Error())
		response := buildErrorResponse(err)
		writeHTTPResponse(w, response)
		return
	}

	// TODO: store all headers directly, and fetch Content-Type/Content-Encoding values when parsing
	encoding := req.Header.Get("Content-Encoding")
	if req.URL.Path == "/support/flare" || encoding == "" {
		encoding = req.Header.Get("Content-Type")
	}

	err = fi.safeAppendPayload(req.URL.Path, payload, encoding)
	if err != nil {
		log.Printf("Error caching payload: %v", err.Error())
		response := buildErrorResponse(err)
		writeHTTPResponse(w, response)
		return
	}

	response := getResponseFromURLPath(req.URL.Path)
	writeHTTPResponse(w, response)
}

func (fi *Server) handleFlushPayloads(w http.ResponseWriter, _ *http.Request) {
	fi.safeFlushPayloads()

	// send response
	writeHTTPResponse(w, httpResponse{
		statusCode: http.StatusOK,
	})
}

func (fi *Server) handleGetPayloads(w http.ResponseWriter, req *http.Request) {
	routes := req.URL.Query()["endpoint"]
	if len(routes) == 0 {
		writeHTTPResponse(w, httpResponse{
			contentType: "text/plain",
			statusCode:  http.StatusBadRequest,
			body:        []byte("missing endpoint query parameter"),
		})
		return
	}

	// we could support multiple endpoints in the future
	route := routes[0]
	log.Printf("Handling GetPayload request for %s payloads.", route)
	var jsonResp []byte
	var err error
	if req.URL.Query().Get("format") != "json" {
		payloads := fi.safeGetRawPayloads(route)

		// build response
		resp := api.APIFakeIntakePayloadsRawGETResponse{
			Payloads: payloads,
		}
		jsonResp, err = json.Marshal(resp)
	} else if fi.payloadParsedStore.IsRouteHandled(route) {
		payloads, payloadErr := fi.safeGetJsonPayloads(route)
		if payloadErr != nil {
			writeHTTPResponse(w, httpResponse{
				contentType: "text/plain",
				statusCode:  http.StatusBadRequest,
				body:        []byte(payloadErr.Error()),
			})
			return
		}
		// build response
		resp := api.APIFakeIntakePayloadsJsonGETResponse{
			Payloads: payloads,
		}
		jsonResp, err = json.Marshal(resp)
	} else {
		writeHTTPResponse(w, httpResponse{
			contentType: "text/plain",
			statusCode:  http.StatusBadRequest,
			body:        []byte("invalid route parameter"),
		})
		return
	}

	if err != nil {
		writeHTTPResponse(w, httpResponse{
			contentType: "text/plain",
			statusCode:  http.StatusInternalServerError,
			body:        []byte(err.Error()),
		})
		return
	}
	// send response
	writeHTTPResponse(w, httpResponse{
		contentType: "application/json",
		statusCode:  http.StatusOK,
		body:        jsonResp,
	})
}

func (fi *Server) handleFakeHealth(w http.ResponseWriter, _ *http.Request) {
	writeHTTPResponse(w, httpResponse{
		statusCode: http.StatusOK,
	})
}

func (fi *Server) safeAppendPayload(route string, data []byte, encoding string) error {
	fi.storeMutex.Lock()
	defer fi.storeMutex.Unlock()
	if _, found := fi.payloadStore[route]; !found {
		fi.payloadStore[route] = []api.Payload{}
	}
	rawPayload := api.Payload{
		Timestamp: fi.clock.Now().UTC(),
		Data:      data,
		Encoding:  encoding,
	}
	fi.payloadStore[route] = append(fi.payloadStore[route], rawPayload)
	return fi.payloadParsedStore.parseAndAppend(rawPayload, route)
}

func (fi *Server) safeGetRawPayloads(route string) []api.Payload {
	fi.storeMutex.Lock()
	defer fi.storeMutex.Unlock()
	payloads := make([]api.Payload, 0, len(fi.payloadStore[route]))
	payloads = append(payloads, fi.payloadStore[route]...)
	return payloads
}

func (fi *Server) safeGetJsonPayloads(route string) ([]api.ParsedPayload, error) {
	fi.storeMutex.Lock()
	defer fi.storeMutex.Unlock()
	payload, err := fi.payloadParsedStore.getJSONPayload(route)
	if err != nil {
		return nil, err
	}
	payloads := make([]api.ParsedPayload, 0, len(payload))
	payloads = append(payloads, payload...)
	return payloads, err
}

func (fi *Server) safeFlushPayloads() {
	fi.storeMutex.Lock()
	defer fi.storeMutex.Unlock()
	fi.payloadStore = map[string][]api.Payload{}
	fi.payloadParsedStore.Clean()
}

func (fi *Server) handleGetRouteStats(w http.ResponseWriter, req *http.Request) {
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
		writeHTTPResponse(w, httpResponse{
			contentType: "text/plain",
			statusCode:  http.StatusInternalServerError,
			body:        []byte(err.Error()),
		})
		return
	}

	// send response
	writeHTTPResponse(w, httpResponse{
		contentType: "application/json",
		statusCode:  http.StatusOK,
		body:        jsonResp,
	})
}

func (fi *Server) safeGetRouteStats() map[string]int {
	routes := map[string]int{}
	fi.storeMutex.Lock()
	defer fi.storeMutex.Unlock()
	for route, payloads := range fi.payloadStore {
		routes[route] = len(payloads)
	}
	return routes
}
