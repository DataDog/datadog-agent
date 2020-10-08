package forwarder

import (
	"context"
	"net/http"
	"time"
)

// SyncDefaultForwarder is a very simple Forwarder synchronously sending
// the data to the intake.
// It doesn't ship any retry mechanism for now.
type SyncDefaultForwarder struct {
	defaultForwarder *DefaultForwarder
	client           *http.Client
}

func NewSyncDefaultForwarder(keysPerDomains map[string][]string, timeout time.Duration) *SyncDefaultForwarder {
	return &SyncDefaultForwarder{
		defaultForwarder: NewDefaultForwarder(NewOptions(keysPerDomains)),
		client:           &http.Client{Timeout: timeout},
	}
}

func (f *SyncDefaultForwarder) Start() error {
	return nil
}
func (f *SyncDefaultForwarder) Stop() {
}
func (f *SyncDefaultForwarder) sendHTTPTransactions(transactions []*HTTPTransaction) error {
	for _, t := range transactions {
		t.Process(context.Background(), f.client)
	}
	return nil
}
func (f *SyncDefaultForwarder) SubmitV1Series(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1SeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitV1Intake(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1IntakeEndpoint, payload, true, extra)
	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitSeries(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(seriesEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitEvents(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(eventsEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitServiceChecks(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(serviceChecksEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(hostMetadataEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitMetadata(payload Payloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(metadataEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}
func (f *SyncDefaultForwarder) SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(processesEndpoint, payload, extra, true)
}
func (f *SyncDefaultForwarder) SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(rtProcessesEndpoint, payload, extra, false)
}
func (f *SyncDefaultForwarder) SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(containerEndpoint, payload, extra, true)
}
func (f *SyncDefaultForwarder) SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(rtContainerEndpoint, payload, extra, false)
}
func (f *SyncDefaultForwarder) SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(connectionsEndpoint, payload, extra, true)
}
func (f *SyncDefaultForwarder) SubmitPodChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(podEndpoint, payload, extra, true)
}
