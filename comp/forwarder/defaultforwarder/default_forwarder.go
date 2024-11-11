// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/internal/retry"
	pkgresolver "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// Stopped represent the internal state of an unstarted Forwarder.
	Stopped uint32 = iota
	// Started represent the internal state of an started Forwarder.
	Started
	// Disabled represent the internal state of a disabled Forwarder.
	Disabled
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

// Response contains the response details of a successfully posted transaction
type Response struct {
	Domain     string
	Body       []byte
	StatusCode int
	Err        error
}

// Forwarder interface allows packages to send payload to the backend
type Forwarder interface {
	SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error
	SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error
	SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error
	SubmitSeries(payload transaction.BytesPayloads, extra http.Header) error
	SubmitSketchSeries(payload transaction.BytesPayloads, extra http.Header) error
	SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitProcessEventChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) (chan Response, error)
	SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
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
	DomainResolvers                map[string]pkgresolver.DomainResolver
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
func NewOptions(config config.Component, log log.Component, keysPerDomain map[string][]string) *Options {
	resolvers := pkgresolver.NewSingleDomainResolvers(keysPerDomain)
	vectorMetricsURL, err := pkgconfigsetup.GetObsPipelineURL(pkgconfigsetup.Metrics, config)
	if err != nil {
		log.Error("Misconfiguration of agent observability_pipelines_worker endpoint for metrics: ", err)
	}
	if r, ok := resolvers[utils.GetInfraEndpoint(config)]; ok && vectorMetricsURL != "" {
		log.Debugf("Configuring forwarder to send metrics to observability_pipelines_worker: %s", vectorMetricsURL)
		resolvers[utils.GetInfraEndpoint(config)] = pkgresolver.NewDomainResolverWithMetricToVector(
			r.GetBaseDomain(),
			r.GetAPIKeys(),
			vectorMetricsURL,
		)
	}
	return NewOptionsWithResolvers(config, log, resolvers)
}

// NewOptionsWithResolvers creates new Options with default values
func NewOptionsWithResolvers(config config.Component, log log.Component, domainResolvers map[string]pkgresolver.DomainResolver) *Options {
	validationInterval := config.GetInt("forwarder_apikey_validation_interval")
	if validationInterval <= 0 {
		log.Warnf(
			"'forwarder_apikey_validation_interval' set to invalid value (%d), defaulting to %d minute(s)",
			validationInterval,
			pkgconfigsetup.DefaultAPIKeyValidationInterval,
		)
		validationInterval = pkgconfigsetup.DefaultAPIKeyValidationInterval
	}

	const forwarderRetryQueueMaxSizeKey = "forwarder_retry_queue_max_size"
	const forwarderRetryQueuePayloadsMaxSizeKey = "forwarder_retry_queue_payloads_max_size"

	retryQueuePayloadsTotalMaxSize := 15 * 1024 * 1024
	if config.IsSet(forwarderRetryQueuePayloadsMaxSizeKey) {
		retryQueuePayloadsTotalMaxSize = config.GetInt(forwarderRetryQueuePayloadsMaxSizeKey)
	}

	option := &Options{
		NumberOfWorkers:                config.GetInt("forwarder_num_workers"),
		DisableAPIKeyChecking:          false,
		RetryQueuePayloadsTotalMaxSize: retryQueuePayloadsTotalMaxSize,
		APIKeyValidationInterval:       time.Duration(validationInterval) * time.Minute,
		DomainResolvers:                domainResolvers,
		ConnectionResetInterval:        time.Duration(config.GetInt("forwarder_connection_reset_interval")) * time.Second,
	}

	if config.IsSet(forwarderRetryQueueMaxSizeKey) {
		if config.IsSet(forwarderRetryQueuePayloadsMaxSizeKey) {
			log.Warnf("'%v' is set, but as this setting is deprecated, '%v' is used instead.", forwarderRetryQueueMaxSizeKey, forwarderRetryQueuePayloadsMaxSizeKey)
		} else {
			forwarderRetryQueueMaxSize := config.GetInt(forwarderRetryQueueMaxSizeKey)
			option.setRetryQueuePayloadsTotalMaxSizeFromQueueMax(forwarderRetryQueueMaxSize)
			log.Warnf("'%v = %v' is used, but this setting is deprecated. '%v = %v' (%v * 2MB) is used instead as the maximum payload size is 2MB.",
				forwarderRetryQueueMaxSizeKey,
				forwarderRetryQueueMaxSize,
				forwarderRetryQueuePayloadsMaxSizeKey,
				option.RetryQueuePayloadsTotalMaxSize,
				forwarderRetryQueueMaxSize)
		}
	}

	// domainforwarder to local DCA for autoscaling failover metrics
	if config.GetBool("autoscaling.failover.enabled") && config.GetBool("cluster_agent.enabled") {
		if domain, err := utils.GetClusterAgentEndpoint(); err != nil {
			log.Errorf("Could not get cluster agent endpoint for autoscaling failover metrics: %s", err)
		} else if authToken, err := security.GetClusterAgentAuthToken(config); err != nil {
			log.Errorf("Failed to get cluster agent auth token: ", err)
		} else {
			log.Infof("Setting cluster agent domain resolver: %s", domain)
			option.DomainResolvers[domain] = pkgresolver.NewLocalDomainResolver(domain, authToken)
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

// SetEnabledFeatures sets the features enabled
func (o *Options) SetEnabledFeatures(features []Features) {
	for _, feature := range features {
		o.EnabledFeatures = SetFeature(o.EnabledFeatures, feature)
	}
}

// DefaultForwarder is the default implementation of the Forwarder.
type DefaultForwarder struct {
	config config.Component
	log    log.Component

	// NumberOfWorkers Number of concurrent HTTP request made by the DefaultForwarder (default 4).
	NumberOfWorkers int

	domainForwarders map[string]*domainForwarder
	domainResolvers  map[string]pkgresolver.DomainResolver
	localForwarder   *domainForwarder // domain forward used for communication with the local cluster-agent
	healthChecker    *forwarderHealth
	internalState    *atomic.Uint32
	m                sync.Mutex // To control Start/Stop races

	completionHandler transaction.HTTPCompletionHandler

	agentName                       string
	queueDurationCapacity           *retry.QueueDurationCapacity
	retryQueueDurationCapacityMutex sync.Mutex
}

// NewDefaultForwarder returns a new DefaultForwarder.
// TODO: (components) Remove this method and other exported methods in comp/forwarder.
func NewDefaultForwarder(config config.Component, log log.Component, options *Options) *DefaultForwarder {
	agentName := getAgentName(options)
	f := &DefaultForwarder{
		config:           config,
		log:              log,
		NumberOfWorkers:  options.NumberOfWorkers,
		domainForwarders: map[string]*domainForwarder{},
		domainResolvers:  map[string]pkgresolver.DomainResolver{},
		internalState:    atomic.NewUint32(Stopped),
		healthChecker: &forwarderHealth{
			log:                   log,
			config:                config,
			domainResolvers:       options.DomainResolvers,
			disableAPIKeyChecking: options.DisableAPIKeyChecking,
			validationInterval:    options.APIKeyValidationInterval,
		},
		completionHandler: options.CompletionHandler,
		agentName:         agentName,
		localForwarder:    nil,
	}
	var optionalRemovalPolicy *retry.FileRemovalPolicy
	storageMaxSize := config.GetInt64("forwarder_storage_max_size_in_bytes")
	var diskUsageLimit *retry.DiskUsageLimit

	// Disk Persistence is a core-only feature for now.
	if storageMaxSize == 0 {
		log.Infof("Retry queue storage on disk is disabled")
	} else if agentName != "" {
		storagePath := config.GetString("forwarder_storage_path")
		if storagePath == "" {
			storagePath = path.Join(config.GetString("run_path"), "transactions_to_retry")
		}
		outdatedFileInDays := config.GetInt("forwarder_outdated_file_in_days")
		var err error

		storagePath = path.Join(storagePath, agentName)
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

		diskRatio := config.GetFloat64("forwarder_storage_max_disk_ratio")
		diskUsageLimit = retry.NewDiskUsageLimit(storagePath, filesystem.NewDisk(), storageMaxSize, diskRatio)

	} else {
		log.Infof("Retry queue storage on disk is disabled because the feature is unavailable for this process.")
	}

	flushToDiskMemRatio := config.GetFloat64("forwarder_flush_to_disk_mem_ratio")
	domainForwarderSort := transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: true}
	transactionContainerSort := transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: false}

	for domain, resolver := range options.DomainResolvers {
		isMRF := false
		if config.GetBool("multi_region_failover.enabled") {
			log.Infof("MRF is enabled, checking site: %v ", domain)
			siteURL, err := utils.GetMRFInfraEndpoint(config)
			if err != nil {
				log.Error("Error building MRF infra endpoint: ", err)
			}
			if domain == siteURL {
				log.Infof("MRF domain '%s', configured ", domain)
				isMRF = true
			}

		}
		domain, _ := utils.AddAgentVersionToDomain(domain, "app")
		resolver.SetBaseDomain(domain)

		_, isLocal := resolver.(*pkgresolver.LocalDomainResolver)
		if !isLocal && (resolver.GetAPIKeys() == nil || len(resolver.GetAPIKeys()) == 0) {
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

			pointCountTelemetry := retry.NewPointCountTelemetry(domain)
			transactionContainer := retry.BuildTransactionRetryQueue(
				log,
				options.RetryQueuePayloadsTotalMaxSize,
				flushToDiskMemRatio,
				domainFolderPath,
				diskUsageLimit,
				transactionContainerSort,
				resolver,
				pointCountTelemetry)
			f.domainResolvers[domain] = resolver
			fwd := newDomainForwarder(
				config,
				log,
				domain,
				isMRF,
				isLocal,
				transactionContainer,
				options.NumberOfWorkers,
				options.ConnectionResetInterval,
				domainForwarderSort,
				pointCountTelemetry)
			f.domainForwarders[domain] = fwd
			// Register all alternate domains for each forwarder
			for _, v := range resolver.GetAlternateDomains() {
				f.domainForwarders[v] = fwd
			}
		}
	}

	config.OnUpdate(func(setting string, oldValue, newValue any) {
		if setting != "api_key" {
			return
		}
		oldAPIKey, ok1 := oldValue.(string)
		newAPIKey, ok2 := newValue.(string)
		if ok1 && ok2 {
			for _, dr := range f.domainResolvers {
				dr.UpdateAPIKey(oldAPIKey, newAPIKey)
			}
		}
	})

	timeInterval := config.GetInt("forwarder_retry_queue_capacity_time_interval_sec")
	if f.agentName != "" {
		f.queueDurationCapacity = retry.NewQueueDurationCapacity(
			time.Duration(timeInterval)*time.Second,
			10*time.Second,
			options.RetryQueuePayloadsTotalMaxSize,
			diskUsageLimit)
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

func getAgentName(options *Options) string {
	if HasFeature(options.EnabledFeatures, CoreFeatures) {
		return "core"
	}
	// If a new Agent is supported by this function, the implementation of
	// QueueDurationCapacity.ComputeCapacity must be updated.
	// More specifically, `totalBytesPerSec` should takes into account other Agent processes.
	return ""
}

// Start initialize and runs the forwarder.
func (f *DefaultForwarder) Start() error {
	// Lock so we can't stop a Forwarder while is starting
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState.Load() == Started {
		return fmt.Errorf("the forwarder is already started")
	}

	for _, df := range f.domainForwarders {
		_ = df.Start()
	}

	// log endpoints configuration
	endpointLogs := make([]string, 0, len(f.domainResolvers))
	for domain, dr := range f.domainResolvers {
		endpointLogs = append(endpointLogs, fmt.Sprintf("\"%s\" (%v api key(s))",
			domain, len(dr.GetAPIKeys())))
	}
	f.log.Infof("Forwarder started, sending to %v endpoint(s) with %v worker(s) each: %s",
		len(endpointLogs), f.NumberOfWorkers, strings.Join(endpointLogs, " ; "))

	f.healthChecker.Start()
	f.internalState.Store(Started)
	return nil
}

// Stop all the component of a forwarder and free resources
func (f *DefaultForwarder) Stop() {
	f.log.Infof("stopping the Forwarder")
	// Lock so we can't start a Forwarder while is stopping
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState.Load() == Stopped {
		f.log.Warnf("the forwarder is already stopped")
		return
	}

	f.internalState.Store(Stopped)

	purgeTimeout := f.config.GetDuration("forwarder_stop_timeout") * time.Second
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
			f.log.Warnf("Timeout emptying new transactions before stopping the forwarder %v", purgeTimeout)
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

	return f.internalState.Load()
}

func (f *DefaultForwarder) createHTTPTransactions(endpoint transaction.Endpoint, payloads transaction.BytesPayloads, kind transaction.Kind, extra http.Header) []*transaction.HTTPTransaction {
	return f.createAdvancedHTTPTransactions(endpoint, payloads, extra, transaction.TransactionPriorityNormal, kind, true)
}

func (f *DefaultForwarder) createAdvancedHTTPTransactions(endpoint transaction.Endpoint, payloads transaction.BytesPayloads, extra http.Header, priority transaction.Priority, kind transaction.Kind, storableOnDisk bool) []*transaction.HTTPTransaction {
	transactions := make([]*transaction.HTTPTransaction, 0, len(payloads)*len(f.domainForwarders))
	allowArbitraryTags := f.config.GetBool("allow_arbitrary_tags")

	for _, payload := range payloads {
		for domain, dr := range f.domainResolvers {
			drDomain, destinationType := dr.Resolve(endpoint) // drDomain is the domain with agent version if not local
			if payload.Destination == transaction.LocalOnly {
				// if it is local payload, we should not send it to the remote endpoint
				if destinationType == pkgresolver.Local && endpoint == endpoints.SeriesEndpoint {
					t := transaction.NewHTTPTransaction()
					t.Domain = drDomain
					t.Endpoint = endpoint
					t.Payload = payload
					t.Priority = priority
					t.Kind = kind
					t.StorableOnDisk = storableOnDisk
					t.Destination = payload.Destination
					t.Headers.Set("Authorization", fmt.Sprintf("Bearer %s", dr.GetBearerAuthToken()))
					for key := range extra {
						t.Headers.Set(key, extra.Get(key))
					}
					tlmTxInputCount.Inc(drDomain, endpoint.Name)
					tlmTxInputBytes.Add(float64(t.GetPayloadSize()), domain, endpoint.Name)
					transactionsInputCountByEndpoint.Add(endpoint.Name, 1)
					transactionsInputBytesByEndpoint.Add(endpoint.Name, int64(t.GetPayloadSize()))
					transactions = append(transactions, t)
				}
			} else {
				for _, apiKey := range dr.GetAPIKeys() {
					t := transaction.NewHTTPTransaction()
					t.Domain = drDomain
					t.Endpoint = endpoint
					t.Payload = payload
					t.Priority = priority
					t.Kind = kind
					t.StorableOnDisk = storableOnDisk
					t.Destination = payload.Destination
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
	}
	return transactions
}

func (f *DefaultForwarder) sendHTTPTransactions(transactions []*transaction.HTTPTransaction) error {
	if f.internalState.Load() == Stopped {
		return fmt.Errorf("the forwarder is not started")
	}

	f.retryQueueDurationCapacityMutex.Lock()
	defer f.retryQueueDurationCapacityMutex.Unlock()

	now := time.Now()
	for _, t := range transactions {
		forwarder := f.domainForwarders[t.Domain]

		forwarder.sendHTTPTransactions(t)

		if f.queueDurationCapacity != nil {
			if err := f.queueDurationCapacity.OnTransaction(t, forwarder.domain, now); err != nil {
				f.log.Errorf("Cannot add a transaction to queueDurationCapacity: %v", err)
			}
		}
	}

	if f.queueDurationCapacity != nil {
		if capacities, err := f.queueDurationCapacity.ComputeCapacity(now); err != nil {
			f.log.Errorf("Cannot compute the capacity of the retry queues: %v", err)
		} else {
			telemetry := telemetry.GetStatsTelemetryProvider()
			metricPrefix := "datadog.agent.retry_queue_duration."
			for domain, t := range capacities {
				tags := []string{
					"agent:" + f.agentName,
					"domain:" + domain,
				}
				telemetry.Gauge(metricPrefix+"capacity_secs", t.Capacity.Seconds(), tags)
				telemetry.Gauge(metricPrefix+"bytes_per_sec", t.BytesPerSec, tags)
				telemetry.Gauge(metricPrefix+"capacity_bytes", float64(t.AvailableSpace), tags)
			}
		}
	}

	return nil
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *DefaultForwarder) SubmitSketchSeries(payload transaction.BytesPayloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(endpoints.SketchSeriesEndpoint, payload, transaction.Sketches, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, transaction.Metadata, extra,
		func(endpoint transaction.Endpoint, payloads transaction.BytesPayloads, _ transaction.Kind, extra http.Header) []*transaction.HTTPTransaction {
			// Host metadata contains the API KEY and should not be stored on disk.
			storableOnDisk := false
			return f.createAdvancedHTTPTransactions(endpoint, payloads, extra, transaction.TransactionPriorityHigh, transaction.Metadata, storableOnDisk)
		})
}

// SubmitAgentChecksMetadata will send a agentchecks_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, transaction.Metadata, extra,
		func(endpoint transaction.Endpoint, payloads transaction.BytesPayloads, _ transaction.Kind, extra http.Header) []*transaction.HTTPTransaction {
			// Agentchecks metadata contains the API KEY and should not be stored on disk.
			storableOnDisk := false
			return f.createAdvancedHTTPTransactions(endpoint, payloads, extra, transaction.TransactionPriorityNormal, transaction.Metadata, storableOnDisk)
		})
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *DefaultForwarder) SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(endpoints.V1MetadataEndpoint, payload, transaction.Metadata, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1Series(payloads transaction.BytesPayloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(endpoints.V1SeriesEndpoint, payloads, transaction.Series, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSeries will send timeseries to the v2 endpoint
func (f *DefaultForwarder) SubmitSeries(payloads transaction.BytesPayloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(endpoints.V1CheckRunsEndpoint, payload, transaction.CheckRuns, extra)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *DefaultForwarder) SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error {
	return f.submitV1IntakeWithTransactionsFactory(payload, kind, extra, f.createHTTPTransactions)
}

func (f *DefaultForwarder) submitV1IntakeWithTransactionsFactory(
	payload transaction.BytesPayloads,
	kind transaction.Kind,
	extra http.Header,
	createHTTPTransactions func(endpoint transaction.Endpoint, payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) []*transaction.HTTPTransaction,
) error {
	transactions := createHTTPTransactions(endpoints.V1IntakeEndpoint, payload, kind, extra)

	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitProcessChecks sends process checks
func (f *DefaultForwarder) SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.ProcessesEndpoint, payload, extra, true)
}

// SubmitProcessDiscoveryChecks sends process discovery checks
func (f *DefaultForwarder) SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.ProcessDiscoveryEndpoint, payload, extra, true)
}

// SubmitProcessEventChecks sends process events checks
func (f *DefaultForwarder) SubmitProcessEventChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.ProcessLifecycleEndpoint, payload, extra, true)
}

// SubmitRTProcessChecks sends real time process checks
func (f *DefaultForwarder) SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.RtProcessesEndpoint, payload, extra, false)
}

// SubmitContainerChecks sends container checks
func (f *DefaultForwarder) SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.ContainerEndpoint, payload, extra, true)
}

// SubmitRTContainerChecks sends real time container checks
func (f *DefaultForwarder) SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.RtContainerEndpoint, payload, extra, false)
}

// SubmitConnectionChecks sends connection checks
func (f *DefaultForwarder) SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.submitProcessLikePayload(endpoints.ConnectionsEndpoint, payload, extra, true)
}

// SubmitOrchestratorChecks sends orchestrator checks
func (f *DefaultForwarder) SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) (chan Response, error) {
	bumpOrchestratorPayload(f.log, payloadType)

	endpoint := endpoints.OrchestratorEndpoint
	if f.config.IsSet("orchestrator_explorer.use_legacy_endpoint") {
		endpoint = endpoints.LegacyOrchestratorEndpoint
	}

	return f.submitProcessLikePayload(endpoint, payload, extra, true)
}

// SubmitOrchestratorManifests sends orchestrator manifests
func (f *DefaultForwarder) SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	transactionsOrchestratorManifest.Add(1)
	return f.submitProcessLikePayload(endpoints.OrchestratorManifestEndpoint, payload, extra, true)
}

func (f *DefaultForwarder) submitProcessLikePayload(ep transaction.Endpoint, payload transaction.BytesPayloads, extra http.Header, retryable bool) (chan Response, error) {
	transactions := f.createHTTPTransactions(ep, payload, transaction.Process, extra)
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
				f.log.Errorf("timed out waiting for responses, received %d/%d", receivedResponses, expectedResponses)
				close(results)
				return
			}
		}
	}()

	return results, f.sendHTTPTransactions(transactions)
}
