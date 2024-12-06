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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/server/serverstore"
)

// defaultResponse is the default response returned by the fakeintake server
var defaultResponseByMethod map[string]httpResponse

func init() {
	defaultResponseByMethod = map[string]httpResponse{
		http.MethodGet: updateResponseFromData(httpResponse{
			statusCode: http.StatusOK,
		}),
		http.MethodPost: updateResponseFromData(httpResponse{
			statusCode:  http.StatusOK,
			contentType: "application/json",
			data: errorResponseBody{
				Errors: make([]string, 0),
			},
		}),
	}
}

// Option is a function that modifies a Server
type Option func(*Server)

// Server is a struct implementing a fakeintake server
type Server struct {
	uuid               uuid.UUID
	server             http.Server
	ready              chan bool
	clock              clock.Clock
	retention          time.Duration
	shutdown           chan struct{}
	dddevForward       bool
	forwardEndpoint    string
	logForwardEndpoint string
	apiKey             string

	urlMutex sync.RWMutex
	url      string

	storeDriver string
	store       serverstore.Store

	responseOverridesMutex    sync.RWMutex
	responseOverridesByMethod map[string]map[string]httpResponse
}

// NewServer creates a new fakeintake server and starts it on localhost:port
// options accept WithPort and WithReadyChan.
// Call Server.Start() to start the server in a separate go-routine
// If the port is 0, a port number is automatically chosen
func NewServer(options ...Option) *Server {
	fi := &Server{
		urlMutex:                  sync.RWMutex{},
		clock:                     clock.New(),
		retention:                 15 * time.Minute,
		responseOverridesMutex:    sync.RWMutex{},
		responseOverridesByMethod: newResponseOverrides(),
		server: http.Server{
			Addr: "0.0.0.0:0",
		},
		storeDriver:     "memory",
		forwardEndpoint: "https://app.datadoghq.com",
		// Source: https://docs.datadoghq.com/api/latest/logs/
		logForwardEndpoint: "https://agent-http-intake.logs.datadoghq.com",
	}

	for _, opt := range options {
		opt(fi)
	}

	fi.store = serverstore.NewStore(fi.storeDriver)
	registry := prometheus.NewRegistry()

	storeMetrics := fi.store.GetInternalMetrics()
	registry.MustRegister(
		append(
			[]prometheus.Collector{
				collectors.NewBuildInfoCollector(),
				collectors.NewGoCollector(),
				collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			},
			storeMetrics...)...,
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/", fi.handleDatadogRequest)
	mux.HandleFunc("/fakeintake/payloads", fi.handleGetPayloads)
	mux.HandleFunc("/fakeintake/health", fi.handleFakeHealth)
	mux.HandleFunc("/fakeintake/routestats", fi.handleGetRouteStats)
	mux.HandleFunc("/fakeintake/flushPayloads", fi.handleFlushPayloads)

	mux.HandleFunc("/fakeintake/configure/override", fi.handleConfigureOverride)

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		Registry:          registry,
	}))

	fi.server.Handler = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Fakeintake-ID", fi.uuid.String())
			next.ServeHTTP(w, r)
		})
	}(mux)

	return fi
}

// WithAddress changes the server host:port.
// If host is empty, it will bind to 0.0.0.0
// If the port is empty or 0, a port number is automatically chosen
func WithAddress(addr string) Option {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the port.")
			return
		}
		fi.server.Addr = addr
	}
}

// WithPort changes the server port.
// If the port is 0, a port number is automatically chosen
func WithPort(port int) Option {
	return WithAddress(fmt.Sprintf("0.0.0.0:%d", port))
}

// WithStoreDriver changes the store driver used by the server
func WithStoreDriver(driver string) func(*Server) {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the store driver.")
			return
		}
		fi.storeDriver = driver
	}
}

// WithReadyChannel assign a boolean channel to get notified when the server is ready
func WithReadyChannel(ready chan bool) Option {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the ready channel.")
			return
		}
		fi.ready = ready
	}
}

// WithClock changes the clock used by the server
func WithClock(clock clock.Clock) Option {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the clock.")
			return
		}
		fi.clock = clock
	}
}

// WithRetention changes the retention time of payloads in the store
func WithRetention(retention time.Duration) Option {
	return func(fi *Server) {
		if fi.IsRunning() {
			log.Println("Fake intake is already running. Stop it and try again to change the ready channel.")
			return
		}
		fi.retention = retention
	}
}

// WithDDDevForward enable forwarding payload to dddev
func WithDDDevForward() Option {
	return func(fi *Server) {
		apiKey, ok := os.LookupEnv("DD_API_KEY")
		if fi.apiKey == "" && !ok {
			log.Println("DD_API_KEY is not set, cannot forward to DDDev")
			return
		}
		if fi.apiKey == "" {
			fi.apiKey = apiKey
		}
		fi.dddevForward = true
	}
}

// WihDDDevAPIKey sets the API key to use when forwarding to DDDev, useful for testing
//
//nolint:unused // this function is used in the tests
func withDDDevAPIKey(apiKey string) Option {
	return func(fi *Server) {
		fi.apiKey = apiKey
	}
}

// withForwardEndpoint sets the endpoint to forward the payload to, useful for testing
//
//nolint:unused // this function is used in the tests
func withForwardEndpoint(endpoint string) Option {
	return func(fi *Server) {
		fi.forwardEndpoint = endpoint
	}
}

// withLogForwardEndpoint sets the endpoint to forward the log payload to, useful for testing
//
//nolint:unused // this function is used in the tests
func withLogForwardEndpoint(endpoint string) Option {
	return func(fi *Server) {
		fi.logForwardEndpoint = endpoint
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

	go fi.listenRoutine()
	go fi.cleanUpPayloadsRoutine()
}

// URL returns the URL of the fakeintake server
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

// IsRunning returns true if the fakeintake server is running
func (fi *Server) IsRunning() bool {
	return fi.URL() != ""
}

// Stop Gracefully stop the http server
func (fi *Server) Stop() error {
	if !fi.IsRunning() {
		return fmt.Errorf("server not running")
	}
	defer close(fi.shutdown)
	defer fi.store.Close()
	err := fi.server.Shutdown(context.Background())
	if err != nil {
		return err
	}
	return nil
}

func (fi *Server) listenRoutine() {
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
		log.Printf("Error listening at %s: %v", listener.Addr().String(), err)
		return
	}
}

func (fi *Server) cleanUpPayloadsRoutine() {
	ticker := fi.clock.Ticker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-fi.shutdown:
			return
		case <-ticker.C:
			now := fi.clock.Now()
			retentionTimeAgo := now.Add(-fi.retention)
			fi.store.CleanUpPayloadsOlderThan(retentionTimeAgo)
		}
	}
}

func (fi *Server) handleDatadogRequest(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		response := buildErrorResponse(errors.New("invalid request, nil request"))
		writeHTTPResponse(w, response)
		return
	}

	log.Printf("Handling Datadog %s request to %s, header %v", req.Method, req.URL.Path, redactHeader(req.Header))

	switch req.Method {
	case http.MethodPost:
		err := fi.handleDatadogPostRequest(w, req)
		if err == nil {
			return
		}

	case http.MethodGet:
		fallthrough
	case http.MethodHead:
		fallthrough
	default:
		if response, ok := fi.getResponseFromURLPath(req.Method, req.URL.Path); ok {
			writeHTTPResponse(w, response)
			return
		}
	}

	response := buildErrorResponse(fmt.Errorf("invalid request with route %s and method %s", req.URL.Path, req.Method))
	writeHTTPResponse(w, response)
}

func (fi *Server) forwardRequestToDDDev(req *http.Request, payload []byte) error {
	forwardEndpoint := fi.forwardEndpoint

	if req.URL.Path == "/api/v2/logs" || req.URL.Path == "/v1/input" {
		forwardEndpoint = fi.logForwardEndpoint
	}

	url := forwardEndpoint + req.URL.Path

	proxyReq, err := http.NewRequest(req.Method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	proxyReq.Header = make(http.Header)
	for h, val := range req.Header {
		if strings.ToLower(h) == "dd-api-key" {
			continue
		}
		proxyReq.Header[h] = val
	}

	proxyReq.Header["DD-API-KEY"] = []string{fi.apiKey}

	client := &http.Client{}
	res, err := client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return nil
}
func (fi *Server) handleDatadogPostRequest(w http.ResponseWriter, req *http.Request) error {
	if req.Body == nil {
		response := buildErrorResponse(errors.New("invalid request, nil body"))
		writeHTTPResponse(w, response)
		return nil
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err.Error())
		response := buildErrorResponse(err)
		writeHTTPResponse(w, response)
		return nil
	}
	if fi.dddevForward {
		err := fi.forwardRequestToDDDev(req, payload)
		if err != nil {
			log.Printf("Error forwarding request on endpoint %v to DDDev: %v", req.URL.Path, err)
		}
	}
	encoding := req.Header.Get("Content-Encoding")
	if req.URL.Path == "/support/flare" || encoding == "" {
		encoding = req.Header.Get("Content-Type")
	}
	contentType := req.Header.Get("Content-Type")

	err = fi.store.AppendPayload(req.URL.Path, payload, encoding, contentType, fi.clock.Now().UTC())
	if err != nil {
		log.Printf("Error adding payload to store: %v", err)
		response := buildErrorResponse(err)
		writeHTTPResponse(w, response)
		return nil
	}

	if response, ok := fi.getResponseFromURLPath(http.MethodPost, req.URL.Path); ok {
		writeHTTPResponse(w, response)
		return nil
	}

	return fmt.Errorf("no POST response found for path %s", req.URL.Path)
}

func (fi *Server) handleFlushPayloads(w http.ResponseWriter, _ *http.Request) {
	fi.store.Flush()

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
		payloads := fi.store.GetRawPayloads(route)

		// build response
		resp := api.APIFakeIntakePayloadsRawGETResponse{
			Payloads: payloads,
		}
		jsonResp, err = json.Marshal(resp)
	} else if serverstore.IsRouteHandled(route) {
		payloads, payloadErr := serverstore.GetJSONPayloads(fi.store, route)
		if payloadErr != nil {
			writeHTTPResponse(w, buildErrorResponse(payloadErr))
			return
		}

		resp := api.APIFakeIntakePayloadsJsonGETResponse{
			Payloads: payloads,
		}
		jsonResp, err = json.Marshal(resp)
	} else {
		writeHTTPResponse(w, buildErrorResponse(fmt.Errorf("invalid route parameter")))
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

func (fi *Server) handleGetRouteStats(w http.ResponseWriter, _ *http.Request) {
	log.Print("Handling getRouteStats request")
	routes := fi.store.GetRouteStats()
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

// handleConfigureOverride sets a hardcoded HTTP response for requests to a particular endpoint
func (fi *Server) handleConfigureOverride(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		response := buildErrorResponse(errors.New("invalid request, nil request"))
		writeHTTPResponse(w, response)
		return
	}

	if req.Method != http.MethodPost {
		response := buildErrorResponse(fmt.Errorf("invalid request method %s", req.Method))
		writeHTTPResponse(w, response)
		return
	}

	if req.Body == nil {
		response := buildErrorResponse(errors.New("invalid request, nil body"))
		writeHTTPResponse(w, response)
		return
	}

	var payload api.ResponseOverride
	err := json.NewDecoder(req.Body).Decode(&payload)
	if err != nil {
		log.Printf("Error reading body: %v", err.Error())
		response := buildErrorResponse(err)
		writeHTTPResponse(w, response)
		return
	}

	if payload.Method == "" {
		payload.Method = http.MethodPost
	}

	if !isValidMethod(payload.Method) {
		response := buildErrorResponse(fmt.Errorf("invalid request method %s", payload.Method))
		writeHTTPResponse(w, response)
		return
	}

	log.Printf("Handling configureOverride request for endpoint %s", payload.Endpoint)

	fi.safeSetResponseOverride(payload.Method, payload.Endpoint, httpResponse{
		statusCode:  payload.StatusCode,
		contentType: payload.ContentType,
		body:        payload.Body,
	})

	writeHTTPResponse(w, httpResponse{
		statusCode: http.StatusOK,
	})
}

func (fi *Server) safeSetResponseOverride(method string, endpoint string, response httpResponse) {
	fi.responseOverridesMutex.Lock()
	defer fi.responseOverridesMutex.Unlock()
	fi.responseOverridesByMethod[method][endpoint] = response
}

// getResponseFromURLPath returns the HTTP response for a given URL path, or the default response if
// no override exists
func (fi *Server) getResponseFromURLPath(method string, path string) (httpResponse, bool) {
	fi.responseOverridesMutex.RLock()
	defer fi.responseOverridesMutex.RUnlock()

	// by default, update the response for POST requests
	if method == "" {
		method = http.MethodPost
	}

	if respForMethod, ok := fi.responseOverridesByMethod[method]; ok {
		if resp, ok := respForMethod[path]; ok {
			return resp, true
		}
	}

	response, found := defaultResponseByMethod[method]
	return response, found
}
