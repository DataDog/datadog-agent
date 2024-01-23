// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package http

import (
	"bytes"
	"context"
	"errors"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ContentType options,
const (
	TextContentType     = "text/plain"
	JSONContentType     = "application/json"
	ProtobufContentType = "application/x-protobuf"
)

// HTTP errors.
var (
	errClient = errors.New("client error")
	errServer = errors.New("server error")
	tlmSend   = telemetry.NewCounter("logs_client_http_destination", "send", []string{"endpoint_host", "error"}, "Payloads sent")
	tlmInUse  = telemetry.NewCounter("logs_client_http_destination", "in_use_ms", []string{"sender"}, "Time spent sending payloads in ms")
	tlmIdle   = telemetry.NewCounter("logs_client_http_destination", "idle_ms", []string{"sender"}, "Time spent idle while not sending payloads in ms")

	expVarIdleMsMapKey  = "idleMs"
	expVarInUseMsMapKey = "inUseMs"
)

// emptyJsonPayload is an empty payload used to check HTTP connectivity without sending logs.
//
//nolint:revive // TODO(AML) Fix revive linter
var emptyJsonPayload = message.Payload{Messages: []*message.Message{}, Encoded: []byte("{}")}

// Destination sends a payload over HTTP.
type Destination struct {
	// Config
	url                 string
	apiKey              string
	contentType         string
	host                string
	client              *httputils.ResetClient
	destinationsContext *client.DestinationsContext
	protocol            config.IntakeProtocol
	origin              config.IntakeOrigin

	// Concurrency
	climit chan struct{} // semaphore for limiting concurrent background sends
	wg     sync.WaitGroup

	// Retry
	backoff        backoff.Policy
	nbErrors       int
	blockedUntil   time.Time
	retryLock      sync.Mutex
	shouldRetry    bool
	lastRetryError error

	// Telemetry
	expVars       *expvar.Map
	telemetryName string
}

// NewDestination returns a new Destination.
// If `maxConcurrentBackgroundSends` > 0, then at most that many background payloads will be sent concurrently, else
// there is no concurrency and the background sending pipeline will block while sending each payload.
// TODO: add support for SOCKS5
func NewDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	maxConcurrentBackgroundSends int,
	shouldRetry bool,
	telemetryName string) *Destination {
	panic("not called")
}

func newDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	timeout time.Duration,
	maxConcurrentBackgroundSends int,
	shouldRetry bool,
	telemetryName string) *Destination {
	panic("not called")
}

func errorToTag(err error) string {
	if err == nil {
		return "none"
	} else if _, ok := err.(*client.RetryableError); ok {
		return "retryable"
	} else {
		return "non-retryable"
	}
}

// Start starts reading the input channel
func (d *Destination) Start(input chan *message.Payload, output chan *message.Payload, isRetrying chan bool) (stopChan <-chan struct{}) {
	stop := make(chan struct{})
	go d.run(input, output, stop, isRetrying)
	return stop
}

func (d *Destination) run(input chan *message.Payload, output chan *message.Payload, stopChan chan struct{}, isRetrying chan bool) {
	var startIdle = time.Now()

	for p := range input {
		idle := float64(time.Since(startIdle) / time.Millisecond)
		d.expVars.AddFloat(expVarIdleMsMapKey, idle)
		tlmIdle.Add(idle, d.telemetryName)
		var startInUse = time.Now()

		d.sendConcurrent(p, output, isRetrying)

		inUse := float64(time.Since(startInUse) / time.Millisecond)
		d.expVars.AddFloat(expVarInUseMsMapKey, inUse)
		tlmInUse.Add(inUse, d.telemetryName)
		startIdle = time.Now()
	}
	// Wait for any pending concurrent sends to finish or terminate
	d.wg.Wait()

	d.updateRetryState(nil, isRetrying)
	stopChan <- struct{}{}
}

func (d *Destination) sendConcurrent(payload *message.Payload, output chan *message.Payload, isRetrying chan bool) {
	d.wg.Add(1)
	d.climit <- struct{}{}
	go func() {
		defer func() {
			<-d.climit
			d.wg.Done()
		}()
		d.sendAndRetry(payload, output, isRetrying)
	}()
}

// Send sends a payload over HTTP,
func (d *Destination) sendAndRetry(payload *message.Payload, output chan *message.Payload, isRetrying chan bool) {
	for {

		d.retryLock.Lock()
		backoffDuration := d.backoff.GetBackoffDuration(d.nbErrors)
		d.blockedUntil = time.Now().Add(backoffDuration)
		if d.blockedUntil.After(time.Now()) {
			log.Debugf("%s: sleeping until %v before retrying. Backoff duration %s due to %d errors", d.url, d.blockedUntil, backoffDuration.String(), d.nbErrors)
			d.waitForBackoff()
		}
		d.retryLock.Unlock()

		err := d.unconditionalSend(payload)

		if err != nil {
			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()
			log.Warnf("Could not send payload: %v", err)
		}

		if err == context.Canceled {
			d.updateRetryState(nil, isRetrying)
			return
		}

		if d.shouldRetry {
			if d.updateRetryState(err, isRetrying) {
				continue
			}
		}

		metrics.LogsSent.Add(int64(len(payload.Messages)))
		metrics.TlmLogsSent.Add(float64(len(payload.Messages)))
		output <- payload
		return
	}
}

func (d *Destination) unconditionalSend(payload *message.Payload) (err error) {
	defer func() {
		tlmSend.Inc(d.host, errorToTag(err))
	}()

	ctx := d.destinationsContext.Context()

	if err != nil {
		return err
	}
	metrics.BytesSent.Add(int64(payload.UnencodedSize))
	metrics.TlmBytesSent.Add(float64(payload.UnencodedSize))
	metrics.EncodedBytesSent.Add(int64(len(payload.Encoded)))
	metrics.TlmEncodedBytesSent.Add(float64(len(payload.Encoded)))

	req, err := http.NewRequest("POST", d.url, bytes.NewReader(payload.Encoded))
	if err != nil {
		// the request could not be built,
		// this can happen when the method or the url are valid.
		return err
	}
	req.Header.Set("DD-API-KEY", d.apiKey)
	req.Header.Set("Content-Type", d.contentType)
	if payload.Encoding != "" {
		req.Header.Set("Content-Encoding", payload.Encoding)
	}
	if d.protocol != "" {
		req.Header.Set("DD-PROTOCOL", string(d.protocol))
	}
	if d.origin != "" {
		req.Header.Set("DD-EVP-ORIGIN", string(d.origin))
		req.Header.Set("DD-EVP-ORIGIN-VERSION", version.AgentVersion)
	}
	req.Header.Set("dd-message-timestamp", strconv.FormatInt(getMessageTimestamp(payload.Messages), 10))
	then := time.Now()
	req.Header.Set("dd-current-timestamp", strconv.FormatInt(then.UnixMilli(), 10))

	req = req.WithContext(ctx)
	resp, err := d.client.Do(req)

	latency := time.Since(then).Milliseconds()
	metrics.TlmSenderLatency.Observe(float64(latency))
	metrics.SenderLatency.Set(latency)

	if err != nil {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		// most likely a network or a connect error, the callee should retry.
		return client.NewRetryableError(err)
	}

	defer resp.Body.Close()
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		// the read failed because the server closed or terminated the connection
		// *after* serving the request.
		log.Debugf("Server closed or terminated the connection after serving the request with err %v", err)
		return err
	}

	metrics.DestinationHttpRespByStatusAndUrl.Add(strconv.Itoa(resp.StatusCode), 1)
	metrics.TlmDestinationHttpRespByStatusAndUrl.Inc(strconv.Itoa(resp.StatusCode), d.url)

	if resp.StatusCode >= http.StatusBadRequest {
		log.Warnf("failed to post http payload. code=%d host=%s response=%s", resp.StatusCode, d.host, string(response))
	}
	if resp.StatusCode == http.StatusBadRequest ||
		resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusRequestEntityTooLarge {
		// the logs-agent is likely to be misconfigured,
		// the URL or the API key may be wrong.
		return errClient
	} else if resp.StatusCode > http.StatusBadRequest {
		// the server could not serve the request, most likely because of an
		// internal error. We should retry these requests.
		return client.NewRetryableError(errServer)
	} else {
		return nil
	}
}

func (d *Destination) updateRetryState(err error, isRetrying chan bool) bool {
	panic("not called")
}

func httpClientFactory(timeout time.Duration) func() *http.Client {
	return func() *http.Client {
		return &http.Client{
			Timeout: timeout,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: httputils.CreateHTTPTransport(pkgconfig.Datadog),
		}
	}
}

// buildURL buils a url from a config endpoint.
func buildURL(endpoint config.Endpoint) string {
	var scheme string
	if endpoint.GetUseSSL() {
		scheme = "https"
	} else {
		scheme = "http"
	}
	var address string
	if endpoint.Port != 0 {
		address = fmt.Sprintf("%v:%v", endpoint.Host, endpoint.Port)
	} else {
		address = endpoint.Host
	}
	url := url.URL{
		Scheme: scheme,
		Host:   address,
	}
	if endpoint.Version == config.EPIntakeVersion2 && endpoint.TrackType != "" {
		url.Path = fmt.Sprintf("/api/v2/%s", endpoint.TrackType)
	} else {
		url.Path = "/v1/input"
	}
	return url.String()
}

func getMessageTimestamp(messages []*message.Message) int64 {
	timestampNanos := int64(-1)
	if len(messages) > 0 {
		timestampNanos = messages[len(messages)-1].IngestionTimestamp
	}
	return timestampNanos / int64(time.Millisecond/time.Nanosecond)
}

func prepareCheckConnectivity(endpoint config.Endpoint) (*client.DestinationsContext, *Destination) {
	panic("not called")
}

func completeCheckConnectivity(ctx *client.DestinationsContext, destination *Destination) error {
	panic("not called")
}

// CheckConnectivity check if sending logs through HTTP works
func CheckConnectivity(endpoint config.Endpoint) config.HTTPConnectivity {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func CheckConnectivityDiagnose(endpoint config.Endpoint) (url string, err error) {
	panic("not called")
}

func (d *Destination) waitForBackoff() {
	panic("not called")
}
