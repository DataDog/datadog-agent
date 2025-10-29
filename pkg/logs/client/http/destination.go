// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"strings"
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

// NoTimeoutOverride is a special value that tells the httpClientFactory to use the logs_config.http_timeout setting.
// This should generally be used for all HTTP destinations barring special cases like the HTTP connectivity check.
const (
	NoTimeoutOverride = -1
)

// HTTP errors.
var (
	errClient  = errors.New("client error")
	errServer  = errors.New("server error")
	tlmSend    = telemetry.NewCounter("logs_client_http_destination", "send", []string{"endpoint_host", "error"}, "Payloads sent")
	tlmInUse   = telemetry.NewCounter("logs_client_http_destination", "in_use_ms", []string{"sender"}, "Time spent sending payloads in ms")
	tlmIdle    = telemetry.NewCounter("logs_client_http_destination", "idle_ms", []string{"sender"}, "Time spent idle while not sending payloads in ms")
	tlmDropped = telemetry.NewCounterWithOpts("logs_client_http_destination", "payloads_dropped", []string{}, "Number of payloads dropped because of unrecoverable errors", telemetry.Options{DefaultMetric: true})

	expVarIdleMsMapKey  = "idleMs"
	expVarInUseMsMapKey = "inUseMs"
)

// emptyJSONPayload is an empty payload used to check HTTP connectivity without sending logs.
var emptyJSONPayload = message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("{}")}

type destinationResult struct {
	latency time.Duration
	err     error
}

// Destination sends a payload over HTTP.
type Destination struct {
	// Config
	url                 string
	endpoint            config.Endpoint
	contentType         string
	host                string
	client              *httputils.ResetClient
	destinationsContext *client.DestinationsContext
	protocol            config.IntakeProtocol
	origin              config.IntakeOrigin
	isMRF               bool

	// Concurrency
	workerPool *workerPool
	wg         sync.WaitGroup

	// Retry
	backoff        backoff.Policy
	nbErrors       int
	retryLock      sync.Mutex
	shouldRetry    bool
	lastRetryError error

	// Telemetry
	expVars         *expvar.Map
	destMeta        *client.DestinationMetadata
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

// NewDestination returns a new Destination.
// minConcurrency denotes the minimum number of concurrent http requests the pipeline will allow at once.
// maxConcurrency represents the maximum number of concurrent http requests, reachable when the client is experiencing a large latency in sends.
// TODO: add support for SOCKS5
func NewDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	shouldRetry bool,
	destMeta *client.DestinationMetadata,
	cfg pkgconfigmodel.Reader,
	minConcurrency int,
	maxConcurrency int,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string) *Destination {

	return newDestination(endpoint,
		contentType,
		destinationsContext,
		NoTimeoutOverride,
		shouldRetry,
		destMeta,
		cfg,
		minConcurrency,
		maxConcurrency,
		pipelineMonitor,
		instanceID)
}

func newDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	timeoutOverride time.Duration,
	shouldRetry bool,
	destMeta *client.DestinationMetadata,
	cfg pkgconfigmodel.Reader,
	minConcurrency int,
	maxConcurrency int,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string) *Destination {

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

	if destMeta.ReportingEnabled {
		metrics.DestinationExpVars.Set(destMeta.TelemetryName(), expVars)
	}

	workerPool := newDefaultWorkerPool(minConcurrency, maxConcurrency, destMeta)

	return &Destination{
		host:                endpoint.Host,
		url:                 buildURL(endpoint),
		endpoint:            endpoint,
		contentType:         contentType,
		client:              httputils.NewResetClient(endpoint.ConnectionResetInterval, httpClientFactory(cfg, timeoutOverride)),
		destinationsContext: destinationsContext,
		workerPool:          workerPool,
		wg:                  sync.WaitGroup{},
		backoff:             policy,
		protocol:            endpoint.Protocol,
		origin:              endpoint.Origin,
		lastRetryError:      nil,
		retryLock:           sync.Mutex{},
		shouldRetry:         shouldRetry,
		expVars:             expVars,
		destMeta:            destMeta,
		isMRF:               endpoint.IsMRF,
		pipelineMonitor:     pipelineMonitor,
		utilization:         pipelineMonitor.MakeUtilizationMonitor(destMeta.MonitorTag(), instanceID),
		instanceID:          instanceID,
	}
}

func errorToTag(err error) string {
	if err == nil {
		return "none"
	}
	if _, ok := err.(*client.RetryableError); ok {
		return "retryable"
	}
	return "non-retryable"
}

// IsMRF indicates that this destination is a Multi-Region Failover destination.
func (d *Destination) IsMRF() bool {
	return d.isMRF
}

// Target is the address of the destination.
func (d *Destination) Target() string {
	return d.url
}

// Metadata returns the metadata of the destination
func (d *Destination) Metadata() *client.DestinationMetadata {
	return d.destMeta
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
		d.utilization.Start()
		idle := float64(time.Since(startIdle) / time.Millisecond)
		d.expVars.AddFloat(expVarIdleMsMapKey, idle)
		tlmIdle.Add(idle, d.destMeta.TelemetryName())
		var startInUse = time.Now()

		d.sendConcurrent(p, output, isRetrying)

		inUse := float64(time.Since(startInUse) / time.Millisecond)
		d.expVars.AddFloat(expVarInUseMsMapKey, inUse)
		tlmInUse.Add(inUse, d.destMeta.TelemetryName())
		startIdle = time.Now()
		d.utilization.Stop()
	}
	// Wait for any pending concurrent sends to finish or terminate
	d.wg.Wait()

	d.updateRetryState(nil, isRetrying)
	stopChan <- struct{}{}
}

func (d *Destination) sendConcurrent(payload *message.Payload, output chan *message.Payload, isRetrying chan bool) {
	d.wg.Add(1)
	d.workerPool.run(func() destinationResult {
		result := d.sendAndRetry(payload, output, isRetrying)
		d.wg.Done()
		return result
	})
}

// Send sends a payload over HTTP,
func (d *Destination) sendAndRetry(payload *message.Payload, output chan *message.Payload, isRetrying chan bool) destinationResult {
	for {
		d.retryLock.Lock()
		nbErrors := d.nbErrors
		d.retryLock.Unlock()
		backoffDuration := d.backoff.GetBackoffDuration(nbErrors)
		blockedUntil := time.Now().Add(backoffDuration)
		if blockedUntil.After(time.Now()) {
			log.Warnf("%s: sleeping until %v before retrying. Backoff duration %s due to %d errors", d.url, blockedUntil, backoffDuration.String(), nbErrors)
			d.waitForBackoff(blockedUntil)
			metrics.RetryTimeSpent.Add(int64(backoffDuration))
			metrics.RetryCount.Add(1)
			metrics.TlmRetryCount.Add(1)
		}

		start := time.Now()
		err := d.unconditionalSend(payload)
		latency := time.Since(start)
		result := destinationResult{
			latency: latency,
			err:     err,
		}

		if err != nil {
			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()

			// shouldRetry is false for serverless. This log line is too verbose for serverless so make it debug only.
			if d.shouldRetry {
				log.Warnf("Could not send payload: %v", err)
			} else {
				log.Debugf("Could not send payload: %v", err)
			}
		}

		if err == context.Canceled {
			d.updateRetryState(nil, isRetrying)
			return result
		}

		if d.shouldRetry {
			if d.updateRetryState(err, isRetrying) {
				continue
			}
		}

		metrics.LogsSent.Add(payload.Count())
		metrics.TlmLogsSent.Add(float64(payload.Count()))
		output <- payload
		return result
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
	var sourceTag string
	compressionKind := "none"

	if d.endpoint.UseCompression {
		compressionKind = d.endpoint.CompressionKind
	}

	if strings.Contains(d.Metadata().TelemetryName(), "logs") {
		sourceTag = "logs"
	} else {
		sourceTag = "epforwarder"
	}

	metrics.TlmBytesSent.Add(float64(payload.UnencodedSize), sourceTag)
	metrics.EncodedBytesSent.Add(int64(len(payload.Encoded)))
	metrics.TlmEncodedBytesSent.Add(float64(len(payload.Encoded)), sourceTag, compressionKind)

	req, err := http.NewRequest("POST", d.url, bytes.NewReader(payload.Encoded))
	if err != nil {
		// the request could not be built,
		// this can happen when the method or the url are valid.
		return err
	}
	req.Header.Set("DD-API-KEY", d.endpoint.GetAPIKey())
	req.Header.Set("Content-Type", d.contentType)
	req.Header.Set("User-Agent", fmt.Sprintf("datadog-agent/%s", version.AgentVersion))

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
	req.Header.Set("dd-message-timestamp", strconv.FormatInt(getMessageTimestamp(payload.MessageMetas), 10))
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
	log.Tracef("Log payload sent to %s. Response resolved with protocol %s in %d ms", d.url, resp.Proto, latency)

	metrics.DestinationHTTPRespByStatusAndURL.Add(strconv.Itoa(resp.StatusCode), 1)
	metrics.TlmDestinationHTTPRespByStatusAndURL.Inc(strconv.Itoa(resp.StatusCode), d.url)

	if resp.StatusCode >= http.StatusBadRequest {
		log.Warnf("failed to post http payload. code=%d, url=%s, EvP track type=%s, content type=%s, EvP category=%s, origin=%s, response=%s", resp.StatusCode, d.url, d.endpoint.TrackType, d.contentType, d.destMeta.EvpCategory(), d.origin, string(response))
	}
	if resp.StatusCode == http.StatusBadRequest ||
		resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusRequestEntityTooLarge {
		// the logs-agent is likely to be misconfigured,
		// the URL or the API key may be wrong.
		tlmDropped.Inc()
		return errClient
	} else if resp.StatusCode > http.StatusBadRequest {
		// the server could not serve the request, most likely because of an
		// internal error. We should retry these requests.
		return client.NewRetryableError(errServer)
	}
	d.pipelineMonitor.ReportComponentEgress(payload, d.destMeta.MonitorTag(), d.instanceID)
	return nil
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
	}
	d.nbErrors = d.backoff.DecError(d.nbErrors)
	if isRetrying != nil && d.lastRetryError != nil {
		isRetrying <- false
	}
	d.lastRetryError = nil

	return false
}

func httpClientFactory(cfg pkgconfigmodel.Reader, timeoutOverride time.Duration) func() *http.Client {
	var transport *http.Transport

	transportConfig := cfg.Get("logs_config.http_protocol")
	timeout := timeoutOverride
	if timeout == NoTimeoutOverride {
		timeout = time.Second * time.Duration(cfg.GetInt("logs_config.http_timeout"))
	}

	// Configure transport based on user setting
	switch transportConfig {
	case "http1":
		// Use default ALPN auto-negotiation to negotiate up to http/1.1
		transport = httputils.CreateHTTPTransport(cfg)
	case "auto":
		fallthrough
	default:
		if cfg.Get("logs_config.http_protocol") != "auto" {
			log.Warnf("Invalid http_protocol '%v', falling back to 'auto'", transportConfig)
		}
		// Use default ALPN auto-negotiation and negotiate to HTTP/2 if possible, if not it will automatically fallback to best available protocol
		transport = httputils.CreateHTTPTransport(cfg, httputils.WithHTTP2())
	}

	return func() *http.Client {
		client := &http.Client{
			Timeout: timeout,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: transport,
		}

		return client
	}
}

// buildURL buils a url from a config endpoint.
func buildURL(endpoint config.Endpoint) string {
	var scheme string
	if endpoint.UseSSL() {
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
		url.Path = fmt.Sprintf("%s/api/v2/%s", endpoint.PathPrefix, endpoint.TrackType)
	} else {
		url.Path = fmt.Sprintf("%s/v1/input", endpoint.PathPrefix)
	}
	return url.String()
}

func getMessageTimestamp(messages []*message.MessageMetadata) int64 {
	timestampNanos := int64(-1)
	if len(messages) > 0 {
		timestampNanos = messages[len(messages)-1].IngestionTimestamp
	}
	return timestampNanos / int64(time.Millisecond/time.Nanosecond)
}

func prepareCheckConnectivity(endpoint config.Endpoint, cfg pkgconfigmodel.Reader) (*client.DestinationsContext, *Destination) {
	ctx := client.NewDestinationsContext()
	// Lower the timeout to 5s because HTTP connectivity test is done synchronously during the agent bootstrap sequence
	destination := newDestination(endpoint, JSONContentType, ctx, time.Second*5, false, client.NewNoopDestinationMetadata(), cfg, 1, 1, metrics.NewNoopPipelineMonitor(""), "")

	return ctx, destination
}

func completeCheckConnectivity(ctx *client.DestinationsContext, destination *Destination) error {
	ctx.Start()
	defer ctx.Stop()
	return destination.unconditionalSend(&emptyJSONPayload)
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

// CheckConnectivityDiagnose checks HTTP connectivity to an endpoint and returns the URL and any errors for diagnostic purposes
func CheckConnectivityDiagnose(endpoint config.Endpoint, cfg pkgconfigmodel.Reader) (url string, err error) {
	ctx, destination := prepareCheckConnectivity(endpoint, cfg)
	return destination.url, completeCheckConnectivity(ctx, destination)
}

func (d *Destination) waitForBackoff(blockedUntil time.Time) {
	ctx, cancel := context.WithDeadline(d.destinationsContext.Context(), blockedUntil)
	defer cancel()
	<-ctx.Done()
}
