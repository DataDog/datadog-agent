// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transaction defines the transaction of the forwarder
package transaction

import (
	"bytes"
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	// ForwarderExpvars is the root for expvars in the forwarder.
	ForwarderExpvars = expvar.NewMap("forwarder")

	// TransactionsExpvars the transactions Expvars
	TransactionsExpvars = expvar.Map{}

	connectionDNSSuccess         = expvar.Int{}
	connectionConnectSuccess     = expvar.Int{}
	transactionsConnectionEvents = expvar.Map{}

	// TransactionsDropped is the number of transaction dropped.
	TransactionsDropped = expvar.Int{}

	// TransactionsDroppedByEndpoint is the number of transaction dropped by endpoint.
	TransactionsDroppedByEndpoint = expvar.Map{}

	// TransactionsSuccessByEndpoint is the number of transaction succeeded by endpoint.
	TransactionsSuccessByEndpoint = expvar.Map{}

	transactionsSuccessBytesByEndpoint = expvar.Map{}
	transactionsSuccess                = expvar.Int{}
	transactionsErrors                 = expvar.Int{}
	transactionsErrorsByType           = expvar.Map{}
	transactionsDNSErrors              = expvar.Int{}
	transactionsTLSErrors              = expvar.Int{}
	transactionsConnectionErrors       = expvar.Int{}
	transactionsWroteRequestErrors     = expvar.Int{}
	transactionsSentRequestErrors      = expvar.Int{}
	transactionsHTTPErrors             = expvar.Int{}
	transactionsHTTPErrorsByCode       = expvar.Map{}

	tlmConnectEvents = telemetry.NewCounter("transactions", "connection_events",
		[]string{"connection_event_type"}, "Count of new connection events grouped by type of event")

	// TlmTxDropped is a telemetry counter that counts the number transaction dropped.
	TlmTxDropped = telemetry.NewCounter("transactions", "dropped",
		[]string{"domain", "endpoint"}, "Transaction drop count")
	tlmTxSuccessCount = telemetry.NewCounter("transactions", "success",
		[]string{"domain", "endpoint"}, "Successful transaction count")
	tlmTxSuccessBytes = telemetry.NewCounter("transactions", "success_bytes",
		[]string{"domain", "endpoint"}, "Successful transaction sizes in bytes")
	tlmTxErrors = telemetry.NewCounter("transactions", "errors",
		[]string{"domain", "endpoint", "error_type"}, "Count of transactions errored grouped by type of error")
	tlmTxHTTPErrors = telemetry.NewCounter("transactions", "http_errors",
		[]string{"domain", "endpoint", "code"}, "Count of transactions http errors per http code")
)

var trace *httptrace.ClientTrace
var traceOnce sync.Once

// GetClientTrace is an httptrace.ClientTrace instance that traces the events within HTTP client requests.
func GetClientTrace(log log.Component) *httptrace.ClientTrace {
	traceOnce.Do(func() {
		trace = &httptrace.ClientTrace{
			DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
				if dnsInfo.Err != nil {
					transactionsDNSErrors.Add(1)
					tlmTxErrors.Inc("unknown", "unknown", "dns_lookup_failure")
					log.Debugf("DNS Lookup failure: %s", dnsInfo.Err)
					return
				}
				connectionDNSSuccess.Add(1)
				tlmConnectEvents.Inc("dns_lookup_success")
				log.Tracef("DNS Lookup success, addresses: %s", dnsInfo.Addrs)
			},
			WroteRequest: func(wroteInfo httptrace.WroteRequestInfo) {
				if wroteInfo.Err != nil {
					transactionsWroteRequestErrors.Add(1)
					tlmTxErrors.Inc("unknown", "unknown", "writing_failure")
					log.Debugf("Request writing failure: %s", wroteInfo.Err)
				}
			},
			ConnectDone: func(_, addr string, err error) {
				if err != nil {
					transactionsConnectionErrors.Add(1)
					tlmTxErrors.Inc("unknown", "unknown", "connection_failure")
					log.Debugf("Connection failure: %s", err)
					return
				}
				connectionConnectSuccess.Add(1)
				tlmConnectEvents.Inc("connection_success")
				log.Tracef("New successful connection to address: %q", addr)
			},
			TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
				if err != nil {
					transactionsTLSErrors.Add(1)
					tlmTxErrors.Inc("unknown", "unknown", "tls_handshake_failure")
					log.Errorf("TLS Handshake failure: %s", err)
				}
			},
		}
	})
	return trace
}

// Compile-time check to ensure that HTTPTransaction conforms to the Transaction interface
var _ Transaction = &HTTPTransaction{}

// HTTPAttemptHandler is an event handler that will get called each time this transaction is attempted
type HTTPAttemptHandler func(transaction *HTTPTransaction)

// HTTPCompletionHandler is an  event handler that will get called after this transaction has completed
type HTTPCompletionHandler func(transaction *HTTPTransaction, statusCode int, body []byte, err error)

var defaultAttemptHandler = func(_ *HTTPTransaction) {}
var defaultCompletionHandler = func(_ *HTTPTransaction, _ int, _ []byte, _ error) {}

func init() {
	TransactionsExpvars.Init()
	transactionsConnectionEvents.Init()
	TransactionsDroppedByEndpoint.Init()
	TransactionsSuccessByEndpoint.Init()
	transactionsSuccessBytesByEndpoint.Init()
	transactionsErrorsByType.Init()
	transactionsHTTPErrorsByCode.Init()
	ForwarderExpvars.Set("Transactions", &TransactionsExpvars)
	transactionsConnectionEvents.Set("DNSSuccess", &connectionDNSSuccess)
	transactionsConnectionEvents.Set("ConnectSuccess", &connectionConnectSuccess)
	TransactionsExpvars.Set("ConnectionEvents", &transactionsConnectionEvents)
	TransactionsExpvars.Set("Dropped", &TransactionsDropped)
	TransactionsExpvars.Set("DroppedByEndpoint", &TransactionsDroppedByEndpoint)
	TransactionsExpvars.Set("SuccessByEndpoint", &TransactionsSuccessByEndpoint)
	TransactionsExpvars.Set("SuccessBytesByEndpoint", &transactionsSuccessBytesByEndpoint)
	TransactionsExpvars.Set("Success", &transactionsSuccess)
	TransactionsExpvars.Set("Errors", &transactionsErrors)
	TransactionsExpvars.Set("ErrorsByType", &transactionsErrorsByType)
	transactionsErrorsByType.Set("DNSErrors", &transactionsDNSErrors)
	transactionsErrorsByType.Set("TLSErrors", &transactionsTLSErrors)
	transactionsErrorsByType.Set("ConnectionErrors", &transactionsConnectionErrors)
	transactionsErrorsByType.Set("WroteRequestErrors", &transactionsWroteRequestErrors)
	transactionsErrorsByType.Set("SentRequestErrors", &transactionsSentRequestErrors)
	TransactionsExpvars.Set("HTTPErrors", &transactionsHTTPErrors)
	TransactionsExpvars.Set("HTTPErrorsByCode", &transactionsHTTPErrorsByCode)
}

// Priority defines the priority of a transaction
// Transactions with priority `TransactionPriorityNormal` are dropped from the retry queue
// before dropping transactions with priority `TransactionPriorityHigh`.
type Priority int

const (
	// TransactionPriorityNormal defines a transaction with a normal priority
	TransactionPriorityNormal Priority = iota

	// TransactionPriorityHigh defines a transaction with an high priority
	TransactionPriorityHigh Priority = iota
)

// Kind defines de kind of transaction (metrics, metadata, process, ...)
type Kind int

const (
	// Series is the transaction type for metrics series
	Series = iota
	// Sketches is the transaction type for distribution sketches
	Sketches
	// ServiceChecks is the transaction type for service checks
	ServiceChecks
	// Events is the transaction type for events
	Events
	// CheckRuns is the transaction type for agent check runs
	CheckRuns
	// Metadata is the transaction type for metadata payloads
	Metadata
	// Process is the transaction type for live-process monitoring payloads
	Process
)

// Destination indicates which regions the transaction should be sent to
type Destination int

const (
	// AllRegions indicates the transaction should be sent to all regions (default behavior)
	AllRegions = iota
	// PrimaryOnly indicates the transaction should be sent to the primary region during MRF
	PrimaryOnly
	// SecondaryOnly indicates the transaction should be sent to the secondary region during MRF
	SecondaryOnly
	// LocalOnly indicates the transaction should be sent to the local endpoint (cluster-agent) only
	LocalOnly
)

func (d Destination) String() string {
	switch d {
	case AllRegions:
		return "AllRegions"
	case PrimaryOnly:
		return "PrimaryOnly"
	case SecondaryOnly:
		return "SecondaryOnly"
	case LocalOnly:
		return "LocalOnly"
	default:
		return "Unknown"
	}
}

// HTTPTransaction represents one Payload for one Endpoint on one Domain.
type HTTPTransaction struct {
	// Domain represents the domain target by the HTTPTransaction.
	Domain string
	// Endpoint is the API Endpoint used by the HTTPTransaction.
	Endpoint Endpoint
	// Headers are the HTTP headers used by the HTTPTransaction.
	Headers http.Header
	// Payload is the content delivered to the backend.
	Payload *BytesPayload
	// ErrorCount is the number of times this HTTPTransaction failed to be processed.
	ErrorCount int

	CreatedAt time.Time
	// Retryable indicates whether this transaction can be retried
	Retryable bool

	// StorableOnDisk indicates whether this transaction can be stored on disk
	StorableOnDisk bool

	// AttemptHandler will be called with a transaction before the attempting to send the request
	// This field is not restored when a transaction is deserialized from the disk (the default value is used).
	AttemptHandler HTTPAttemptHandler
	// CompletionHandler will be called with a transaction after it has been successfully sent
	// This field is not restored when a transaction is deserialized from the disk (the default value is used).
	CompletionHandler HTTPCompletionHandler

	Priority Priority

	Kind Kind

	Destination Destination
}

// TransactionsSerializer serializes Transaction instances.
type TransactionsSerializer interface {
	Add(transaction *HTTPTransaction) error
}

// Transaction represents the task to process for a Worker.
type Transaction interface {
	Process(ctx context.Context, config config.Component, log log.Component, client *http.Client) error
	GetCreatedAt() time.Time
	GetTarget() string
	GetPriority() Priority
	GetKind() Kind
	GetEndpointName() string
	GetPayloadSize() int
	GetPointCount() int
	GetDestination() Destination

	// This method serializes the transaction to `TransactionsSerializer`.
	// It forces a new implementation of `Transaction` to define how to
	// serialize the transaction to `TransactionsSerializer` as a `Transaction`
	// must be serializable in domainForwarder.
	SerializeTo(log.Component, TransactionsSerializer) error
}

// NewHTTPTransaction returns a new HTTPTransaction.
func NewHTTPTransaction() *HTTPTransaction {
	tr := &HTTPTransaction{
		CreatedAt:      time.Now(),
		ErrorCount:     0,
		Retryable:      true,
		StorableOnDisk: true,
		Headers:        make(http.Header),
	}
	tr.SetDefaultHandlers()
	return tr
}

// SetDefaultHandlers sets the default handlers for AttemptHandler and CompletionHandler
func (t *HTTPTransaction) SetDefaultHandlers() {
	t.AttemptHandler = defaultAttemptHandler
	t.CompletionHandler = defaultCompletionHandler
}

// GetCreatedAt returns the creation time of the HTTPTransaction.
func (t *HTTPTransaction) GetCreatedAt() time.Time {
	return t.CreatedAt
}

// GetTarget return the url used by the transaction
func (t *HTTPTransaction) GetTarget() string {
	url := t.Domain + t.Endpoint.Route
	return scrubber.ScrubLine(url) // sanitized url that can be logged
}

// GetPriority returns the priority
func (t *HTTPTransaction) GetPriority() Priority {
	return t.Priority
}

// GetKind returns the transaction kind
func (t *HTTPTransaction) GetKind() Kind {
	return t.Kind
}

// GetEndpointName returns the name of the endpoint used by the transaction
func (t *HTTPTransaction) GetEndpointName() string {
	return t.Endpoint.Name
}

// GetPayloadSize returns the size of the payload.
func (t *HTTPTransaction) GetPayloadSize() int {
	if t.Payload != nil {
		return t.Payload.Len()
	}

	return 0
}

// GetPointCount gets the number of points in the payload of this transaction.
func (t *HTTPTransaction) GetPointCount() int {
	if t.Payload != nil {
		return t.Payload.GetPointCount()
	}
	return 0
}

// GetDestination returns the region(s) this transaction should be sent to.
func (t *HTTPTransaction) GetDestination() Destination {
	return t.Destination
}

// Process sends the Payload of the transaction to the right Endpoint and Domain.
func (t *HTTPTransaction) Process(ctx context.Context, config config.Component, log log.Component, client *http.Client) error {
	t.AttemptHandler(t)

	statusCode, body, err := t.internalProcess(ctx, config, log, client)

	if err == nil || !t.Retryable {
		t.CompletionHandler(t, statusCode, body, err)
	}

	// If the txn is retryable, return the error (if present) to the worker to allow it to be retried
	// Otherwise, return nil so the txn won't be retried.
	if t.Retryable {
		return err
	}

	return nil
}

// internalProcess does the  work of actually sending the http request to the specified domain
// This will return  (http status code, response body, error).
func (t *HTTPTransaction) internalProcess(ctx context.Context, config config.Component, log log.Component, client *http.Client) (int, []byte, error) {
	payload := t.Payload.GetContent()
	reader := bytes.NewReader(payload)
	url := t.Domain + t.Endpoint.Route
	transactionEndpointName := t.GetEndpointName()
	logURL := scrubber.ScrubLine(url) // sanitized url that can be logged

	req, err := http.NewRequestWithContext(ctx, "POST", url, reader)
	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL %q (dropping transaction): %s", logURL, err)
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, transactionEndpointName, "invalid_request")
		transactionsSentRequestErrors.Add(1)
		return 0, nil, nil
	}
	req.Header = t.Headers
	log.Tracef("Sending %s request to %s with body size %d and headers %v", req.Method, logURL, len(payload), req.Header)
	resp, err := client.Do(req)

	if err != nil {
		// Do not requeue transaction if that one was canceled
		if ctx.Err() == context.Canceled {
			return 0, nil, nil
		}
		t.ErrorCount++
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, transactionEndpointName, "cant_send")
		return 0, nil, fmt.Errorf("error while sending transaction, rescheduling it: %s", scrubber.ScrubLine(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Fail to read the response Body: %s", err)
		return 0, nil, err
	}

	if resp.StatusCode >= 400 {
		statusCode := strconv.Itoa(resp.StatusCode)
		var codeCount *expvar.Int
		if count := transactionsHTTPErrorsByCode.Get(statusCode); count == nil {
			codeCount = &expvar.Int{}
			transactionsHTTPErrorsByCode.Set(statusCode, codeCount)
		} else {
			codeCount = count.(*expvar.Int)
		}
		codeCount.Add(1)
		transactionsHTTPErrors.Add(1)
		tlmTxHTTPErrors.Inc(t.Domain, transactionEndpointName, statusCode)
	}

	// We want to retry 404s even if that means that the agent would retry
	// payloads on endpoints that don’t exist at the intake it’s sending data
	// to (example: a specific DD region, or a http proxy)
	if resp.StatusCode == 400 || resp.StatusCode == 413 {
		log.Errorf("Error code %q received while sending transaction to %q: %q, dropping it", resp.Status, logURL, truncateBodyForLog(body))
		TransactionsDroppedByEndpoint.Add(transactionEndpointName, 1)
		TransactionsDropped.Add(1)
		TlmTxDropped.Inc(t.Domain, transactionEndpointName)
		return resp.StatusCode, body, nil
	} else if resp.StatusCode == 403 {
		log.Errorf("API Key invalid, dropping transaction for %s", logURL)
		TransactionsDroppedByEndpoint.Add(transactionEndpointName, 1)
		TransactionsDropped.Add(1)
		TlmTxDropped.Inc(t.Domain, transactionEndpointName)
		return resp.StatusCode, body, nil
	} else if resp.StatusCode > 400 {
		t.ErrorCount++
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, transactionEndpointName, "gt_400")
		return resp.StatusCode, body, fmt.Errorf("error %q while sending transaction to %q, rescheduling it: %q", resp.Status, logURL, truncateBodyForLog(body))
	}

	tlmTxSuccessCount.Inc(t.Domain, transactionEndpointName)
	tlmTxSuccessBytes.Add(float64(t.GetPayloadSize()), t.Domain, transactionEndpointName)
	TransactionsSuccessByEndpoint.Add(transactionEndpointName, 1)
	transactionsSuccessBytesByEndpoint.Add(transactionEndpointName, int64(t.GetPayloadSize()))
	transactionsSuccess.Add(1)

	loggingFrequency := config.GetInt64("logging_frequency")

	if transactionsSuccess.Value() == 1 {
		log.Infof("Successfully posted payload to %q (%s), the agent will only log transaction success every %d transactions", logURL, resp.Status, loggingFrequency)
		log.Tracef("Url: %q, response status %s, content length %d, payload: %q", logURL, resp.Status, resp.ContentLength, truncateBodyForLog(body))
		return resp.StatusCode, body, nil
	}
	if transactionsSuccess.Value()%loggingFrequency == 0 {
		log.Infof("Successfully posted payload to %q (%s)", logURL, resp.Status)
		log.Tracef("Url: %q, response status %s, content length %d, payload: %q", logURL, resp.Status, resp.ContentLength, truncateBodyForLog(body))
		return resp.StatusCode, body, nil
	}
	log.Tracef("Successfully posted payload to %q (%s): %q", logURL, resp.Status, truncateBodyForLog(body))
	return resp.StatusCode, body, nil
}

// SerializeTo serializes the transaction using TransactionsSerializer
func (t *HTTPTransaction) SerializeTo(log log.Component, serializer TransactionsSerializer) error {
	if t.StorableOnDisk {
		return serializer.Add(t)
	}
	log.Trace("The transaction is not stored on disk because `storableOnDisk` is false.")
	return nil
}

// truncateBodyForLog truncates body to prevent from logging a huge message
// if body is more than 1000 bytes. 1000 bytes are enough to tell which tools a response comes.
func truncateBodyForLog(body []byte) []byte {
	if len(body) == 0 {
		return []byte{}
	}
	limit := 1000
	if len(body) < limit {
		return body
	}
	return append(body[:limit], []byte("...")...)
}
