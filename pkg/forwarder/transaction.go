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
	connectionDNSSuccess           = expvar.Int{}
	connectionConnectSuccess       = expvar.Int{}
	transactionsRetryQueueSize     = expvar.Int{}
	transactionsSuccessful         = expvar.Int{}
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

	tlmConnectEvents = telemetry.NewCounter("forwarder", "connection_events",
		[]string{"connection_event_type"}, "Count of new connection events grouped by type of event")
	tlmTxRetryQueueSize = telemetry.NewGauge("transactions", "retry_queue_size",
		[]string{"domain"}, "Retry queue size")
	tlmTxSuccess = telemetry.NewCounter("transactions", "success",
		[]string{"domain"}, "Count of successful transactions")
	tlmTxDroppedOnInput = telemetry.NewCounter("transactions", "dropped_on_input",
		[]string{"domain"}, "Count of transactions dropped on input")
	tlmTxErrors = telemetry.NewCounter("transactions", "errors",
		[]string{"domain", "error_type"}, "Count of transactions errored grouped by type of error")
	tlmTxHTTPErrors = telemetry.NewCounter("transactions", "http_errors",
		[]string{"domain", "code"}, "Count of transactions http errors per http code")
)

var trace = &httptrace.ClientTrace{
	DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
		if dnsInfo.Err != nil {
			transactionsDNSErrors.Add(1)
			tlmTxErrors.Inc("unknown", "dns_lookup_failure")
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
			tlmTxErrors.Inc("unknown", "writing_failure")
			log.Debugf("Request writing failure: %s", wroteInfo.Err)
		}
	},
	ConnectDone: func(network, addr string, err error) {
		if err != nil {
			transactionsConnectionErrors.Add(1)
			tlmTxErrors.Inc("unknown", "connection_failure")
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
			tlmTxErrors.Inc("unknown", "tls_handshake_failure")
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
	transactionsErrorsByType.Init()
	transactionsHTTPErrorsByCode.Init()
	transactionsExpvars.Set("RetryQueueSize", &transactionsRetryQueueSize)
	transactionsExpvars.Set("Success", &transactionsSuccessful)
	transactionsExpvars.Set("DroppedOnInput", &transactionsDroppedOnInput)
	transactionsExpvars.Set("HTTPErrors", &transactionsHTTPErrors)
	transactionsExpvars.Set("HTTPErrorsByCode", &transactionsHTTPErrorsByCode)
	transactionsExpvars.Set("Errors", &transactionsErrors)
	transactionsExpvars.Set("ErrorsByType", &transactionsErrorsByType)
	connectionEvents.Set("DNSSuccess", &connectionDNSSuccess)
	connectionEvents.Set("ConnectSuccess", &connectionConnectSuccess)
	transactionsErrorsByType.Set("DNSErrors", &transactionsDNSErrors)
	transactionsErrorsByType.Set("TLSErrors", &transactionsTLSErrors)
	transactionsErrorsByType.Set("ConnectionErrors", &transactionsConnectionErrors)
	transactionsErrorsByType.Set("WroteRequestErrors", &transactionsWroteRequestErrors)
	transactionsErrorsByType.Set("SentRequestErrors", &transactionsSentRequestErrors)
}

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
}

// Transaction represents the task to process for a Worker.
type Transaction interface {
	Process(ctx context.Context, client *http.Client) error
	GetCreatedAt() time.Time
	GetTarget() string
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
		tlmTxErrors.Inc(t.Domain, "invalid_request")
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
		tlmTxErrors.Inc(t.Domain, "cant_send")
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
		tlmTxHTTPErrors.Inc(t.Domain, statusCode)
	}

	if resp.StatusCode == 400 || resp.StatusCode == 404 || resp.StatusCode == 413 {
		log.Errorf("Error code %q received while sending transaction to %q: %s, dropping it", resp.Status, logURL, string(body))
		transactionsDropped.Add(1)
		tlmTxDropped.Inc(t.Domain)
		return resp.StatusCode, body, nil
	} else if resp.StatusCode == 403 {
		log.Errorf("API Key invalid, dropping transaction for %s", logURL)
		transactionsDropped.Add(1)
		tlmTxDropped.Inc(t.Domain)
		return resp.StatusCode, body, nil
	} else if resp.StatusCode > 400 {
		t.ErrorCount++
		transactionsErrors.Add(1)
		tlmTxErrors.Inc(t.Domain, "gt_400")
		return resp.StatusCode, body, fmt.Errorf("error %q while sending transaction to %q, rescheduling it", resp.Status, logURL)
	}

	transactionsSuccessful.Add(1)
	tlmTxSuccess.Inc(t.Domain)

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
