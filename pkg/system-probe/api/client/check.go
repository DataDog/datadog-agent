// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	checkLabelName     = "check"
	telemetrySubsystem = "system_probe__remote_client"
)

var checkTelemetry = struct {
	totalRequests      telemetry.Counter
	failedRequests     telemetry.Counter
	failedResponses    telemetry.Counter
	responseErrors     telemetry.Counter
	malformedResponses telemetry.Counter
	requestDuration    telemetry.Gauge
}{
	telemetry.NewCounter(telemetrySubsystem, "requests__total", []string{checkLabelName}, "Counter measuring how many system-probe check requests were made"),
	telemetry.NewCounter(telemetrySubsystem, "requests__failed", []string{checkLabelName}, "Counter measuring how many system-probe check requests failed to be sent"),
	telemetry.NewCounter(telemetrySubsystem, "responses__not_received", []string{checkLabelName}, "Counter measuring how many responses from system-probe check were not read from the socket"),
	telemetry.NewCounter(telemetrySubsystem, "responses__errors", []string{checkLabelName}, "Counter measuring how many non_ok status code received from system-probe checks"),
	telemetry.NewCounter(telemetrySubsystem, "responses__malformed", []string{checkLabelName}, "Counter measuring how many malformed responses were received from system-probe checks"),
	telemetry.NewGauge(telemetrySubsystem, "requests__duration", []string{checkLabelName, "status"}, "Histogram measuring the duration of system-probe check requests"),
}

// startChecker is a helper to ensure that the system-probe is started before making a request. It's
// a singleton shared by all check clients.
type startChecker struct {
	mutex          sync.Mutex
	startTime      time.Time
	startupTimeout time.Duration
	started        bool
	neverStarted   bool
}

// getStartChecker is a memoized function that returns the singleton startChecker.
var getStartChecker = funcs.MemoizeNoError[*startChecker](func() *startChecker {
	return &startChecker{
		startTime:      time.Now(),
		startupTimeout: pkgconfigsetup.Datadog().GetDuration("check_system_probe_startup_time"),
	}
})

// ensureStarted ensures that the system-probe is started before making a
// request. Returns an error if the system-probe is not started yet. The error
// should be checked with IgnoreStartupError(), to avoid propagating errors to
// the check infrastructure.
func (c *startChecker) ensureStarted(client *http.Client) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.started {
		return nil
	}

	if c.neverStarted {
		return ErrNotAvailable
	}

	req, err := http.NewRequest("GET", "http://sysprobe/debug/stats", nil)
	if err != nil {
		return err
	}

	_, err = doReq(client, req, "status")
	if err != nil {
		if time.Since(c.startTime) < c.startupTimeout {
			// For the first few minutes after startup, only emit warnings
			// instead of reporting errors from the check, to allow a reasonable
			// time for system-probe to become ready to serve requests
			log.Warnf("system-probe not started yet: %v", err)

			// Callers should check for this error and not propagate it to avoid
			// error logs from the check infrastructure.
			return ErrNotStartedYet
		}

		// Past the startup timeout and system-probe never started.
		// Mark as permanently unavailable to suppress future errors.
		c.neverStarted = true
		log.Warnf("system-probe did not start within the startup timeout, marking as unavailable: %v", err)
		return ErrNotAvailable
	}

	c.started = true
	return nil
}

// CheckClient is a client for communicating with the system-probe check API
type CheckClient struct {
	checkClient    *http.Client
	startupClient  *http.Client
	startupChecker *startChecker
}

// checkClientConfig is the configuration for the check client.
type checkClientConfig struct {
	startupCheckRequestTimeout time.Duration
	checkRequestTimeout        time.Duration
	socketPath                 string
}

// CheckClientOption is a function that can be used to configure the check client.
type CheckClientOption func(c *checkClientConfig)

// GetCheckClient returns a new check client with the given options.
func GetCheckClient(options ...CheckClientOption) *CheckClient {
	config := &checkClientConfig{
		startupCheckRequestTimeout: defaultHTTPTimeout,
		checkRequestTimeout:        pkgconfigsetup.Datadog().GetDuration("check_system_probe_timeout"),
		socketPath:                 pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"),
	}

	for _, option := range options {
		option(config)
	}

	return &CheckClient{
		checkClient: &http.Client{
			Timeout: config.checkRequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          2,
				IdleConnTimeout:       idleConnTimeout,
				DialContext:           DialContextFunc(config.socketPath),
				TLSHandshakeTimeout:   1 * time.Second,
				ResponseHeaderTimeout: config.checkRequestTimeout,
				ExpectContinueTimeout: 50 * time.Millisecond,
			},
		},
		startupClient: &http.Client{
			Timeout: config.startupCheckRequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          2,
				IdleConnTimeout:       idleConnTimeout,
				DialContext:           DialContextFunc(config.socketPath),
				TLSHandshakeTimeout:   1 * time.Second,
				ResponseHeaderTimeout: 5 * time.Second,
				ExpectContinueTimeout: 50 * time.Millisecond,
			},
		},
		startupChecker: getStartChecker(),
	}
}

// WithCheckTimeout configures the check request timeout. This is
// HTTP timeout when making a request to the check endpoint once system-probe is
// started.
func WithCheckTimeout(timeout time.Duration) CheckClientOption {
	return func(c *checkClientConfig) {
		c.checkRequestTimeout = timeout
	}
}

// WithStartupCheckTimeout configures the startup check request timeout. This is
// the HTTP timeout when making a request to the debug/stats endpoint to check
// if system-probe is started.
func WithStartupCheckTimeout(timeout time.Duration) CheckClientOption {
	return func(c *checkClientConfig) {
		c.startupCheckRequestTimeout = timeout
	}
}

// WithSocketPath configures the socket path to use for the check client.
func WithSocketPath(socketPath string) CheckClientOption {
	return func(c *checkClientConfig) {
		c.socketPath = socketPath
	}
}

func doReq(client *http.Client, req *http.Request, module types.ModuleName) (body []byte, err error) {
	startTime := time.Now()
	defer func() {
		status := "error"
		if err == nil {
			status = "success"
		}
		checkTelemetry.requestDuration.Set(float64(time.Since(startTime).Milliseconds()), string(module), status)
	}()

	resp, err := client.Do(req)
	if err != nil {
		checkTelemetry.failedRequests.IncWithTags(map[string]string{checkLabelName: string(module)})
		return nil, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		checkTelemetry.failedResponses.IncWithTags(map[string]string{checkLabelName: string(module)})
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		checkTelemetry.responseErrors.IncWithTags(map[string]string{checkLabelName: string(module)})
		return nil, fmt.Errorf("non-ok status code: url %s, status_code: %d, response: `%s`", req.URL, resp.StatusCode, string(body))
	}

	return body, err
}

// GetCheck returns data unmarshalled from JSON to T, from the specified module at the /<module>/check endpoint.
func GetCheck[T any](client *CheckClient, module types.ModuleName) (T, error) {
	return request[T](client, http.MethodGet, "/check", nil, module)
}

// Post makes a POST request to a module endpoint with an optional JSON
// request body and returns data unmarshalled from JSON to T.  The endpoint
// parameter should be the path relative to the module (e.g., "/check",
// "/services").
func Post[T any](client *CheckClient, endpoint string, requestBody any, module types.ModuleName) (T, error) {
	return request[T](client, http.MethodPost, endpoint, requestBody, module)
}

func request[T any](client *CheckClient, method string, endpoint string, requestBody any, module types.ModuleName) (T, error) {
	var data T
	err := client.startupChecker.ensureStarted(client.startupClient)
	if err != nil {
		return data, err
	}

	checkTelemetry.totalRequests.IncWithTags(map[string]string{checkLabelName: string(module)})

	var bodyReader io.Reader
	if requestBody != nil {
		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			return data, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, ModuleURL(module, endpoint), bodyReader)
	if err != nil {
		return data, err
	}

	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	body, err := doReq(client.checkClient, req, module)
	if err != nil {
		return data, err
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		checkTelemetry.malformedResponses.IncWithTags(map[string]string{checkLabelName: string(module)})
	}
	return data, err
}

// IgnoreStartupError is used to avoid reporting errors from checks if
// system-probe has not started yet, or if it never started and is
// considered permanently unavailable for this agent lifetime.
func IgnoreStartupError(err error) error {
	if errors.Is(err, ErrNotStartedYet) || errors.Is(err, ErrNotAvailable) {
		return nil
	}
	return err
}
