// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
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
	telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "requests__total", []string{checkLabelName}, "Counter measuring how many system-probe check requests were made"),
	telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "requests__failed", []string{checkLabelName}, "Counter measuring how many system-probe check requests failed to be sent"),
	telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "responses__not_received", []string{checkLabelName}, "Counter measuring how many responses from system-probe check were not read from the socket"),
	telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "responses__errors", []string{checkLabelName}, "Counter measuring how many non_ok status code received from system-probe checks"),
	telemetryimpl.GetCompatComponent().NewCounter(telemetrySubsystem, "responses__malformed", []string{checkLabelName}, "Counter measuring how many malformed responses were received from system-probe checks"),
	telemetryimpl.GetCompatComponent().NewGauge(telemetrySubsystem, "requests__duration", []string{checkLabelName, "status"}, "Histogram measuring the duration of system-probe check requests"),
}

// startChecker is a helper to ensure that the system-probe is started before making a request. It's
// a singleton shared by all check clients.
type startChecker struct {
	mutex          sync.Mutex
	startTime      time.Time
	startupTimeout time.Duration
	started        bool
	inFlight       chan struct{}
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
func (c *startChecker) ensureStarted(ctx context.Context, client *http.Client) error {
	for {
		c.mutex.Lock()
		if c.started {
			c.mutex.Unlock()
			return nil
		}
		if c.inFlight != nil {
			done := c.inFlight
			c.mutex.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				// The probe result belongs to its owner. Re-evaluate shared
				// state so an active waiter can start a new probe when the
				// previous owner was canceled or otherwise failed.
				continue
			}
		}

		done := make(chan struct{})
		c.inFlight = done
		startTime := c.startTime
		startupTimeout := c.startupTimeout
		c.mutex.Unlock()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://sysprobe/debug/stats", nil)
		if err == nil {
			_, err = doReq(client, req, "status")
		}

		c.mutex.Lock()
		if err == nil {
			c.started = true
		}
		c.inFlight = nil
		close(done)
		c.mutex.Unlock()

		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Since(startTime) < startupTimeout {
			// For the first few minutes after startup, only emit warnings
			// instead of reporting errors from the check, to allow a reasonable
			// time for system-probe to become ready to serve requests
			log.Warnf("system-probe not started yet: %v", err)

			// Callers should check for this error and not propagate it to avoid
			// error logs from the check infrastructure.
			return ErrNotStartedYet
		}
		return err
	}
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

	return NewCheckClient(
		&http.Client{
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
		&http.Client{
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
	)
}

// NewCheckClient builds a check client with the given HTTP clients.
func NewCheckClient(checkClient, startupClient *http.Client) *CheckClient {
	return &CheckClient{
		checkClient:    checkClient,
		startupClient:  startupClient,
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
	return GetCheckWithContext[T](context.Background(), client, module)
}

// GetCheckWithContext returns check data and cancels all startup and module
// HTTP work when ctx is canceled.
func GetCheckWithContext[T any](ctx context.Context, client *CheckClient, module types.ModuleName) (T, error) {
	return request[T](ctx, client, http.MethodGet, "/check", nil, module)
}

// Post makes a POST request to a module endpoint with an optional JSON
// request body and returns data unmarshalled from JSON to T.  The endpoint
// parameter should be the path relative to the module (e.g., "/check",
// "/services").
func Post[T any](client *CheckClient, endpoint string, requestBody any, module types.ModuleName) (T, error) {
	return PostWithContext[T](context.Background(), client, endpoint, requestBody, module)
}

// PostWithContext posts to a module endpoint and cancels all startup and
// module HTTP work when ctx is canceled.
func PostWithContext[T any](ctx context.Context, client *CheckClient, endpoint string, requestBody any, module types.ModuleName) (T, error) {
	return request[T](ctx, client, http.MethodPost, endpoint, requestBody, module)
}

func request[T any](ctx context.Context, client *CheckClient, method string, endpoint string, requestBody any, module types.ModuleName) (T, error) {
	var data T
	err := client.startupChecker.ensureStarted(ctx, client.startupClient)
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

	req, err := http.NewRequestWithContext(ctx, method, ModuleURL(module, endpoint), bodyReader)
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
// system-probe has not started yet and can reasonably be expected to.
func IgnoreStartupError(err error) error {
	if errors.Is(err, ErrNotStartedYet) {
		return nil
	}
	return err
}
