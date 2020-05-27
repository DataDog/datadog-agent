// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"expvar"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	forwarderExpvars              = expvar.NewMap("forwarder")
	connectionEvents              = expvar.Map{}
	transactionsExpvars           = expvar.Map{}
	transactionsSeries            = expvar.Int{}
	transactionsEvents            = expvar.Int{}
	transactionsServiceChecks     = expvar.Int{}
	transactionsSketchSeries      = expvar.Int{}
	transactionsHostMetadata      = expvar.Int{}
	transactionsMetadata          = expvar.Int{}
	transactionsTimeseriesV1      = expvar.Int{}
	transactionsCheckRunsV1       = expvar.Int{}
	transactionsIntakeV1          = expvar.Int{}
	transactionsIntakeProcesses   = expvar.Int{}
	transactionsIntakeRTProcesses = expvar.Int{}
	transactionsIntakeContainer   = expvar.Int{}
	transactionsIntakeRTContainer = expvar.Int{}
	transactionsIntakeConnections = expvar.Int{}
	transactionsIntakePod         = expvar.Int{}

	tlm = telemetry.NewCounter("forwarder", "transactions",
		[]string{"endpoint", "route"}, "Forwarder telemetry")

	v1SeriesEndpoint       = endpoint{"/api/v1/series", "series_v1"}
	v1CheckRunsEndpoint    = endpoint{"/api/v1/check_run", "check_run_v1"}
	v1IntakeEndpoint       = endpoint{"/intake/", "intake"}
	v1SketchSeriesEndpoint = endpoint{"/api/v1/sketches", "sketches_v1"} // nolint unused for now
	v1ValidateEndpoint     = endpoint{"/api/v1/validate", "validate_v1"}

	seriesEndpoint        = endpoint{"/api/v2/series", "series_v2"}
	eventsEndpoint        = endpoint{"/api/v2/events", "events_v2"}
	serviceChecksEndpoint = endpoint{"/api/v2/service_checks", "services_checks_v2"}
	sketchSeriesEndpoint  = endpoint{"/api/beta/sketches", "sketches_v2"}
	hostMetadataEndpoint  = endpoint{"/api/v2/host_metadata", "host_metadata_v2"}
	metadataEndpoint      = endpoint{"/api/v2/metadata", "metadata_v2"}

	processesEndpoint   = endpoint{"/api/v1/collector", "process"}
	rtProcessesEndpoint = endpoint{"/api/v1/collector", "rtprocess"}
	containerEndpoint   = endpoint{"/api/v1/container", "container"}
	rtContainerEndpoint = endpoint{"/api/v1/container", "rtcontainer"}
	connectionsEndpoint = endpoint{"/api/v1/collector", "connections"}
	podEndpoint         = endpoint{"/api/v1/orchestrator", "pod"}
)

func init() {
	transactionsExpvars.Init()
	connectionEvents.Init()
	forwarderExpvars.Set("Transactions", &transactionsExpvars)
	forwarderExpvars.Set("ConnectionEvents", &connectionEvents)
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
	initDomainForwarderExpvars()
	initTransactionExpvars()
	initForwarderHealthExpvars()
}

const (
	// Stopped represent the internal state of an unstarted Forwarder.
	Stopped uint32 = iota
	// Started represent the internal state of an started Forwarder.
	Started
)

const (
	apiHTTPHeaderKey       = "DD-Api-Key"
	versionHTTPHeaderKey   = "DD-Agent-Version"
	useragentHTTPHeaderKey = "User-Agent"
)

// The amount of time the forwarder will wait to receive process-like response payloads before giving up
// This is a var so that it can be changed for testing
var defaultResponseTimeout = 30 * time.Second

type endpoint struct {
	// Route to hit in the HTTP transaction
	route string
	// Name of the endpoint for the telemetry metrics
	name string
}

func (e endpoint) String() string {
	return e.route
}

// Payloads is a slice of pointers to byte arrays, an alias for the slices of
// payloads we pass into the forwarder
type Payloads []*[]byte

// Response contains the response details of a successfully posted transaction
type Response struct {
	Domain     string
	Body       []byte
	StatusCode int
	Err        error
}

// Forwarder interface allows packages to send payload to the backend
type Forwarder interface {
	Start() error
	Stop()
	SubmitV1Series(payload Payloads, extra http.Header) error
	SubmitV1Intake(payload Payloads, extra http.Header) error
	SubmitV1CheckRuns(payload Payloads, extra http.Header) error
	SubmitSeries(payload Payloads, extra http.Header) error
	SubmitEvents(payload Payloads, extra http.Header) error
	SubmitServiceChecks(payload Payloads, extra http.Header) error
	SubmitSketchSeries(payload Payloads, extra http.Header) error
	SubmitHostMetadata(payload Payloads, extra http.Header) error
	SubmitMetadata(payload Payloads, extra http.Header) error
	SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitPodChecks(payload Payloads, extra http.Header) (chan Response, error)
}

// Compile-time check to ensure that DefaultForwarder implements the Forwarder interface
var _ Forwarder = &DefaultForwarder{}

// Options contain the configuration options for the DefaultForwarder
type Options struct {
	NumberOfWorkers          int
	RetryQueueSize           int
	DisableAPIKeyChecking    bool
	APIKeyValidationInterval time.Duration
	KeysPerDomain            map[string][]string
	ConnectionResetInterval  time.Duration
}

// NewOptions creates new Options with default values
func NewOptions(keysPerDomain map[string][]string) *Options {
	validationInterval := config.Datadog.GetInt("forwarder_apikey_validation_interval")
	if validationInterval <= 0 {
		log.Warnf(
			"'forwarder_apikey_validation_interval' set to invalid value (%d), defaulting to %d minute(s)",
			validationInterval,
			config.DefaultAPIKeyValidationInterval,
		)
		validationInterval = config.DefaultAPIKeyValidationInterval
	}

	return &Options{
		NumberOfWorkers:          config.Datadog.GetInt("forwarder_num_workers"),
		RetryQueueSize:           config.Datadog.GetInt("forwarder_retry_queue_max_size"),
		DisableAPIKeyChecking:    false,
		APIKeyValidationInterval: time.Duration(validationInterval) * time.Minute,
		KeysPerDomain:            keysPerDomain,
		ConnectionResetInterval:  time.Duration(config.Datadog.GetInt("forwarder_connection_reset_interval")) * time.Second,
	}
}

// DefaultForwarder is the default implementation of the Forwarder.
type DefaultForwarder struct {
	// NumberOfWorkers Number of concurrent HTTP request made by the DefaultForwarder (default 4).
	NumberOfWorkers int

	domainForwarders map[string]*domainForwarder
	keysPerDomains   map[string][]string
	healthChecker    *forwarderHealth
	internalState    uint32
	m                sync.Mutex // To control Start/Stop races
}

// NewDefaultForwarder returns a new DefaultForwarder.
func NewDefaultForwarder(options *Options) *DefaultForwarder {
	f := &DefaultForwarder{
		NumberOfWorkers:  options.NumberOfWorkers,
		domainForwarders: map[string]*domainForwarder{},
		keysPerDomains:   map[string][]string{},
		internalState:    Stopped,
		healthChecker: &forwarderHealth{
			keysPerDomains:        options.KeysPerDomain,
			disableAPIKeyChecking: options.DisableAPIKeyChecking,
			validationInterval:    options.APIKeyValidationInterval,
		},
	}

	for domain, keys := range options.KeysPerDomain {
		domain, _ := config.AddAgentVersionToDomain(domain, "app")
		if keys == nil || len(keys) == 0 {
			log.Errorf("No API keys for domain '%s', dropping domain ", domain)
		} else {
			f.keysPerDomains[domain] = keys
			f.domainForwarders[domain] = newDomainForwarder(domain, options.NumberOfWorkers, options.RetryQueueSize, options.ConnectionResetInterval)
		}
	}

	return f
}

// Start initialize and runs the forwarder.
func (f *DefaultForwarder) Start() error {
	// Lock so we can't stop a Forwarder while is starting
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Started {
		return fmt.Errorf("the forwarder is already started")
	}

	for _, df := range f.domainForwarders {
		_ = df.Start()
	}

	// log endpoints configuration
	endpointLogs := make([]string, 0, len(f.keysPerDomains))
	for domain, apiKeys := range f.keysPerDomains {
		endpointLogs = append(endpointLogs, fmt.Sprintf("\"%s\" (%v api key(s))",
			domain, len(apiKeys)))
	}
	log.Infof("Forwarder started, sending to %v endpoint(s) with %v worker(s) each: %s",
		len(endpointLogs), f.NumberOfWorkers, strings.Join(endpointLogs, " ; "))

	f.healthChecker.Start()
	f.internalState = Started
	return nil
}

// Stop all the component of a forwarder and free resources
func (f *DefaultForwarder) Stop() {
	log.Infof("stopping the Forwarder")
	// Lock so we can't start a Forwarder while is stopping
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Stopped {
		log.Warnf("the forwarder is already stopped")
		return
	}

	f.internalState = Stopped

	purgeTimeout := config.Datadog.GetDuration("forwarder_stop_timeout") * time.Second
	if purgeTimeout > 0 {
		var wg sync.WaitGroup

		for _, df := range f.domainForwarders {
			wg.Add(1)
			go func(df *domainForwarder) {
				df.Stop(true)
				wg.Done()
			}(df)
		}

		donePurging := make(chan struct{})
		go func() {
			wg.Wait()
			close(donePurging)
		}()

		select {
		case <-donePurging:
		case <-time.After(purgeTimeout):
			log.Warnf("Timeout emptying new transactions before stopping the forwarder %v", purgeTimeout)
		}
	} else {
		for _, df := range f.domainForwarders {
			df.Stop(false)
		}
	}

	f.healthChecker.Stop()

	f.healthChecker = nil
	f.domainForwarders = map[string]*domainForwarder{}

}

// State returns the internal state of the forwarder (Started or Stopped)
func (f *DefaultForwarder) State() uint32 {
	// Lock so we can't start/stop a Forwarder while getting its state
	f.m.Lock()
	defer f.m.Unlock()

	return f.internalState
}

func (f *DefaultForwarder) createHTTPTransactions(endpoint endpoint, payloads Payloads, apiKeyInQueryString bool, extra http.Header) []*HTTPTransaction {
	transactions := make([]*HTTPTransaction, 0, len(payloads)*len(f.keysPerDomains))
	for _, payload := range payloads {
		for domain, apiKeys := range f.keysPerDomains {
			for _, apiKey := range apiKeys {
				transactionEndpoint := endpoint.route
				if apiKeyInQueryString {
					transactionEndpoint = fmt.Sprintf("%s?api_key=%s", endpoint.route, apiKey)
				}
				t := NewHTTPTransaction()
				t.Domain = domain
				t.Endpoint = transactionEndpoint
				t.Payload = payload
				t.Headers.Set(apiHTTPHeaderKey, apiKey)
				t.Headers.Set(versionHTTPHeaderKey, version.AgentVersion)
				t.Headers.Set(useragentHTTPHeaderKey, fmt.Sprintf("datadog-agent/%s", version.AgentVersion))

				tlm.Inc(domain, endpoint.name)

				for key := range extra {
					t.Headers.Set(key, extra.Get(key))
				}
				transactions = append(transactions, t)
			}
		}
	}
	return transactions
}

func (f *DefaultForwarder) sendHTTPTransactions(transactions []*HTTPTransaction) error {
	if atomic.LoadUint32(&f.internalState) == Stopped {
		return fmt.Errorf("the forwarder is not started")
	}

	for _, t := range transactions {
		if err := f.domainForwarders[t.Domain].sendHTTPTransactions(t); err != nil {
			log.Errorf(err.Error())
		}
	}
	return nil
}

// SubmitSeries will send a series type payload to Datadog backend.
func (f *DefaultForwarder) SubmitSeries(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(seriesEndpoint, payload, false, extra)
	transactionsSeries.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvents will send an event type payload to Datadog backend.
func (f *DefaultForwarder) SubmitEvents(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(eventsEndpoint, payload, false, extra)
	transactionsEvents.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitServiceChecks will send a service check type payload to Datadog backend.
func (f *DefaultForwarder) SubmitServiceChecks(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(serviceChecksEndpoint, payload, false, extra)
	transactionsServiceChecks.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *DefaultForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extra)
	transactionsSketchSeries.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(hostMetadataEndpoint, payload, false, extra)
	transactionsHostMetadata.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *DefaultForwarder) SubmitMetadata(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(metadataEndpoint, payload, false, extra)
	transactionsMetadata.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1Series(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1SeriesEndpoint, payload, true, extra)
	transactionsTimeseriesV1.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extra)
	transactionsCheckRunsV1.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *DefaultForwarder) SubmitV1Intake(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1IntakeEndpoint, payload, true, extra)

	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}

	transactionsIntakeV1.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitProcessChecks sends process checks
func (f *DefaultForwarder) SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	transactionsIntakeProcesses.Add(1)

	return f.submitProcessLikePayload(processesEndpoint, payload, extra, true)
}

// SubmitRTProcessChecks sends real time process checks
func (f *DefaultForwarder) SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	transactionsIntakeRTProcesses.Add(1)

	return f.submitProcessLikePayload(rtProcessesEndpoint, payload, extra, false)
}

// SubmitContainerChecks sends container checks
func (f *DefaultForwarder) SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	transactionsIntakeContainer.Add(1)

	return f.submitProcessLikePayload(containerEndpoint, payload, extra, true)
}

// SubmitRTContainerChecks sends real time container checks
func (f *DefaultForwarder) SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	transactionsIntakeRTContainer.Add(1)

	return f.submitProcessLikePayload(rtContainerEndpoint, payload, extra, false)
}

// SubmitConnectionChecks sends connection checks
func (f *DefaultForwarder) SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error) {
	transactionsIntakeConnections.Add(1)

	return f.submitProcessLikePayload(connectionsEndpoint, payload, extra, true)
}

// SubmitPodChecks sends pod checks
func (f *DefaultForwarder) SubmitPodChecks(payload Payloads, extra http.Header) (chan Response, error) {
	transactionsIntakePod.Add(1)

	return f.submitProcessLikePayload(podEndpoint, payload, extra, true)
}

func (f *DefaultForwarder) submitProcessLikePayload(ep endpoint, payload Payloads, extra http.Header, retryable bool) (chan Response, error) {
	transactions := f.createHTTPTransactions(ep, payload, false, extra)

	results := make(chan Response, len(transactions))
	internalResults := make(chan Response, len(transactions))
	expectedResponses := len(transactions)

	for _, txn := range transactions {
		txn.retryable = retryable
		txn.attemptHandler = func(transaction *HTTPTransaction) {
			if v := transaction.Headers.Get("X-DD-Agent-Attempts"); v == "" {
				transaction.Headers.Set("X-DD-Agent-Attempts", "1")
			} else {
				attempts, _ := strconv.ParseInt(v, 10, 0)
				transaction.Headers.Set("X-DD-Agent-Attempts", strconv.Itoa(int(attempts+1)))
			}
		}

		txn.completionHandler = func(transaction *HTTPTransaction, statusCode int, body []byte, err error) {
			internalResults <- Response{
				Domain:     transaction.Domain,
				Body:       body,
				StatusCode: statusCode,
				Err:        err,
			}
		}
	}

	go func() {
		receivedResponses := 0
		for {
			select {
			case r := <-internalResults:
				results <- r
				receivedResponses++
				if receivedResponses == expectedResponses {
					close(results)
					return
				}
			case <-time.After(defaultResponseTimeout):
				log.Errorf("timed out waiting for responses, received %d/%d", receivedResponses, expectedResponses)
				close(results)
				return
			}
		}
	}()

	return results, f.sendHTTPTransactions(transactions)
}
