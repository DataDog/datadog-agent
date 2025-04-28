// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains the client for the API exposed by system-probe
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkLabelName     = "check"
	telemetrySubsystem = "system_probe__remote_client"
)

var (
	// ErrNotImplemented is an error used when system-probe is attempted to be accessed on an unsupported OS
	ErrNotImplemented = errors.New("system-probe unsupported")
	// ErrNotStartedYet is an error used when system-probe is attempted to be
	// accessed while it hasn't started yet (and could still be reasonably
	// expected to)
	ErrNotStartedYet = errors.New("system-probe not started yet")
)

var checkTelemetry = struct {
	totalRequests      telemetry.Counter
	failedRequests     telemetry.Counter
	failedResponses    telemetry.Counter
	responseErrors     telemetry.Counter
	malformedResponses telemetry.Counter
}{
	telemetry.NewCounter(telemetrySubsystem, "requests__total", []string{checkLabelName}, "Counter measuring how many system-probe check requests were made"),
	telemetry.NewCounter(telemetrySubsystem, "requests__failed", []string{checkLabelName}, "Counter measuring how many system-probe check requests failed to be sent"),
	telemetry.NewCounter(telemetrySubsystem, "responses__not_received", []string{checkLabelName}, "Counter measuring how many responses from system-probe check were not read from the socket"),
	telemetry.NewCounter(telemetrySubsystem, "responses__errors", []string{checkLabelName}, "Counter measuring how many non_ok status code received from system-probe checks"),
	telemetry.NewCounter(telemetrySubsystem, "responses__malformed", []string{checkLabelName}, "Counter measuring how many malformed responses were received from system-probe checks"),
}

// Get returns a http client configured to talk to the system-probe
var Get = funcs.MemoizeArgNoError[string, *http.Client](get)

// GetCheckClient returns a client used to communicate with the system-probe check API
var GetCheckClient = funcs.MemoizeArgNoError(getCheckClient)

// CheckClient is a client for communicating with the system-probe check API
type CheckClient struct {
	client         *http.Client
	startupClient  *http.Client
	started        bool
	startTime      time.Time
	mutex          sync.Mutex
	startupTimeout time.Duration
}

func get(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:          2,
			IdleConnTimeout:       idleConnTimeout,
			DialContext:           DialContextFunc(socketPath),
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}

func getCheckClient(socketPath string) *CheckClient {
	timeout := pkgconfigsetup.Datadog().GetDuration("check_system_probe_timeout")
	return &CheckClient{
		// This client has longer timeouts than the default, since some checks
		// may need it.
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:          2,
				IdleConnTimeout:       idleConnTimeout,
				DialContext:           DialContextFunc(socketPath),
				TLSHandshakeTimeout:   1 * time.Second,
				ResponseHeaderTimeout: timeout,
				ExpectContinueTimeout: 50 * time.Millisecond,
			},
		},
		startupClient:  get(socketPath),
		startTime:      time.Now(),
		startupTimeout: pkgconfigsetup.Datadog().GetDuration("check_system_probe_startup_time"),
	}
}

func doReq(client *http.Client, req *http.Request, module types.ModuleName) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		checkTelemetry.failedRequests.IncWithTags(map[string]string{checkLabelName: string(module)})
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
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

func (c *CheckClient) ensureStarted(module types.ModuleName) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.started {
		return nil
	}

	req, err := http.NewRequest("GET", "http://sysprobe/debug/stats", nil)
	if err != nil {
		return err
	}

	_, err = doReq(c.startupClient, req, "status")
	if err != nil {
		if time.Since(c.startTime) < c.startupTimeout {
			// For the first few minutes after startup, only emit warnings
			// instead of reporting errors from the check, to allow a reasonable
			// time for system-probe to become ready to serve requests
			log.Warnf("%v: system-probe not started yet: %v", string(module), err)

			// Callers should check for this error and not propagate it to avoid
			// error logs from the check infrastructure.
			return ErrNotStartedYet
		}

		return err
	}

	c.started = true
	return nil
}

// GetCheck returns data unmarshalled from JSON to T, from the specified module at the /<module>/check endpoint.
func GetCheck[T any](client *CheckClient, module types.ModuleName) (T, error) {
	var data T
	err := client.ensureStarted(module)
	if err != nil {
		return data, err
	}

	checkTelemetry.totalRequests.IncWithTags(map[string]string{checkLabelName: string(module)})

	req, err := http.NewRequest("GET", ModuleURL(module, "/check"), nil)
	if err != nil {
		//we don't have a counter for this case, because this function can't really fail, since ModuleURL function constructs a safe URL
		return data, err
	}

	body, err := doReq(client.client, req, module)
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

func constructURL(module string, endpoint string) string {
	u, _ := url.Parse("http://sysprobe")
	if module != "" {
		u = u.JoinPath(module)
	}
	path, query, found := strings.Cut(endpoint, "?")
	u = u.JoinPath(path)
	if found {
		u.RawQuery = query
	}
	return u.String()
}

// URL constructs a system-probe URL for a module-less endpoint.
func URL(endpoint string) string {
	return constructURL("", endpoint)
}

// DebugURL constructs a system-probe URL for the debug module and endpoint.
func DebugURL(endpoint string) string {
	return constructURL("debug", endpoint)
}

// ModuleURL constructs a system-probe ModuleURL given the specified module and endpoint.
func ModuleURL(module types.ModuleName, endpoint string) string {
	return constructURL(string(module), endpoint)
}

// ReadAllResponseBody reads the entire HTTP response body as a byte slice
func ReadAllResponseBody(resp *http.Response) ([]byte, error) {
	// if we are not able to determine the content length
	// we read the whole body without pre-allocation
	if resp.ContentLength <= 0 {
		return io.ReadAll(resp.Body)
	}

	// if we know the content length we pre-allocate the buffer
	var buf bytes.Buffer
	buf.Grow(int(resp.ContentLength))

	_, err := buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
