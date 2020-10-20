package forwarder

import (
	"context"
	"net/http"
	"time"

	utilhttp "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SyncDefaultForwarder is a very simple Forwarder synchronously sending
// the data to the intake.
// It doesn't ship any retry mechanism for now.
type SyncDefaultForwarder struct {
	defaultForwarder *DefaultForwarder
	client           *http.Client
}

// NewSyncDefaultForwarder returns a new synchronous forwarder.
func NewSyncDefaultForwarder(keysPerDomains map[string][]string, timeout time.Duration) *SyncDefaultForwarder {
	return &SyncDefaultForwarder{
		defaultForwarder: NewDefaultForwarder(NewOptions(keysPerDomains)),
		client: &http.Client{
			Timeout:   timeout,
			Transport: utilhttp.CreateHTTPTransport(),
		},
	}
}

// Start starts the sync forwarder: nothing to do.
func (f *SyncDefaultForwarder) Start() error {
	return nil
}

// Stop stops the sync forwarder: nothing to do.
func (f *SyncDefaultForwarder) Stop() {
}

func (f *SyncDefaultForwarder) sendHTTPTransactions(transactions []*HTTPTransaction) error {
	for _, t := range transactions {
		t.Process(context.Background(), f.client)
	}
	log.Debugf("SyncDefaultForwarder has flushed %d transactions", len(transactions))
	return nil
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *SyncDefaultForwarder) SubmitV1Series(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1SeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *SyncDefaultForwarder) SubmitV1Intake(payload Payloads, extra http.Header, priority TransactionPriority) error {
	transactions := f.defaultForwarder.createPriorityHTTPTransactions(v1IntakeEndpoint, payload, true, extra, priority)
	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *SyncDefaultForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSeries will send a series type payload to Datadog backend.
func (f *SyncDefaultForwarder) SubmitSeries(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(seriesEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvents will send an event type payload to Datadog backend.
func (f *SyncDefaultForwarder) SubmitEvents(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(eventsEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitServiceChecks will send a service check type payload to Datadog backend.
func (f *SyncDefaultForwarder) SubmitServiceChecks(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(serviceChecksEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *SyncDefaultForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *SyncDefaultForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(hostMetadataEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *SyncDefaultForwarder) SubmitMetadata(payload Payloads, extra http.Header, priority TransactionPriority) error {
	transactions := f.defaultForwarder.createPriorityHTTPTransactions(metadataEndpoint, payload, false, extra, priority)
	return f.sendHTTPTransactions(transactions)
}

// SubmitProcessChecks sends process checks
func (f *SyncDefaultForwarder) SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(processesEndpoint, payload, extra, true)
}

// SubmitRTProcessChecks sends real time process checks
func (f *SyncDefaultForwarder) SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(rtProcessesEndpoint, payload, extra, false)
}

// SubmitContainerChecks sends container checks
func (f *SyncDefaultForwarder) SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(containerEndpoint, payload, extra, true)
}

// SubmitRTContainerChecks sends real time container checks
func (f *SyncDefaultForwarder) SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(rtContainerEndpoint, payload, extra, false)
}

// SubmitConnectionChecks sends connection checks
func (f *SyncDefaultForwarder) SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(connectionsEndpoint, payload, extra, true)
}

// SubmitOrchestratorChecks sends orchestrator checks
func (f *SyncDefaultForwarder) SubmitOrchestratorChecks(payload Payloads, extra http.Header, payloadType string) (chan Response, error) {
	return f.defaultForwarder.SubmitOrchestratorChecks(payload, extra, payloadType)
}
