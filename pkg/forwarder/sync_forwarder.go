package forwarder

import (
	"context"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	utilhttp "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SyncForwarder is a very simple Forwarder synchronously sending
// the data to the intake.
type SyncForwarder struct {
	defaultForwarder *DefaultForwarder
	client           *http.Client
}

// NewSyncForwarder returns a new synchronous forwarder.
func NewSyncForwarder(keysPerDomains map[string][]string, timeout time.Duration) *SyncForwarder {
	return &SyncForwarder{
		defaultForwarder: NewDefaultForwarder(NewOptions(keysPerDomains)),
		client: &http.Client{
			Timeout:   timeout,
			Transport: utilhttp.CreateHTTPTransport(),
		},
	}
}

// Start starts the sync forwarder: nothing to do.
func (f *SyncForwarder) Start() error {
	return nil
}

// Stop stops the sync forwarder: nothing to do.
func (f *SyncForwarder) Stop() {
}

func (f *SyncForwarder) sendHTTPTransactions(transactions []*transaction.HTTPTransaction) error {
	for _, t := range transactions {
		if err := t.Process(context.Background(), f.client); err != nil {
			log.Debugf("SyncForwarder.sendHTTPTransactions first attempt: %s", err)
			// Retry once after error
			log.Debug("Retrying transaction")
			if err := t.Process(context.Background(), f.client); err != nil {
				log.Errorf("SyncForwarder.sendHTTPTransactions final attempt: %s", err)
			}
		}
	}
	log.Debugf("SyncForwarder has flushed %d transactions", len(transactions))
	return nil
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *SyncForwarder) SubmitV1Series(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1SeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *SyncForwarder) SubmitV1Intake(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1IntakeEndpoint, payload, true, extra)
	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *SyncForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSeries will send a series type payload to Datadog backend.
func (f *SyncForwarder) SubmitSeries(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(seriesEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvents will send an event type payload to Datadog backend.
func (f *SyncForwarder) SubmitEvents(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(eventsEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitServiceChecks will send a service check type payload to Datadog backend.
func (f *SyncForwarder) SubmitServiceChecks(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(serviceChecksEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *SyncForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *SyncForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error {
	return f.SubmitV1Intake(payload, extra)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *SyncForwarder) SubmitMetadata(payload Payloads, extra http.Header) error {
	return f.SubmitV1Intake(payload, extra)
}

// SubmitAgentChecksMetadata will send a agentchecks_metadata tag type payload to Datadog backend.
func (f *SyncForwarder) SubmitAgentChecksMetadata(payload Payloads, extra http.Header) error {
	return f.SubmitV1Intake(payload, extra)
}

// SubmitProcessChecks sends process checks
func (f *SyncForwarder) SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(processesEndpoint, payload, extra, true)
}

// SubmitRTProcessChecks sends real time process checks
func (f *SyncForwarder) SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(rtProcessesEndpoint, payload, extra, false)
}

// SubmitContainerChecks sends container checks
func (f *SyncForwarder) SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(containerEndpoint, payload, extra, true)
}

// SubmitRTContainerChecks sends real time container checks
func (f *SyncForwarder) SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(rtContainerEndpoint, payload, extra, false)
}

// SubmitConnectionChecks sends connection checks
func (f *SyncForwarder) SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(connectionsEndpoint, payload, extra, true)
}

// SubmitOrchestratorChecks sends orchestrator checks
func (f *SyncForwarder) SubmitOrchestratorChecks(payload Payloads, extra http.Header, payloadType string) (chan Response, error) {
	return f.defaultForwarder.SubmitOrchestratorChecks(payload, extra, payloadType)
}
