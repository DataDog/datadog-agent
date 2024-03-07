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
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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
	telemetryName string,
	cfg pkgconfigmodel.Reader) *Destination {

	return newDestination(endpoint,
		contentType,
		destinationsContext,
		time.Second*10,
		maxConcurrentBackgroundSends,
		shouldRetry,
		telemetryName,
		cfg)
}

func newDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	timeout time.Duration,
	maxConcurrentBackgroundSends int,
	shouldRetry bool,
	telemetryName string,
	cfg pkgconfigmodel.Reader) *Destination {

	if maxConcurrentBackgroundSends <= 0 {
		maxConcurrentBackgroundSends = 1
	}
	policy := backoff.NewExpBackoffPolicy(
		endpoint.BackoffFactor,
		endpoint.BackoffBase,
		endpoint.BackoffMax,
		endpoint.RecoveryInterval,
		endpoint.RecoveryReset,
	)

	expVars := &expvar.Map{}
	expVars.AddFloat(expVarIdleMsMapKey, 0)
	expVars.AddFloat(expVarInUseMsMapKey, 0)
	if telemetryName != "" {
		metrics.DestinationExpVars.Set(telemetryName, expVars)
	}

	return &Destination{
		host:                endpoint.Host,
		url:                 buildURL(endpoint),
		apiKey:              endpoint.APIKey,
		contentType:         contentType,
		client:              httputils.NewResetClient(endpoint.ConnectionResetInterval, httpClientFactory(timeout, cfg)),
		destinationsContext: destinationsContext,
		climit:              make(chan struct{}, maxConcurrentBackgroundSends),
		wg:                  sync.WaitGroup{},
		backoff:             policy,
		protocol:            endpoint.Protocol,
		origin:              endpoint.Origin,
		lastRetryError:      nil,
		retryLock:           sync.Mutex{},
		shouldRetry:         shouldRetry,
		expVars:             expVars,
		telemetryName:       telemetryName,
	}
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
			log.Warnf("%s: sleeping until %v before retrying. Backoff duration %s due to %d errors", d.url, d.blockedUntil, backoffDuration.String(), d.nbErrors)
			d.waitForBackoff()
			metrics.RetryTimeSpent.Add(int64(backoffDuration))
			metrics.RetryCount.Add(1)
			metrics.TlmRetryCount.Add(1)
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
	d.retryLock.Lock()
	defer d.retryLock.Unlock()

	if _, ok := err.(*client.RetryableError); ok {
		d.nbErrors = d.backoff.IncError(d.nbErrors)
		if isRetrying != nil && d.lastRetryError == nil {
			isRetrying <- true
		}
		d.lastRetryError = err

		return true
	} else { //nolint:revive // TODO(AML) Fix revive linter
		d.nbErrors = d.backoff.DecError(d.nbErrors)
		if isRetrying != nil && d.lastRetryError != nil {
			isRetrying <- false
		}
		d.lastRetryError = nil

		return false
	}
}

func httpClientFactory(timeout time.Duration, cfg pkgconfigmodel.Reader) func() *http.Client {
	return func() *http.Client {
		return &http.Client{
			Timeout: timeout,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: httputils.CreateHTTPTransport(cfg),
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

func prepareCheckConnectivity(endpoint config.Endpoint, cfg pkgconfigmodel.Reader) (*client.DestinationsContext, *Destination) {
	ctx := client.NewDestinationsContext()
	// Lower the timeout to 5s because HTTP connectivity test is done synchronously during the agent bootstrap sequence
	destination := newDestination(endpoint, JSONContentType, ctx, time.Second*5, 0, false, "", cfg)
	return ctx, destination
}

func completeCheckConnectivity(ctx *client.DestinationsContext, destination *Destination) error {
	ctx.Start()
	defer ctx.Stop()
	return destination.unconditionalSend(&emptyJsonPayload)
}

// CheckConnectivity check if sending logs through HTTP works
func CheckConnectivity(endpoint config.Endpoint, cfg pkgconfigmodel.Reader) config.HTTPConnectivity {
	log.Info("Checking HTTP connectivity...")
	ctx, destination := prepareCheckConnectivity(endpoint, cfg)
	log.Infof("Sending HTTP connectivity request to %s...", destination.url)
	err := completeCheckConnectivity(ctx, destination)
	if err != nil {
		log.Warnf("HTTP connectivity failure: %v", err)
	} else {
		log.Info("HTTP connectivity successful")
	}
	return err == nil
}

//nolint:revive // TODO(AML) Fix revive linter
func CheckConnectivityDiagnose(endpoint config.Endpoint, cfg pkgconfigmodel.Reader) (url string, err error) {
	ctx, destination := prepareCheckConnectivity(endpoint, cfg)
	return destination.url, completeCheckConnectivity(ctx, destination)
}

func (d *Destination) waitForBackoff() {
	ctx, cancel := context.WithDeadline(d.destinationsContext.Context(), d.blockedUntil)
	defer cancel()
	<-ctx.Done()
}
