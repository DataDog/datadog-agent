// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"bytes"
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	connectionEvents               = expvar.Map{}
	connectionDNSSuccess           = expvar.Int{}
	connectionConnectSuccess       = expvar.Int{}
	transactionsExpvars            = expvar.Map{}
	transactionsSuccessful         = expvar.Int{}
	transactionsSeries             = expvar.Int{}
	transactionsEvents             = expvar.Int{}
	transactionsServiceChecks      = expvar.Int{}
	transactionsSketchSeries       = expvar.Int{}
	transactionsHostMetadata       = expvar.Int{}
	transactionsMetadata           = expvar.Int{}
	transactionsTimeseriesV1       = expvar.Int{}
	transactionsCheckRunsV1        = expvar.Int{}
	transactionsIntakeV1           = expvar.Int{}
	transactionsIntakeProcesses    = expvar.Int{}
	transactionsIntakeRTProcesses  = expvar.Int{}
	transactionsIntakeContainer    = expvar.Int{}
	transactionsIntakeRTContainer  = expvar.Int{}
	transactionsIntakeConnections  = expvar.Int{}
	transactionsIntakePod          = expvar.Int{}
	transactionsIntakeDeployment   = expvar.Int{}
	transactionsIntakeReplicaSet   = expvar.Int{}
	transactionsIntakeService      = expvar.Int{}
	transactionsIntakeNode         = expvar.Int{}
	transactionsRetryQueueSize     = expvar.Int{}
	transactionsDroppedOnInput     = expvar.Int{}
	transactionsErrors             = expvar.Int{}
	transactionsErrorsByType       = expvar.Map{}
	transactionsDNSErrors          = expvar.Int{}
	transactionsTLSErrors          = expvar.Int{}
	transactionsConnectionErrors   = expvar.Int{}
	transactionsWroteRequestErrors = expvar.Int{}
	transactionsSentRequestErrors  = expvar.Int{}
	transactionsHTTPErrors         = expvar.Int{}
	transactionsHTTPErrorsByCode   = expvar.Map{}

	tlmConnectEvents = telemetry.NewCounter("forwarder_transactions", "connection_events",
		[]string{"connection_event_type"}, "Count of new connection events grouped by type of event")
	tlmTxCount = telemetry.NewCounter("forwarder_transactions", "count",
		[]string{"domain", "endpoint"}, "Incoming transaction count")
	tlmTxBytes = telemetry.NewCounter("forwarder_transactions", "bytes",
		[]string{"domain", "endpoint"}, "Incoming transaction sizes in bytes")
	tlmTxSuccessCount = telemetry.NewCounter("forwarder_transactions", "success",
		[]string{"domain", "endpoint"}, "Successful transaction count")
	tlmTxSuccessBytes = telemetry.NewCounter("forwarder_transactions", "success_bytes",
		[]string{"domain", "endpoint"}, "Successful transaction sizes in bytes")
	tlmTxRetryQueueSize = telemetry.NewGauge("forwarder_transactions", "retry_queue_size",
		[]string{"domain"}, "Retry queue size")
	tlmTxDroppedOnInput = telemetry.NewCounter("forwarder_transactions", "dropped_on_input",
		[]string{"domain", "endpoint"}, "Count of transactions dropped on input")
	tlmTxErrors = telemetry.NewCounter("forwarder_transactions", "errors",
		[]string{"domain", "endpoint", "error_type"}, "Count of transactions errored grouped by type of error")
	tlmTxHTTPErrors = telemetry.NewCounter("forwarder_transactions", "http_errors",
		[]string{"domain", "endpoint", "code"}, "Count of transactions http errors per http code")
)

var trace = &httptrace.ClientTrace{
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
	ConnectDone: func(network, addr string, err error) {
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
	TLSHandshakeDone: func(tlsState tls.ConnectionState, err error) {
		if err != nil {
			transactionsTLSErrors.Add(1)
			tlmTxErrors.Inc("unknown", "unknown", "tls_handshake_failure")
			log.Errorf("TLS Handshake failure: %s", err)
		}
	},
}

// Compile-time check to ensure that HTTPTransaction conforms to the Transaction interface
var _ Transaction = &HTTPTransaction{}

// HTTPAttemptHandler is an event handler that will get called each time this transaction is attempted
type HTTPAttemptHandler func(transaction *HTTPTransaction)

// HTTPCompletionHandler is an  event handler that will get called after this transaction has completed
type HTTPCompletionHandler func(transaction *HTTPTransaction, statusCode int, body []byte, err error)

var defaultAttemptHandler = func(transaction *HTTPTransaction) {}
var defaultCompletionHandler = func(transaction *HTTPTransaction, statusCode int, body []byte, err error) {}

func initTransactionExpvars() {
	connectionEvents.Init()
	transactionsErrorsByType.Init()
	transactionsHTTPErrorsByCode.Init()
	connectionEvents.Set("DNSSuccess", &connectionDNSSuccess)
	connectionEvents.Set("ConnectSuccess", &connectionConnectSuccess)
	transactionsExpvars.Set("ConnectionEvents", &connectionEvents)
	transactionsExpvars.Set("Success", &transactionsSuccessful)
	transactionsExpvars.Set("Series", &transactionsSeries)
	transactionsExpvars.Set("Events", &transactionsEvents)
	transactionsExpvars.Set("ServiceChecks", &transactionsServiceChecks)
	transactionsExpvars.Set("SketchSeries", &transactionsSketchSeries)
	transactionsExpvars.Set("HostMetadata", &transactionsHostMetadata)
	transactionsExpvars.Set("Metadata", &transactionsMetadata)
	transactionsExpvars.Set("TimeseriesV1", &transactionsTimeseriesV1)
	transactionsExpvars.Set("CheckRunsV1", &transactionsCheckRunsV1)
	transactionsExpvars.Set("IntakeV1", &transactionsIntakeV1)
	transactionsExpvars.Set("Processes", &transactionsIntakeProcesses)
	transactionsExpvars.Set("RTProcesses", &transactionsIntakeRTProcesses)
	transactionsExpvars.Set("Containers", &transactionsIntakeContainer)
	transactionsExpvars.Set("RTContainers", &transactionsIntakeRTContainer)
	transactionsExpvars.Set("Connections", &transactionsIntakeConnections)
	transactionsExpvars.Set("Pods", &transactionsIntakePod)
	transactionsExpvars.Set("Deployments", &transactionsIntakeDeployment)
	transactionsExpvars.Set("ReplicaSets", &transactionsIntakeReplicaSet)
	transactionsExpvars.Set("Services", &transactionsIntakeService)
	transactionsExpvars.Set("Nodes", &transactionsIntakeNode)
	transactionsExpvars.Set("RetryQueueSize", &transactionsRetryQueueSize)
	transactionsExpvars.Set("DroppedOnInput", &transactionsDroppedOnInput)
	transactionsExpvars.Set("HTTPErrors", &transactionsHTTPErrors)
	transactionsExpvars.Set("HTTPErrorsByCode", &transactionsHTTPErrorsByCode)
	transactionsExpvars.Set("Errors", &transactionsErrors)
	transactionsExpvars.Set("ErrorsByType", &transactionsErrorsByType)
	transactionsErrorsByType.Set("DNSErrors", &transactionsDNSErrors)
	transactionsErrorsByType.Set("TLSErrors", &transactionsTLSErrors)
	transactionsErrorsByType.Set("ConnectionErrors", &transactionsConnectionErrors)
	transactionsErrorsByType.Set("WroteRequestErrors", &transactionsWroteRequestErrors)
	transactionsErrorsByType.Set("SentRequestErrors", &transactionsSentRequestErrors)
}

// TransactionPriority defines the priority of a transaction
// Transactions with priority `TransactionPriorityNormal` are dropped from the retry queue
// before dropping transactions with priority `TransactionPriorityHigh`.
type TransactionPriority int

const (
	// TransactionPriorityNormal defines a transaction with a normal priority
	TransactionPriorityNormal TransactionPriority = 0

	// TransactionPriorityHigh defines a transaction with an high priority
	TransactionPriorityHigh TransactionPriority = 1
)

// HTTPTransaction represents one Payload for one Endpoint on one Domain.
type HTTPTransaction struct {
	// Domain represents the domain target by the HTTPTransaction.
	Domain string
	// Endpoint is the API Endpoint used by the HTTPTransaction.
	Endpoint string
	// Headers are the HTTP headers used by the HTTPTransaction.
	Headers http.Header
	// Payload is the content delivered to the backend.
	Payload *[]byte
	// ErrorCount is the number of times this HTTPTransaction failed to be processed.
	ErrorCount int

	createdAt time.Time
	// retryable indicates whether this transaction can be retried
	retryable bool

	// attemptHandler will be called with a transaction before the attempting to send the request
	attemptHandler HTTPAttemptHandler
	// completionHandler will be called with a transaction after it has been successfully sent
	completionHandler HTTPCompletionHandler

	priority     TransactionPriority
	endpointName string
}

// Transaction represents the task to process for a Worker.
type Transaction interface {
	Process(ctx context.Context, client *http.Client) error
	GetCreatedAt() time.Time
	GetTarget() string
	GetPriority() TransactionPriority
	GetEndpointName() string
	GetPayloadSize() int
}

// NewHTTPTransaction returns a new HTTPTransaction.
func NewHTTPTransaction() *HTTPTransaction {
	return &HTTPTransaction{
		createdAt:         time.Now(),
		ErrorCount:        0,
		retryable:         true,
		Headers:           make(http.Header),
		attemptHandler:    defaultAttemptHandler,
		completionHandler: defaultCompletionHandler,
	}
}

// GetCreatedAt returns the creation time of the HTTPTransaction.
func (t *HTTPTransaction) GetCreatedAt() time.Time {
	return t.createdAt
}

// GetTarget return the url used by the transaction
func (t *HTTPTransaction) GetTarget() string {
	url := t.Domain + t.Endpoint
	return httputils.SanitizeURL(url) // sanitized url that can be logged
}

// GetPriority returns the priority
func (t *HTTPTransaction) GetPriority() TransactionPriority {
	return t.priority
}

// GetEndpointName returns the name of the endpoint used by the transaction
func (t *HTTPTransaction) GetEndpointName() string {
	return t.endpointName
}

// GetPayloadSize returns the size of the payload.
func (t *HTTPTransaction) GetPayloadSize() int {
	if t.Payload != nil {
		return len(*t.Payload)
	}

	return 0
}

// Process sends the Payload of the transaction to the right Endpoint and Domain.
func (t *HTTPTransaction) Process(ctx context.Context, client *http.Client) error {
	t.attemptHandler(t)

	statusCode, body, err := t.internalProcess(ctx, client)

	if err == nil || !t.retryable {
		t.completionHandler(t, statusCode, body, err)
	}

	// If the txn is retryable, return the error (if present) to the worker to allow it to be retried
	// Otherwise, return nil so the txn won't be retried.
	if t.retryable {
		return err
	}

	return nil
}

// internalProcess does the  work of actually sending the http request to the specified domain
// This will return  (http status code, response body, error).
func (t *HTTPTransaction) internalProcess(ctx context.Context, client *http.Client) (int, []byte, error) {
	reader := bytes.NewReader(*t.Payload)
	url := t.Domain + t.Endpoint
	logURL := httputils.SanitizeURL(url) // sanitized url that can be logged

	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL %q (dropping transaction): %s", logURL, err)
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, getTransactionEndpointName(t), "invalid_request")
		transactionsSentRequestErrors.Add(1)
		return 0, nil, nil
	}
	req = req.WithContext(ctx)
	req.Header = t.Headers
	resp, err := client.Do(req)

	if err != nil {
		// Do not requeue transaction if that one was canceled
		if ctx.Err() == context.Canceled {
			return 0, nil, nil
		}
		t.ErrorCount++
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, getTransactionEndpointName(t), "cant_send")
		return 0, nil, fmt.Errorf("error while sending transaction, rescheduling it: %s", httputils.SanitizeURL(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := ioutil.ReadAll(resp.Body)
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
		tlmTxHTTPErrors.Inc(t.Domain, getTransactionEndpointName(t), statusCode)
	}

	if resp.StatusCode == 400 || resp.StatusCode == 404 || resp.StatusCode == 413 {
		log.Errorf("Error code %q received while sending transaction to %q: %s, dropping it", resp.Status, logURL, string(body))
		transactionsDropped.Add(1)
		tlmTxDropped.Inc(t.Domain, getTransactionEndpointName(t))
		return resp.StatusCode, body, nil
	} else if resp.StatusCode == 403 {
		log.Errorf("API Key invalid, dropping transaction for %s", logURL)
		transactionsDropped.Add(1)
		tlmTxDropped.Inc(t.Domain, getTransactionEndpointName(t))
		return resp.StatusCode, body, nil
	} else if resp.StatusCode > 400 {
		t.ErrorCount++
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, getTransactionEndpointName(t), "gt_400")
		return resp.StatusCode, body, fmt.Errorf("error %q while sending transaction to %q, rescheduling it", resp.Status, logURL)
	}

	transactionsSuccessful.Add(1)
	tlmTxSuccessCount.Inc(t.Domain, getTransactionEndpointName(t))
	tlmTxSuccessBytes.Add(float64(t.GetPayloadSize()), t.Domain, getTransactionEndpointName(t))

	loggingFrequency := config.Datadog.GetInt64("logging_frequency")

	if transactionsSuccessful.Value() == 1 {
		log.Infof("Successfully posted payload to %q, the agent will only log transaction success every %d transactions", logURL, loggingFrequency)
		log.Tracef("Url: %q payload: %s", logURL, string(body))
		return resp.StatusCode, body, nil
	}
	if transactionsSuccessful.Value()%loggingFrequency == 0 {
		log.Infof("Successfully posted payload to %q", logURL)
		log.Tracef("Url: %q payload: %s", logURL, string(body))
		return resp.StatusCode, body, nil
	}
	log.Tracef("Successfully posted payload to %q: %s", logURL, string(body))
	return resp.StatusCode, body, nil
}

func getTransactionEndpointName(transaction Transaction) string {
	if transaction != nil {
		return transaction.GetEndpointName()
	}

	return "unknown"
}
