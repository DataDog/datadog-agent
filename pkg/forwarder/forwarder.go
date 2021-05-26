// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder/internal/retry"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// PayloadTypePod is the name of the pod payload type
	PayloadTypePod = "pod"
	// PayloadTypeDeployment is the name of the deployment payload type
	PayloadTypeDeployment = "deployment"
	// PayloadTypeReplicaSet is the name of the replica set payload type
	PayloadTypeReplicaSet = "replicaset"
	// PayloadTypeService is the name of the service payload type
	PayloadTypeService = "service"
	// PayloadTypeNode is the name of the node payload type
	PayloadTypeNode = "node"
	// PayloadTypeCluster is the name of the cluster payload type
	PayloadTypeCluster = "cluster"
	// PayloadTypeJob is the name of the job payload type
	PayloadTypeJob = "job"
	// PayloadTypeCronJob is the name of the cronjob payload type
	PayloadTypeCronJob = "cronjob"
	// PayloadTypeDaemonset is the name of the daemonset payload type
	PayloadTypeDaemonset = "daemonset"
)

const (
	// Stopped represent the internal state of an unstarted Forwarder.
	Stopped uint32 = iota
	// Started represent the internal state of an started Forwarder.
	Started
)

const (
	apiHTTPHeaderKey          = "DD-Api-Key"
	versionHTTPHeaderKey      = "DD-Agent-Version"
	useragentHTTPHeaderKey    = "User-Agent"
	arbitraryTagHTTPHeaderKey = "Allow-Arbitrary-Tag-Value"
)

// The amount of time the forwarder will wait to receive process-like response payloads before giving up
// This is a var so that it can be changed for testing
var defaultResponseTimeout = 30 * time.Second

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
	SubmitAgentChecksMetadata(payload Payloads, extra http.Header) error
	SubmitMetadata(payload Payloads, extra http.Header) error
	SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error)
	SubmitOrchestratorChecks(payload Payloads, extra http.Header, payloadType string) (chan Response, error)
}

// Compile-time check to ensure that DefaultForwarder implements the Forwarder interface
var _ Forwarder = &DefaultForwarder{}

// Features is a bitmask to enable specific forwarder features
type Features uint8

const (
	// CoreFeatures bitmask to enable specific core features
	CoreFeatures Features = 1 << iota
	// TraceFeatures bitmask to enable specific trace features
	TraceFeatures
	// ProcessFeatures bitmask to enable specific process features
	ProcessFeatures
	// SysProbeFeatures bitmask to enable specific system-probe features
	SysProbeFeatures
)

// Options contain the configuration options for the DefaultForwarder
type Options struct {
	NumberOfWorkers                int
	RetryQueuePayloadsTotalMaxSize int
	DisableAPIKeyChecking          bool
	EnabledFeatures                Features
	APIKeyValidationInterval       time.Duration
	KeysPerDomain                  map[string][]string
	ConnectionResetInterval        time.Duration
	CompletionHandler              transaction.HTTPCompletionHandler
}

// SetFeature sets forwarder features in a feature set
func SetFeature(features, flag Features) Features { return features | flag }

// ClearFeature clears forwarder features from a feature set
func ClearFeature(features, flag Features) Features { return features &^ flag }

// ToggleFeature toggles forwarder features in a feature set
func ToggleFeature(features, flag Features) Features { return features ^ flag }

// HasFeature lets you know if a specific feature flag is set in a feature set
func HasFeature(features, flag Features) bool { return features&flag != 0 }

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

	const forwarderRetryQueueMaxSizeKey = "forwarder_retry_queue_max_size"
	const forwarderRetryQueuePayloadsMaxSizeKey = "forwarder_retry_queue_payloads_max_size"

	retryQueuePayloadsTotalMaxSize := 15 * 1024 * 1024
	if config.Datadog.IsSet(forwarderRetryQueuePayloadsMaxSizeKey) {
		retryQueuePayloadsTotalMaxSize = config.Datadog.GetInt(forwarderRetryQueuePayloadsMaxSizeKey)
	}

	option := &Options{
		NumberOfWorkers:                config.Datadog.GetInt("forwarder_num_workers"),
		DisableAPIKeyChecking:          false,
		RetryQueuePayloadsTotalMaxSize: retryQueuePayloadsTotalMaxSize,
		APIKeyValidationInterval:       time.Duration(validationInterval) * time.Minute,
		KeysPerDomain:                  keysPerDomain,
		ConnectionResetInterval:        time.Duration(config.Datadog.GetInt("forwarder_connection_reset_interval")) * time.Second,
	}

	if config.Datadog.IsSet(forwarderRetryQueueMaxSizeKey) {
		if config.Datadog.IsSet(forwarderRetryQueuePayloadsMaxSizeKey) {
			log.Warnf("'%v' is set, but as this setting is deprecated, '%v' is used instead.", forwarderRetryQueueMaxSizeKey, forwarderRetryQueuePayloadsMaxSizeKey)
		} else {
			forwarderRetryQueueMaxSize := config.Datadog.GetInt(forwarderRetryQueueMaxSizeKey)
			option.setRetryQueuePayloadsTotalMaxSizeFromQueueMax(forwarderRetryQueueMaxSize)
			log.Warnf("'%v = %v' is used, but this setting is deprecated. '%v = %v' (%v * 2MB) is used instead as the maximum payload size is 2MB.",
				forwarderRetryQueueMaxSizeKey,
				forwarderRetryQueueMaxSize,
				forwarderRetryQueuePayloadsMaxSizeKey,
				option.RetryQueuePayloadsTotalMaxSize,
				forwarderRetryQueueMaxSize)
		}
	}

	return option
}

// setRetryQueuePayloadsTotalMaxSizeFromQueueMax set `RetryQueuePayloadsTotalMaxSize` from the value
// of the deprecated settings `forwarder_retry_queue_max_size`
func (o *Options) setRetryQueuePayloadsTotalMaxSizeFromQueueMax(v int) {
	maxPayloadSize := 2 * 1024 * 1024
	o.RetryQueuePayloadsTotalMaxSize = v * maxPayloadSize
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

	completionHandler transaction.HTTPCompletionHandler
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
		completionHandler: options.CompletionHandler,
	}
	var optionalRemovalPolicy *retry.FileRemovalPolicy
	storageMaxSize := config.Datadog.GetInt64("forwarder_storage_max_size_in_bytes")

	// Disk Persistence is a core-only feature for now.
	if storageMaxSize == 0 {
		log.Infof("Retry queue storage on disk is disabled")
	} else if agentFolder := getAgentFolder(options); agentFolder != "" {
		storagePath := config.Datadog.GetString("forwarder_storage_path")
		if storagePath == "" {
			storagePath = path.Join(config.Datadog.GetString("run_path"), "transactions_to_retry")
		}
		outdatedFileInDays := config.Datadog.GetInt("forwarder_outdated_file_in_days")
		var err error

		storagePath = path.Join(storagePath, agentFolder)
		optionalRemovalPolicy, err = retry.NewFileRemovalPolicy(storagePath, outdatedFileInDays, retry.FileRemovalPolicyTelemetry{})
		if err != nil {
			log.Errorf("Error when initializing the removal policy: %v", err)
		} else {
			filesRemoved, err := optionalRemovalPolicy.RemoveOutdatedFiles()
			if err != nil {
				log.Errorf("Error when removing outdated files: %v", err)
			}
			log.Debugf("Outdated files removed: %v", strings.Join(filesRemoved, ", "))
		}
	} else {
		log.Infof("Retry queue storage on disk is disabled because the feature is unavailable for this process.")
	}

	flushToDiskMemRatio := config.Datadog.GetFloat64("forwarder_flush_to_disk_mem_ratio")
	domainForwarderSort := transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: true}
	transactionContainerSort := transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: false}

	for domain, keys := range options.KeysPerDomain {
		domain, _ := config.AddAgentVersionToDomain(domain, "app")
		if keys == nil || len(keys) == 0 {
			log.Errorf("No API keys for domain '%s', dropping domain ", domain)
		} else {
			var domainFolderPath string
			var err error
			if optionalRemovalPolicy != nil {
				domainFolderPath, err = optionalRemovalPolicy.RegisterDomain(domain)
				if err != nil {
					log.Errorf("Retry queue storage on disk disabled. Cannot register the domain '%v': %v", domain, err)
				}
			}

			transactionContainer := retry.BuildTransactionRetryQueue(
				options.RetryQueuePayloadsTotalMaxSize,
				flushToDiskMemRatio,
				domainFolderPath,
				storageMaxSize,
				transactionContainerSort,
				domain,
				keys)

			f.keysPerDomains[domain] = keys
			f.domainForwarders[domain] = newDomainForwarder(
				domain,
				transactionContainer,
				options.NumberOfWorkers,
				options.ConnectionResetInterval,
				domainForwarderSort)
		}
	}

	if optionalRemovalPolicy != nil {
		filesRemoved, err := optionalRemovalPolicy.RemoveUnknownDomains()
		if err != nil {
			log.Errorf("Error when removing outdated files: %v", err)
		}
		log.Debugf("Outdated files removed: %v", strings.Join(filesRemoved, ", "))
	}

	return f
}

func getAgentFolder(options *Options) string {
	if HasFeature(options.EnabledFeatures, CoreFeatures) {
		return "core"
	}
	return ""
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
func (f *DefaultForwarder) createHTTPTransactions(endpoint transaction.Endpoint, payloads Payloads, apiKeyInQueryString bool, extra http.Header) []*transaction.HTTPTransaction {
	return f.createAdvancedHTTPTransactions(endpoint, payloads, apiKeyInQueryString, extra, transaction.TransactionPriorityNormal, true)
}

func (f *DefaultForwarder) createAdvancedHTTPTransactions(endpoint transaction.Endpoint, payloads Payloads, apiKeyInQueryString bool, extra http.Header, priority transaction.Priority, storableOnDisk bool) []*transaction.HTTPTransaction {
	transactions := make([]*transaction.HTTPTransaction, 0, len(payloads)*len(f.keysPerDomains))
	allowArbitraryTags := config.Datadog.GetBool("allow_arbitrary_tags")

	for _, payload := range payloads {
		for domain, apiKeys := range f.keysPerDomains {
			for _, apiKey := range apiKeys {
				t := transaction.NewHTTPTransaction()
				t.Domain = domain
				t.Endpoint = endpoint
				if apiKeyInQueryString {
					t.Endpoint.Route = fmt.Sprintf("%s?api_key=%s", endpoint.Route, apiKey)
				}
				t.Payload = payload
				t.Priority = priority
				t.StorableOnDisk = storableOnDisk
				t.Headers.Set(apiHTTPHeaderKey, apiKey)
				t.Headers.Set(versionHTTPHeaderKey, version.AgentVersion)
				t.Headers.Set(useragentHTTPHeaderKey, fmt.Sprintf("datadog-agent/%s", version.AgentVersion))
				if allowArbitraryTags {
					t.Headers.Set(arbitraryTagHTTPHeaderKey, "true")
				}

				if f.completionHandler != nil {
					t.CompletionHandler = f.completionHandler
				}

				tlmTxInputCount.Inc(domain, endpoint.Name)
				tlmTxInputBytes.Add(float64(t.GetPayloadSize()), domain, endpoint.Name)
				transactionsInputCountByEndpoint.Add(endpoint.Name, 1)
				transactionsInputBytesByEndpoint.Add(endpoint.Name, int64(t.GetPayloadSize()))

				for key := range extra {
					t.Headers.Set(key, extra.Get(key))
				}
				transactions = append(transactions, t)
			}
		}
	}
	return transactions
}

func (f *DefaultForwarder) sendHTTPTransactions(transactions []*transaction.HTTPTransaction) error {
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
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvents will send an event type payload to Datadog backend.
func (f *DefaultForwarder) SubmitEvents(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(eventsEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitServiceChecks will send a service check type payload to Datadog backend.
func (f *DefaultForwarder) SubmitServiceChecks(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(serviceChecksEndpoint, payload, false, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *DefaultForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, extra,
		func(endpoint transaction.Endpoint, payloads Payloads, apiKeyInQueryString bool, extra http.Header) []*transaction.HTTPTransaction {
			// Host metadata contains the API KEY and should not be stored on disk.
			storableOnDisk := false
			return f.createAdvancedHTTPTransactions(endpoint, payloads, apiKeyInQueryString, extra, transaction.TransactionPriorityHigh, storableOnDisk)
		})
}

// SubmitAgentChecksMetadata will send a agentchecks_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitAgentChecksMetadata(payload Payloads, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, extra,
		func(endpoint transaction.Endpoint, payloads Payloads, apiKeyInQueryString bool, extra http.Header) []*transaction.HTTPTransaction {
			// Agentchecks metadata contains the API KEY and should not be stored on disk.
			storableOnDisk := false
			return f.createAdvancedHTTPTransactions(endpoint, payloads, apiKeyInQueryString, extra, transaction.TransactionPriorityNormal, storableOnDisk)
		})
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *DefaultForwarder) SubmitMetadata(payload Payloads, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, extra, f.createHTTPTransactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1Series(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1SeriesEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *DefaultForwarder) SubmitV1Intake(payload Payloads, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, extra, f.createHTTPTransactions)
}

func (f *DefaultForwarder) submitV1IntakeWithTransactionsFactory(
	payload Payloads,
	extra http.Header,
	createHTTPTransactions func(endpoint transaction.Endpoint, payload Payloads, apiKeyInQueryString bool, extra http.Header) []*transaction.HTTPTransaction) error {
	transactions := createHTTPTransactions(v1IntakeEndpoint, payload, true, extra)

	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitProcessChecks sends process checks
func (f *DefaultForwarder) SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(processesEndpoint, payload, extra, true)
}

// SubmitRTProcessChecks sends real time process checks
func (f *DefaultForwarder) SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(rtProcessesEndpoint, payload, extra, false)
}

// SubmitContainerChecks sends container checks
func (f *DefaultForwarder) SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(containerEndpoint, payload, extra, true)
}

// SubmitRTContainerChecks sends real time container checks
func (f *DefaultForwarder) SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(rtContainerEndpoint, payload, extra, false)
}

// SubmitConnectionChecks sends connection checks
func (f *DefaultForwarder) SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(connectionsEndpoint, payload, extra, true)
}

// SubmitOrchestratorChecks sends orchestrator checks
func (f *DefaultForwarder) SubmitOrchestratorChecks(payload Payloads, extra http.Header, payloadType string) (chan Response, error) {
	switch payloadType {
	case PayloadTypePod:
		transactionsIntakePod.Add(1)
	case PayloadTypeDeployment:
		transactionsIntakeDeployment.Add(1)
	case PayloadTypeReplicaSet:
		transactionsIntakeReplicaSet.Add(1)
	case PayloadTypeService:
		transactionsIntakeService.Add(1)
	case PayloadTypeNode:
		transactionsIntakeNode.Add(1)
	case PayloadTypeJob:
		transactionsIntakeJob.Add(1)
	case PayloadTypeCronJob:
		transactionsIntakeCronJob.Add(1)
	case PayloadTypeCluster:
		transactionsIntakeCluster.Add(1)
	case PayloadTypeDaemonset:
		transactionsIntakeDaemonSet.Add(1)
	}

	return f.submitProcessLikePayload(orchestratorEndpoint, payload, extra, true)
}

func (f *DefaultForwarder) submitProcessLikePayload(ep transaction.Endpoint, payload Payloads, extra http.Header, retryable bool) (chan Response, error) {
	transactions := f.createHTTPTransactions(ep, payload, false, extra)
	results := make(chan Response, len(transactions))
	internalResults := make(chan Response, len(transactions))
	expectedResponses := len(transactions)

	for _, txn := range transactions {
		txn.Retryable = retryable
		txn.AttemptHandler = func(transaction *transaction.HTTPTransaction) {
			if v := transaction.Headers.Get("X-DD-Agent-Attempts"); v == "" {
				transaction.Headers.Set("X-DD-Agent-Attempts", "1")
			} else {
				attempts, _ := strconv.ParseInt(v, 10, 0)
				transaction.Headers.Set("X-DD-Agent-Attempts", strconv.Itoa(int(attempts+1)))
			}
		}

		txn.CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
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
