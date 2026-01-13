// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package service represents the agent's core remoteconfig service
//
// The `CoreAgentService` type provides a communication layer for downstream
// clients to request configuration, as well as the ability to track clients for
// requesting complete update payloads from the remote config backend.
package service

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"expvar"
	"fmt"
	"io"
	"maps"
	"net/url"
	"path"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/go-tuf/data"
	tufutil "github.com/DataDog/go-tuf/util"
	"github.com/benbjohnson/clock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	defaultRefreshInterval  = 1 * time.Second
	minimalRefreshInterval  = 1 * time.Second
	defaultClientsTTL       = 30 * time.Second
	maxClientsTTL           = 60 * time.Second
	newClientBlockTTL       = 2 * time.Second
	defaultCacheBypassLimit = 5
	minCacheBypassLimit     = 1
	maxCacheBypassLimit     = 10
	orgStatusPollInterval   = 1 * time.Minute
	// Number of /configurations where we get 503 or 504 errors until the log level is increased to ERROR
	maxFetchConfigsUntilLogLevelErrors = 5
	// Number of /status calls where we get 503 or 504 errors until the log level is increased to ERROR
	maxFetchOrgStatusUntilLogLevelErrors = 5
)

// Constraints on the maximum backoff time when errors occur
const (
	minimalMaxBackoffTime = 2 * time.Minute
	maximalMaxBackoffTime = 5 * time.Minute
)

const (
	// When the agent continuously has the same authorization error when fetching RC updates
	// The first initialLogRefreshError are logged as ERROR, and then it's only logged as INFO
	initialFetchErrorLog uint64 = 5
)

var (
	exportedMapStatus = expvar.NewMap("remoteConfigStatus")
	// Status expvar exported
	exportedStatusOrgEnabled    = expvar.String{}
	exportedStatusKeyAuthorized = expvar.String{}
	exportedLastUpdateErr       = expvar.String{}
)

func getNewDirectorRoots(uptane uptaneClient, currentVersion uint64, newVersion uint64) ([][]byte, error) {
	var roots [][]byte
	for i := currentVersion + 1; i <= newVersion; i++ {
		root, err := uptane.DirectorRoot(i)
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func getTargetFiles(uptaneClient uptaneClient, targetFilePaths []string) ([]*pbgo.File, error) {
	files, err := uptaneClient.TargetFiles(targetFilePaths)
	if err != nil {
		return nil, err
	}

	var configFiles []*pbgo.File
	for path, contents := range files {
		// Note: This unconditionally succeeds as long as we don't change bufferDestination earlier
		configFiles = append(configFiles, &pbgo.File{
			Path: path,
			Raw:  contents,
		})
	}

	return configFiles, nil
}

// CoreAgentService implements rcservice.Component.
//
// It fetches and orchestrates the fetching of Remote Configurations from the RC
// backend.  and maintains the local state of the configuration on behalf of its
// clients.
type CoreAgentService struct {
	// rcType is used to differentiate multiple RC services running in a single
	// agent.  Today, it is simply logged as a prefix in all log messages to
	// help when triaging via logs.
	rcType string

	api           api.API
	clock         clock.Clock
	hostname      string
	tagsGetter    func() []string
	traceAgentEnv string
	agentVersion  string
	site          string
	configRoot    string
	directorRoot  string
	// We record the startup time to ensure we flush client configs if we can't
	// contact the backend in time
	startupTime           time.Time
	disableConfigPollLoop bool

	// The backoff policy used for retries when errors are encountered
	backoffPolicy backoff.Policy
	// Used to report metrics on cache bypass requests
	telemetryReporter RcTelemetryReporter
	// A background task used to perform connectivity tests using a WebSocket -
	// data gathering for future development.
	websocketTest startstop.StartStoppable

	// refreshBypassLimiter is used from the polling loop goroutine exclusively,
	// so it does not need to be synchronized.
	refreshBypassLimiter rateLimiter
	// refreshBypassCh is used from GetClientConfigs to trigger a refresh
	// request that may bypass the usual refresh loop's interval.
	refreshBypassCh chan<- chan<- struct{}

	clients         *clients
	orgStatusPoller *orgStatusPoller

	// Channels to stop the services main goroutines
	stopConfigPoller chan struct{}
	stopOnce         sync.Once

	// The set of products to which calls to CreateConfigSubscription can
	// subscribe. At the time of writing, this is a single-entry map in
	// production code, but is extended in some testing scenarios to exercise
	// subscriptions to differing product sets.
	subscriptionProductMappings productsMappings
	// The maximum number of subscriptions than can be open at the same time.
	maxConcurrentSubscriptions int
	// The maximum number of runtime IDs that may be tracked per subscription.
	maxTrackedRuntimeIDsPerSubscription int

	mu struct {
		sync.Mutex

		subscriptions *subscriptions

		uptane coreAgentUptaneClient

		firstUpdate bool

		defaultRefreshInterval         time.Duration
		refreshIntervalOverrideAllowed bool

		lastUpdateTimestamp time.Time
		lastUpdateErr       error

		// Used to rate limit the 4XX error logs
		fetchErrorCount    uint64
		lastFetchErrorType error

		// Number of /configurations calls where we get 503 or 504 errors.
		fetchConfigs503And504ErrCount uint64

		// The number of errors we're currently tracking within the context
		// of our backoff policy
		backoffErrorCount int

		products    map[rdata.Product]struct{}
		newProducts map[rdata.Product]struct{}
	}
}

// uptaneClient provides functions to get TUF/uptane repo data.
type uptaneClient interface {
	State() (uptane.State, error)
	DirectorRoot(version uint64) ([]byte, error)
	StoredOrgUUID() (string, error)
	Targets() (data.TargetFiles, error)
	TargetFile(path string) ([]byte, error)
	TargetFiles(files []string) (map[string][]byte, error)
	TargetsMeta() ([]byte, error)
	UnsafeTargetsMeta() ([]byte, error)
	TargetsCustom() ([]byte, error)
	TimestampExpires() (time.Time, error)
	TUFVersionState() (uptane.TUFVersions, error)
	Close() error
	GetTransactionalStoreMetadata() (*uptane.Metadata, error)
}

// coreAgentUptaneClient provides functions to get TUF/uptane repo data and update the agent's state via the RC backend.
type coreAgentUptaneClient interface {
	uptaneClient
	Update(response *pbgo.LatestConfigsResponse) error
}

// RcTelemetryReporter should be implemented by the agent to publish metrics
// on RC-specific events.
type RcTelemetryReporter interface {
	// IncRateLimit is invoked when a cache bypass request is prevented due to rate limiting
	IncRateLimit()
	// IncTimeout is invoked when a cache bypass request is cancelled due to timeout or a previous cache bypass request is still pending
	IncTimeout()

	// IncConfigSubscriptionsConnectedCounter increments the
	// DdRcTelemetryReporter ConfigSubscriptionsConnectedCounter counter.
	IncConfigSubscriptionsConnectedCounter()
	// IncConfigSubscriptionsDisconnectedCounter increments the
	// DdRcTelemetryReporter ConfigSubscriptionsDisconnectedCounter counter.
	IncConfigSubscriptionsDisconnectedCounter()
	// SetConfigSubscriptionsActive sets the DdRcTelemetryReporter
	// ConfigSubscriptionsActive gauge to the given value.
	SetConfigSubscriptionsActive(value int)
	// SetConfigSubscriptionClientsTracked sets the DdRcTelemetryReporter
	// ConfigSubscriptionClientsTracked gauge to the given value.
	SetConfigSubscriptionClientsTracked(value int)
}

// orgStatusPoller handles periodic polling of the organization status from the remote config backend
type orgStatusPoller struct {
	refreshInterval time.Duration
	stopChan        chan struct{}

	mu struct {
		sync.Mutex
		// Number of /status calls where we get 503 or 504 errors.
		fetchOrgStatus503And504ErrCount uint64
		// Previous /status response
		previousOrgStatus *pbgo.OrgStatusResponse
	}
}

func newOrgStatusPoller(refreshInterval time.Duration) *orgStatusPoller {
	p := &orgStatusPoller{
		refreshInterval: refreshInterval,
		stopChan:        make(chan struct{}),
	}
	return p
}

// start begins the periodic polling of org status
func (p *orgStatusPoller) start(clock clock.Clock, apiClient api.API, rcType string) {
	go func() {
		timer := clock.Timer(0)
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				p.poll(apiClient, rcType)
				timer.Reset(p.refreshInterval)
			case <-p.stopChan:
				log.Infof("[%s] Stopping Remote Config org status poller", rcType)
				return
			}
		}
	}()
}

// stop stops the org status poller
func (p *orgStatusPoller) stop() {
	close(p.stopChan)
}

// getPreviousStatus returns the previous org status response, if any
func (p *orgStatusPoller) getPreviousStatus() *pbgo.OrgStatusResponse {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mu.previousOrgStatus
}

// poll fetches and processes the current organization status
func (p *orgStatusPoller) poll(apiClient api.API, rcType string) {
	response, err := apiClient.FetchOrgStatus(context.Background())

	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		// Unauthorized and proxy error are caught by the main loop requesting the latest config
		if errors.Is(err, api.ErrUnauthorized) || errors.Is(err, api.ErrProxy) {
			return
		}

		if errors.Is(err, api.ErrGatewayTimeout) || errors.Is(err, api.ErrServiceUnavailable) {
			p.mu.fetchOrgStatus503And504ErrCount++
		}

		if p.mu.fetchOrgStatus503And504ErrCount < maxFetchOrgStatusUntilLogLevelErrors {
			log.Warnf("[%s] Could not refresh Remote Config: %v", rcType, err)
		} else {
			log.Errorf("[%s] Could not refresh Remote Config: %v", rcType, err)
		}
		return
	}
	p.mu.fetchOrgStatus503And504ErrCount = 0

	// Print info log when the new status is different from the previous one, or if it's the first run
	prev := p.mu.previousOrgStatus
	if prev == nil ||
		prev.Enabled != response.Enabled ||
		prev.Authorized != response.Authorized {
		if response.Enabled {
			if response.Authorized {
				log.Infof("[%s] Remote Configuration is enabled for this organization and agent.", rcType)
			} else {
				log.Infof(
					"[%s] Remote Configuration is enabled for this organization but disabled for this agent. Add the Remote Configuration Read permission to its API key to enable it for this agent.", rcType)
			}
		} else {
			if response.Authorized {
				log.Infof("[%s] Remote Configuration is disabled for this organization.", rcType)
			} else {
				log.Infof("[%s] Remote Configuration is disabled for this organization and agent.", rcType)
			}
		}
	}
	p.mu.previousOrgStatus = &pbgo.OrgStatusResponse{
		Enabled:    response.Enabled,
		Authorized: response.Authorized,
	}
	exportedStatusOrgEnabled.Set(strconv.FormatBool(response.Enabled))
	exportedStatusKeyAuthorized.Set(strconv.FormatBool(response.Authorized))
}

func init() {
	// Exported variable to get the state of remote-config
	exportedMapStatus.Init()
	exportedMapStatus.Set("orgEnabled", &exportedStatusOrgEnabled)
	exportedMapStatus.Set("apiKeyScoped", &exportedStatusKeyAuthorized)
	exportedMapStatus.Set("lastError", &exportedLastUpdateErr)
}

type options struct {
	site                           string
	rcKey                          string
	apiKey                         string
	parJWT                         string
	traceAgentEnv                  string
	databaseFileName               string
	databaseFilePath               string
	configRootOverride             string
	directorRootOverride           string
	clientCacheBypassLimit         int
	refresh                        time.Duration
	refreshIntervalOverrideAllowed bool
	maxBackoff                     time.Duration
	clientTTL                      time.Duration
	disableConfigPollLoop          bool
	orgStatusRefreshInterval       time.Duration
	// for mocking creating db instance in test
	uptaneFactory func(md *uptane.Metadata) (coreAgentUptaneClient, error)
	// The allowed products for each subscription products value. Overridable
	// to exercise subscriptions to differing product sets in tests.
	subscriptionProductMappings productsMappings
	// The maximum number of subscriptions than can be open at the same time.
	maxConcurrentSubscriptions int
	// The maximum number of runtime IDs that may be tracked per subscription.
	maxTrackedRuntimeIDsPerSubscription int
	// The maximum number of responses that can be queued per subscription.
	maxSubscriptionQueueSize int
}

var defaultSubscriptionProductMappings = productsMappings{
	pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING: {
		rdata.ProductLiveDebugging:         {},
		rdata.ProductLiveDebuggingSymbolDB: {},
	},
}

var defaultOptions = options{
	rcKey:                               "",
	apiKey:                              "",
	parJWT:                              "",
	traceAgentEnv:                       "",
	databaseFileName:                    "remote-config.db",
	databaseFilePath:                    "",
	configRootOverride:                  "",
	directorRootOverride:                "",
	clientCacheBypassLimit:              defaultCacheBypassLimit,
	refresh:                             defaultRefreshInterval,
	refreshIntervalOverrideAllowed:      false,
	maxBackoff:                          minimalMaxBackoffTime,
	clientTTL:                           defaultClientsTTL,
	disableConfigPollLoop:               false,
	orgStatusRefreshInterval:            defaultRefreshInterval,
	subscriptionProductMappings:         defaultSubscriptionProductMappings,
	maxConcurrentSubscriptions:          defaultMaxConcurrentSubscriptions,
	maxTrackedRuntimeIDsPerSubscription: defaultMaxTrackedRuntimeIDsPerSubscription,
	maxSubscriptionQueueSize:            defaultMaxSubscriptionQueueSize,
}

// Option is a service option
type Option func(s *options)

// WithTraceAgentEnv sets the service trace-agent environment variable
func WithTraceAgentEnv(env string) func(s *options) {
	return func(s *options) { s.traceAgentEnv = env }
}

// WithDatabaseFileName sets the service database file name
func WithDatabaseFileName(fileName string) func(s *options) {
	return func(s *options) { s.databaseFileName = fileName }
}

// WithDatabasePath sets the service database path
func WithDatabasePath(path string) func(s *options) {
	return func(s *options) { s.databaseFilePath = path }
}

// WithConfigRootOverride sets the service config root override
func WithConfigRootOverride(site string, override string) func(s *options) {
	return func(opts *options) {
		opts.site = site
		opts.configRootOverride = override
	}
}

// WithDirectorRootOverride sets the service director root override
func WithDirectorRootOverride(site string, override string) func(s *options) {
	return func(opts *options) {
		opts.site = site
		opts.directorRootOverride = override
	}
}

// WithRefreshInterval validates and sets the service refresh interval
func WithRefreshInterval(interval time.Duration, cfgPath string) func(s *options) {
	if interval < minimalRefreshInterval {
		log.Warnf("%s is set to %v which is below the minimum of %v - using default refresh interval %v", cfgPath, interval, minimalRefreshInterval, defaultRefreshInterval)
		return func(s *options) {
			s.refresh = defaultRefreshInterval
			s.refreshIntervalOverrideAllowed = true
		}
	}
	return func(s *options) {
		s.refresh = interval
		s.refreshIntervalOverrideAllowed = false
	}
}

// WithOrgStatusRefreshInterval validates and sets the service org status refresh interval
func WithOrgStatusRefreshInterval(interval time.Duration, cfgPath string) func(s *options) {
	if interval < minimalRefreshInterval {
		log.Warnf("%s is set to %v which is below the minimum of %v - using default org status refresh interval %v",
			cfgPath, interval, minimalRefreshInterval, defaultRefreshInterval)
		return func(s *options) {
			s.orgStatusRefreshInterval = defaultRefreshInterval
		}
	}

	return func(s *options) {
		s.orgStatusRefreshInterval = interval
	}
}

// WithMaxBackoffInterval validates sets the service maximum retry backoff time
func WithMaxBackoffInterval(interval time.Duration, cfgPath string) func(s *options) {
	if interval < minimalMaxBackoffTime {
		log.Warnf("%s is set to %v which is below the minimum of %v - setting value to %v", cfgPath, interval, minimalMaxBackoffTime, minimalMaxBackoffTime)
		return func(s *options) {
			s.maxBackoff = minimalMaxBackoffTime
		}
	} else if interval > maximalMaxBackoffTime {
		log.Warnf("%s is set to %v which is above the maximum of %v - setting value to %v", cfgPath, interval, maximalMaxBackoffTime, maximalMaxBackoffTime)
		return func(s *options) {
			s.maxBackoff = maximalMaxBackoffTime
		}
	}

	return func(s *options) {
		s.maxBackoff = interval
	}
}

// WithRcKey sets the service remote configuration key
func WithRcKey(rcKey string) func(s *options) {
	return func(s *options) { s.rcKey = rcKey }
}

// WithAPIKey sets the service API key
func WithAPIKey(apiKey string) func(s *options) {
	return func(s *options) { s.apiKey = apiKey }
}

// WithPARJWT sets the JWT for the private action runner
func WithPARJWT(jwt string) func(s *options) {
	return func(s *options) { s.parJWT = jwt }
}

// WithClientCacheBypassLimit validates and sets the service client cache bypass limit
func WithClientCacheBypassLimit(limit int, cfgPath string) func(s *options) {
	if limit < minCacheBypassLimit || limit > maxCacheBypassLimit {
		log.Warnf(
			"%s is not within accepted range (%d - %d): %d. Defaulting to %d",
			cfgPath, minCacheBypassLimit, maxCacheBypassLimit, limit, defaultCacheBypassLimit,
		)
		return func(s *options) {
			s.clientCacheBypassLimit = defaultCacheBypassLimit
		}
	}
	return func(s *options) {
		s.clientCacheBypassLimit = limit
	}
}

// WithClientTTL validates and sets the service client TTL
func WithClientTTL(interval time.Duration, cfgPath string) func(s *options) {
	if interval < minimalRefreshInterval || interval > maxClientsTTL {
		log.Warnf("%s is not within accepted range (%s - %s): %s. Defaulting to %s", cfgPath, minimalRefreshInterval, maxClientsTTL, interval, defaultClientsTTL)
		return func(s *options) {
			s.clientTTL = defaultClientsTTL
		}
	}
	return func(s *options) {
		s.clientTTL = interval
	}
}

// WithAgentPollLoopDisabled disables the config poll loop
func WithAgentPollLoopDisabled() func(s *options) {
	return func(s *options) {
		s.disableConfigPollLoop = true
	}
}

// withUptaneFactory creates a mock version for testing
func withUptaneFactory(f func(md *uptane.Metadata) (coreAgentUptaneClient, error)) Option {
	return func(o *options) { o.uptaneFactory = f }
}

// NewService instantiates a new remote configuration management service
func NewService(cfg model.Reader, rcType, baseRawURL, hostname string, tagsGetter func() []string, telemetryReporter RcTelemetryReporter, agentVersion string, opts ...Option) (*CoreAgentService, error) {
	options := defaultOptions
	for _, opt := range opts {
		opt(&options)
	}

	// A backoff is calculated as a range from which a random value will be selected. The formula is as follows.
	//
	// min = baseBackoffTime * 2^<NumErrors> / minBackoffFactor
	// max = min(maxBackoffTime, baseBackoffTime * 2 ^<NumErrors>)
	//
	// The following values mean each range will always be [30*2^<NumErrors-1>, min(maxBackoffTime, 30*2^<NumErrors>)].
	// Every success will cause numErrors to shrink by 2.
	// This is a sensible default backoff pattern, and there isn't really any need to
	// let clients configure this at this time.
	minBackoffFactor := 2.0
	baseBackoffTime := 30.0
	recoveryInterval := 2
	recoveryReset := false

	backoffPolicy := backoff.NewExpBackoffPolicy(minBackoffFactor, baseBackoffTime,
		options.maxBackoff.Seconds(), recoveryInterval, recoveryReset)

	authKeys, err := getRemoteConfigAuthKeys(options.apiKey, options.rcKey, options.parJWT)
	if err != nil {
		return nil, err
	}

	baseURL, err := url.Parse(baseRawURL)
	if err != nil {
		return nil, err
	}
	http, err := api.NewHTTPClient(authKeys.apiAuth(), cfg, baseURL)
	if err != nil {
		return nil, err
	}

	databaseFilePath := cfg.GetString("run_path")
	if options.databaseFilePath != "" {
		databaseFilePath = options.databaseFilePath
	}

	dbMetadata := &uptane.Metadata{
		Path:         path.Join(databaseFilePath, options.databaseFileName),
		AgentVersion: agentVersion,
		APIKey:       authKeys.apiKey,
		URL:          baseURL.String(),
	}

	configRoot := options.configRootOverride
	directorRoot := options.directorRootOverride
	opt := []uptane.ClientOption{
		uptane.WithConfigRootOverride(options.site, configRoot),
		uptane.WithDirectorRootOverride(options.site, directorRoot),
	}
	if authKeys.rcKeySet {
		opt = append(opt, uptane.WithOrgIDCheck(authKeys.rcKey.OrgID))
	}

	var uptaneClient coreAgentUptaneClient
	if options.uptaneFactory != nil {
		// this allows us to create an uptane client without opening real bolt db for tests
		uptaneClient, err = options.uptaneFactory(dbMetadata)
	} else {
		uptaneClient, err = uptane.NewCoreAgentClientWithNewTransactionalStore(
			dbMetadata,
			newRCBackendOrgUUIDProvider(http),
			opt...,
		)
	}
	if err != nil {
		return nil, err
	}

	clock := clock.New()

	// WebSocket test actor - must call Start() to spawn the background task.
	var websocketTest startstop.StartStoppable
	if cfg.GetBool("remote_configuration.no_websocket_echo") {
		websocketTest = &noOpRunnable{}
	} else {
		websocketTest = NewWebSocketTestActor(http)
	}

	now := clock.Now().UTC()
	cas := &CoreAgentService{
		api:                   http,
		rcType:                rcType,
		startupTime:           now,
		hostname:              hostname,
		tagsGetter:            tagsGetter,
		clock:                 clock,
		traceAgentEnv:         options.traceAgentEnv,
		agentVersion:          agentVersion,
		stopConfigPoller:      make(chan struct{}),
		disableConfigPollLoop: options.disableConfigPollLoop,
		site:                  options.site,
		configRoot:            configRoot,
		directorRoot:          directorRoot,
		backoffPolicy:         backoffPolicy,
		telemetryReporter:     telemetryReporter,
		websocketTest:         websocketTest,
		clients:               newClients(clock, options.clientTTL),
		refreshBypassLimiter: rateLimiter{
			clock: clock,

			// By default, allows for 5 cache bypass every refreshInterval seconds
			// in addition to the usual refresh.
			currentWindow:  time.Now().UTC(),
			windowDuration: options.refresh,
			capacity:       options.clientCacheBypassLimit,
			allowance:      options.clientCacheBypassLimit,
		},
		subscriptionProductMappings:         options.subscriptionProductMappings,
		maxConcurrentSubscriptions:          options.maxConcurrentSubscriptions,
		maxTrackedRuntimeIDsPerSubscription: options.maxTrackedRuntimeIDsPerSubscription,
		orgStatusPoller:                     newOrgStatusPoller(options.orgStatusRefreshInterval),
	}
	cas.mu.subscriptions = newSubscriptions(
		options.subscriptionProductMappings,
		options.maxSubscriptionQueueSize,
	)
	cas.mu.firstUpdate = true
	cas.mu.defaultRefreshInterval = options.refresh
	cas.mu.refreshIntervalOverrideAllowed = options.refreshIntervalOverrideAllowed
	cas.mu.backoffErrorCount = 0
	cas.mu.products = make(map[rdata.Product]struct{})
	cas.mu.newProducts = make(map[rdata.Product]struct{})
	cas.mu.uptane = uptaneClient

	cfg.OnUpdate(cas.apiKeyUpdateCallback())

	return cas, nil
}

func newRCBackendOrgUUIDProvider(http api.API) uptane.OrgUUIDProvider {
	return func() (string, error) {
		// XXX: We may want to tune the context timeout here
		resp, err := http.FetchOrgData(context.Background())
		return resp.GetUuid(), err
	}
}

// Start the remote configuration management service
func (s *CoreAgentService) Start() {
	refreshBypassCh := make(chan chan<- struct{})
	s.refreshBypassCh = refreshBypassCh
	s.orgStatusPoller.start(s.clock, s.api, s.rcType)

	go func() {
		if s.disableConfigPollLoop {
			startWithoutAgentPollLoop(s, refreshBypassCh)
		} else {
			startWithAgentPollLoop(s, refreshBypassCh)
		}
	}()

	s.websocketTest.Start()
}

// UpdatePARJWT updates the stored JWT for Private Action Runners
// for authentication to the remote config backend.
func (s *CoreAgentService) UpdatePARJWT(jwt string) {
	s.api.UpdatePARJWT(jwt)
}

func startWithAgentPollLoop(s *CoreAgentService, refreshBypassRequests <-chan chan<- struct{}) {
	err := s.refresh()
	if err != nil {
		s.logRefreshError(err)
	}

	for {
		var err error
		refreshInterval := s.calculateRefreshInterval()
		select {
		case <-s.clock.After(refreshInterval):
			err = s.refresh()
		// New clients detected, request refresh
		case response := <-refreshBypassRequests:
			if !s.refreshBypassLimiter.Limit() {
				err = s.refresh()
			} else {
				s.telemetryReporter.IncRateLimit()
			}
			close(response)
		case <-s.stopConfigPoller:
			log.Infof("[%s] Stopping Remote Config configuration poller", s.rcType)
			return
		}

		if err != nil {
			s.logRefreshError(err)
		}
	}
}

func startWithoutAgentPollLoop(s *CoreAgentService, refreshBypassRequests <-chan chan<- struct{}) {
	for {
		var err error
		response := <-refreshBypassRequests
		if !s.refreshBypassLimiter.Limit() {
			err = s.refresh()
		} else {
			err = errors.New("cache bypass limit exceeded")
			s.mu.Lock()
			s.mu.lastUpdateErr = err
			s.mu.Unlock()
			s.telemetryReporter.IncRateLimit()
		}
		close(response)
		if err != nil {
			s.logRefreshError(err)
		}
	}
}

func (s *CoreAgentService) logRefreshError(err error) {
	prev := s.orgStatusPoller.getPreviousStatus()
	if prev != nil && prev.Enabled && prev.Authorized {
		fetchConfigs503And504ErrCount := func() uint64 {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.mu.fetchConfigs503And504ErrCount
		}()
		exportedLastUpdateErr.Set(err.Error())
		if fetchConfigs503And504ErrCount < maxFetchConfigsUntilLogLevelErrors {
			log.Warnf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
		} else {
			log.Errorf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
		}
	} else {
		log.Debugf("[%s] Could not refresh Remote Config (org is disabled or key is not authorized): %v", s.rcType, err)
	}
}

// Stop stops the refresh loop and closes the on-disk DB cache
func (s *CoreAgentService) Stop() error {
	var err error
	s.stopOnce.Do(func() {
		s.websocketTest.Stop()
		s.orgStatusPoller.stop()
		close(s.stopConfigPoller)

		s.mu.Lock()
		defer s.mu.Unlock()
		// close boltDB via the transactional store
		err = s.mu.uptane.Close()
	})
	return err
}

func (s *CoreAgentService) calculateRefreshInterval() time.Duration {
	s.mu.Lock()
	backoffErrorCount := s.mu.backoffErrorCount
	defaultRefreshInterval := s.mu.defaultRefreshInterval
	s.mu.Unlock()

	backoffTime := s.backoffPolicy.GetBackoffDuration(backoffErrorCount)

	return defaultRefreshInterval + backoffTime
}

func (s *CoreAgentService) refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// We can't let the backend process an update twice in the same second due to the fact that we
	// use the epoch with seconds resolution as the version for the TUF Director Targets. If this happens,
	// the update will appear to TUF as being identical to the previous update and it will be dropped.
	earliestNextUpdate := s.mu.lastUpdateTimestamp.Add(time.Second)
	if now := s.clock.Now(); now.Before(earliestNextUpdate) {
		func() {
			s.mu.Unlock()
			defer s.mu.Lock()
			delay := earliestNextUpdate.Sub(now)
			log.Debugf("Requests too frequent, delaying by %v", delay)
			s.clock.Sleep(delay)
		}()
	}

	activeClients := s.clients.activeClients()
	s.refreshProductsLocked(activeClients)
	previousState, err := s.mu.uptane.TUFVersionState()
	if err != nil {
		log.Warnf("[%s] could not get previous TUF version state: %v", s.rcType, err)
	}
	if s.mu.firstUpdate || err != nil {
		previousState = uptane.TUFVersions{}
	}
	var clientState []byte
	if rawTargetsCustom, err := s.mu.uptane.TargetsCustom(); err != nil {
		log.Warnf("[%s] could not get previous backend client state: %v", s.rcType, err)
	} else if custom, err := parseTargetsCustom(rawTargetsCustom); err != nil {
		log.Warnf("[%s] could not parse targets custom: %v", s.rcType, err)
	} else {
		clientState = custom.OpaqueBackendState
	}
	orgUUID, err := s.mu.uptane.StoredOrgUUID()
	if err != nil {
		return err
	}

	request := buildLatestConfigsRequest(s.hostname, s.agentVersion, s.tagsGetter(), s.traceAgentEnv, orgUUID, previousState, activeClients, s.mu.products, s.mu.newProducts, s.mu.lastUpdateErr, clientState)
	var response *pbgo.LatestConfigsResponse
	func() {
		s.mu.Unlock()
		defer s.mu.Lock()
		ctx := context.Background()
		response, err = s.api.Fetch(ctx, request)
	}()
	s.mu.lastUpdateErr = nil
	if err != nil {
		s.mu.backoffErrorCount = s.backoffPolicy.IncError(s.mu.backoffErrorCount)
		s.mu.lastUpdateErr = fmt.Errorf("api: %v", err)
		if s.mu.lastFetchErrorType != err {
			s.mu.lastFetchErrorType = err
			s.mu.fetchErrorCount = 0
		}

		if errors.Is(err, api.ErrUnauthorized) || errors.Is(err, api.ErrProxy) {
			if s.mu.fetchErrorCount < initialFetchErrorLog {
				s.mu.fetchErrorCount++
				return err
			}
			// If we saw the error enough time, we consider that RC not working is a normal behavior
			// And we only log as DEBUG
			// The agent will eventually log this error as DEBUG every maximalMaxBackoffTime
			log.Debugf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
			return nil
		}

		if errors.Is(err, api.ErrGatewayTimeout) || errors.Is(err, api.ErrServiceUnavailable) {
			s.mu.fetchConfigs503And504ErrCount++
		}
		return err
	}
	s.mu.fetchErrorCount = 0
	s.mu.fetchConfigs503And504ErrCount = 0
	err = s.mu.uptane.Update(response)
	if err != nil {
		s.mu.backoffErrorCount = s.backoffPolicy.IncError(s.mu.backoffErrorCount)
		s.mu.lastUpdateErr = fmt.Errorf("tuf: %v", err)
		return err
	}
	// If a user hasn't explicitly set the refresh interval, allow the backend to override it based
	// on the contents of our update request
	if s.mu.refreshIntervalOverrideAllowed {
		ri, err := s.getRefreshIntervalLocked()
		if err == nil && ri > 0 && s.mu.defaultRefreshInterval != ri {
			s.mu.defaultRefreshInterval = ri
			s.refreshBypassLimiter.windowDuration = ri
			log.Infof("[%s] Overriding agent's base refresh interval to %v due to backend recommendation", s.rcType, ri)
		}
	}

	s.mu.lastUpdateTimestamp = s.clock.Now()

	s.mu.firstUpdate = false
	for product := range s.mu.newProducts {
		s.mu.products[product] = struct{}{}
	}
	s.mu.newProducts = make(map[rdata.Product]struct{})

	s.mu.backoffErrorCount = s.backoffPolicy.DecError(s.mu.backoffErrorCount)

	exportedLastUpdateErr.Set("")

	return nil
}

func (s *CoreAgentService) refreshProductsLocked(activeClients []*pbgo.Client) {
	for _, client := range activeClients {
		for _, product := range client.Products {
			if _, hasProduct := s.mu.products[rdata.Product(product)]; !hasProduct {
				s.mu.newProducts[rdata.Product(product)] = struct{}{}
			}
		}
	}
}

func (s *CoreAgentService) getRefreshIntervalLocked() (time.Duration, error) {
	rawTargetsCustom, err := s.mu.uptane.TargetsCustom()
	if err != nil {
		return 0, err
	}
	custom, err := parseTargetsCustom(rawTargetsCustom)
	if err != nil {
		return 0, err
	}

	// We only allow intervals to be overridden if they are between [1s, 1m]
	value := time.Second * time.Duration(custom.AgentRefreshInterval)
	if value < time.Second || value > time.Minute {
		return 0, nil
	}

	return value, nil
}

func (s *CoreAgentService) flushCacheResponseLocked() (*pbgo.ClientGetConfigsResponse, error) {
	targets, err := s.mu.uptane.UnsafeTargetsMeta()
	if err != nil {
		return nil, err
	}
	return &pbgo.ClientGetConfigsResponse{
		Roots:         nil,
		Targets:       targets,
		TargetFiles:   nil,
		ClientConfigs: nil,
		ConfigStatus:  pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED,
	}, nil
}

// ClientGetConfigs is the polling API called by tracers and agents to get the latest configurations
//
//nolint:revive // TODO(RC) Fix revive linter
func (s *CoreAgentService) ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := validateRequest(request)
	if err != nil {
		return nil, err
	}

	if !s.clients.active(request.Client) {
		// Trigger a bypass to directly get configurations from the agent.
		// This will timeout to avoid blocking the tracer if:
		// - The previous request is still pending
		// - The triggered request takes too long

		s.clients.seen(request.Client)
		s.mu.Unlock()

		response := make(chan struct{})
		bypassStart := s.clock.Now()

		log.Debugf("Making bypass request for client %s", request.Client.GetId())

		// Timeout in case the previous request is still pending
		// and we can't request another one
		select {
		case s.refreshBypassCh <- response:
		case <-s.clock.After(newClientBlockTTL):
			// No need to add telemetry here, it'll be done in the second
			// timeout case that will automatically be triggered
		}

		partialNewClientBlockTTL := newClientBlockTTL - s.clock.Since(bypassStart)

		// Timeout if the response is taking too long
		select {
		case <-response:
		case <-s.clock.After(partialNewClientBlockTTL):
			log.Debugf("Bypass request timed out for client %s", request.Client.GetId())
			s.telemetryReporter.IncTimeout()
		}

		s.mu.Lock()
	}
	if s.disableConfigPollLoop && s.mu.lastUpdateErr != nil {
		return nil, s.mu.lastUpdateErr
	}

	s.clients.seen(request.Client)
	tufVersions, err := s.mu.uptane.TUFVersionState()
	if err != nil {
		return nil, err
	}

	// We only want to check for this if we have successfully initialized the TUF database
	if !s.mu.firstUpdate {

		// get the expiration time of timestamp.json
		expires, err := s.mu.uptane.TimestampExpires()
		if err != nil {
			return nil, err
		}
		// If timestamp.json has expired and we've waited to ensure connection to the backend,
		// all clients must flush their configuration state.
		if expires.Before(s.clock.Now()) {
			log.Warnf("Timestamp expired at %s, flushing cache", expires.Format(time.RFC3339))
			return s.flushCacheResponseLocked()
		}
	}

	// The rest of the method works to serve the client's request while keeping
	// subscriptions consistent if they need any additional files. The protocol
	// is as follows:
	//
	//	 1) Check if the client needs a TUF targets update, or if there are any
	//	 subscriptions that need a complete view of any products for this
	//   client.
	//	   a) No update is needed and no subscriptions need a complete view,
	//	      return an empty response.
	//	   b) Update is needed or subscriptions need a complete view, continue.
	//	 2) Compute two sets of configs:
	//	   * responseConfigs — files the client is missing vs its cache.
	//	   * subscriptionOnlyConfigs — extra files required to give some
	//	     products a complete first view of this client, but not needed
	//	     for the client's response.
	//	 3) Fetch all the files needed for both sets from the uptane client.
	//	 4) Split the response files from allFiles.
	//	 5) Notify subscriptions with the relevant files.
	//	 6) Return the client's response.
	//	   a) If there is no TUF update to ship, return an empty response.
	//	   b) Otherwise, include new TUF metadata and the client's needed files.

	// (1) Check if either the client or any subscriptions need anything.
	hasUpdate := tufVersions.DirectorTargets != request.Client.State.TargetsVersion
	interestedSubs, productsNeededForSubs := s.mu.subscriptions.interestedSubscriptions(
		request.Client,
	)

	if !hasUpdate && len(productsNeededForSubs) == 0 {
		// (1.a) Neither the client nor any subscriptions need anything.
		return &pbgo.ClientGetConfigsResponse{}, nil
	}

	// (2) Build responseConfigs and subscriptionOnlyConfigs.
	roots, err := getNewDirectorRoots(s.mu.uptane, request.Client.State.RootVersion, tufVersions.DirectorRoot)
	if err != nil {
		return nil, err
	}

	directorTargets, err := s.mu.uptane.Targets()
	if err != nil {
		return nil, err
	}
	matchedClientConfigs, err := executeTracerPredicates(request.Client, directorTargets)
	if err != nil {
		return nil, err
	}
	cachedTargetsMap, err := makeFileMetaMap(request.CachedTargetFiles)
	if err != nil {
		return nil, err
	}
	var subscriptionOnlyConfigs map[string]struct{}
	for _, config := range matchedClientConfigs {
		if _, ok := productsNeededForSubs[productFromPath(config)]; ok {
			if subscriptionOnlyConfigs == nil {
				subscriptionOnlyConfigs = make(map[string]struct{})
			}
			subscriptionOnlyConfigs[config] = struct{}{}
		}
	}
	responseConfigs := filtered(matchedClientConfigs, func(config string) bool {
		return tufutil.FileMetaEqual(
			cachedTargetsMap[config],
			directorTargets[config].FileMeta,
		) != nil
	})
	for _, config := range responseConfigs {
		delete(subscriptionOnlyConfigs, config)
	}

	// (3) Fetch all the files needed for both sets from the uptane client.
	allConfigs := slices.AppendSeq(responseConfigs, maps.Keys(subscriptionOnlyConfigs))
	slices.Sort(allConfigs)
	allFiles, err := getTargetFiles(s.mu.uptane, allConfigs)
	if err != nil {
		return nil, err
	}

	// (4) Move the extra files needed just for subscriptions to the end.
	slices.SortStableFunc(allFiles, func(a, b *pbgo.File) int {
		return cmp.Compare(
			boolToInt(contains(subscriptionOnlyConfigs, a.Path)),
			boolToInt(contains(subscriptionOnlyConfigs, b.Path)),
		)
	})

	// (5) Notify subscriptions with the relevant files.
	responseFiles := allFiles[:len(responseConfigs)]
	if len(interestedSubs) > 0 {
		s.mu.subscriptions.notify(
			interestedSubs,
			request.Client,
			matchedClientConfigs,
			responseFiles,
			allFiles,
		)
	}

	// (6.a) If there is no TUF update to ship, return an empty response.
	if !hasUpdate {
		return &pbgo.ClientGetConfigsResponse{}, nil
	}

	// (6.b) Fill out and return the client's response.
	targetsRaw, err := s.mu.uptane.TargetsMeta()
	if err != nil {
		return nil, err
	}
	return &pbgo.ClientGetConfigsResponse{
		Roots:         roots,
		Targets:       targetsRaw,
		TargetFiles:   responseFiles,
		ClientConfigs: matchedClientConfigs,
		ConfigStatus:  pbgo.ConfigStatus_CONFIG_STATUS_OK,
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func makeFileMetaMap(targetFileMetas []*pbgo.TargetFileMeta) (map[string]data.FileMeta, error) {
	cachedTargetsMap := make(map[string]data.FileMeta, len(targetFileMetas))
	for _, fileMeta := range targetFileMetas {
		hashes := make(data.Hashes, len(fileMeta.Hashes))
		for _, hash := range fileMeta.Hashes {
			h, err := hex.DecodeString(hash.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to decode hash: %w", err)
			}
			hashes[hash.Algorithm] = h
		}
		cachedTargetsMap[fileMeta.Path] = data.FileMeta{
			Hashes: hashes,
			Length: fileMeta.Length,
		}
	}
	return cachedTargetsMap, nil
}

func (s *CoreAgentService) apiKeyUpdateCallback() func(string, model.Source, any, any, uint64) {
	return func(setting string, _ model.Source, _, newvalue any, _ uint64) {
		if setting != "api_key" {
			return
		}

		newKey, ok := newvalue.(string)

		if !ok {
			log.Errorf("Could not convert API key to string")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()

		s.api.UpdateAPIKey(newKey)

		// Verify that the Org UUID hasn't changed
		storedOrgUUID, err := s.mu.uptane.StoredOrgUUID()
		if err != nil {
			log.Warnf("Could not get org uuid: %s", err)
			return
		}

		// TODO: Do not hold the mutex while calling FetchOrgData.
		newOrgUUID, err := s.api.FetchOrgData(context.Background())
		if err != nil {
			log.Warnf("Could not get org uuid: %s", err)
			return
		}

		if storedOrgUUID != newOrgUUID.Uuid {
			log.Errorf("Error switching API key: new API key is from a different organization")
		}
	}
}

// ConfigGetState returns the state of the configuration and the director repos in the local store
func (s *CoreAgentService) ConfigGetState() (*pbgo.GetStateConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uptane := s.mu.uptane

	state, err := uptane.State()
	if err != nil {
		return nil, err
	}

	response := &pbgo.GetStateConfigResponse{
		ConfigState:     map[string]*pbgo.FileMetaState{},
		DirectorState:   map[string]*pbgo.FileMetaState{},
		TargetFilenames: map[string]string{},
		ActiveClients:   s.clients.activeClients(),
	}

	for metaName, metaState := range state.ConfigState {
		response.ConfigState[metaName] = &pbgo.FileMetaState{Version: metaState.Version, Hash: metaState.Hash}
	}

	for metaName, metaState := range state.DirectorState {
		response.DirectorState[metaName] = &pbgo.FileMetaState{Version: metaState.Version, Hash: metaState.Hash}
	}

	maps.Copy(response.TargetFilenames, state.TargetFilenames)

	for _, subscription := range s.mu.subscriptions.subs {
		trackedClients := make([]*pbgo.ConfigSubscriptionState_TrackedClient, 0, len(subscription.trackedClients))
		for runtimeID, trackedClient := range subscription.trackedClients {
			trackedClients = append(trackedClients, &pbgo.ConfigSubscriptionState_TrackedClient{
				RuntimeId: runtimeID,
				SeenAny:   trackedClient.seenAny,
				Products:  trackedClient.products,
			})
		}
		slices.SortFunc(trackedClients, func(a, b *pbgo.ConfigSubscriptionState_TrackedClient) int {
			return cmp.Compare(a.RuntimeId, b.RuntimeId)
		})
		response.ConfigSubscriptionStates = append(response.ConfigSubscriptionStates, &pbgo.ConfigSubscriptionState{
			TrackedClients: trackedClients,
		})
	}

	return response, nil
}

// ConfigResetState resets the remote configuration state, clearing the local store and reinitializing the uptane client
func (s *CoreAgentService) ConfigResetState() (*pbgo.ResetStateConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// get metadata from current ts to recreate it with same params
	metadata, err := s.mu.uptane.GetTransactionalStoreMetadata()
	if err != nil {
		return nil, err
	}

	opt := []uptane.ClientOption{
		uptane.WithConfigRootOverride(s.site, s.configRoot),
		uptane.WithDirectorRootOverride(s.site, s.directorRoot),
	}
	uptaneClient, err := uptane.NewCoreAgentClientWithRecreatedTransactionalStore(
		metadata,
		newRCBackendOrgUUIDProvider(s.api),
		opt...,
	)
	if err != nil {
		return nil, err
	}
	s.mu.uptane = uptaneClient

	return &pbgo.ResetStateConfigResponse{}, err
}

// CreateConfigSubscription creates a new config subscription for a client.
//
// The client can send requests to track or untrack products for clients with a
// given ID. If a client is tracked, all files for products that the
// subscription tracks should be streamed to this subscription when that client
// would receive those files via the regular ClientGetConfigs RPC.
func (s *CoreAgentService) CreateConfigSubscription(
	stream pbgo.AgentSecure_CreateConfigSubscriptionServer,
) error {
	// Register this subscription, checking for limits on total subscriptions.
	subID, updateSignal, err := func() (subscriptionID, <-chan struct{}, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.mu.subscriptions.subs) >= s.maxConcurrentSubscriptions {
			return 0, nil, status.Errorf(
				codes.ResourceExhausted,
				"maximum number of subscriptions reached (%d)",
				s.maxConcurrentSubscriptions,
			)
		}
		subID, updateSignal := s.mu.subscriptions.newSubscription()
		s.telemetryReporter.SetConfigSubscriptionsActive(len(s.mu.subscriptions.subs))
		return subID, updateSignal, nil
	}()
	if err != nil {
		log.Warnf("failed to create subscription: %v", err)
		return err
	}
	defer func() {
		s.telemetryReporter.IncConfigSubscriptionsDisconnectedCounter()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.mu.subscriptions.remove(subID)
		s.telemetryReporter.SetConfigSubscriptionsActive(len(s.mu.subscriptions.subs))
	}()
	s.telemetryReporter.IncConfigSubscriptionsConnectedCounter()
	// Send a header to synchronize with the client and prevent client-side
	// retries.
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	var wg sync.WaitGroup
	defer wg.Wait()
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		popUpdate := func() *pbgo.ConfigSubscriptionResponse {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.mu.subscriptions.popUpdate(subID)
		}
		for {
			if ctx.Err() != nil {
				return
			}
			if update := popUpdate(); update != nil {
				if err := stream.Send(update); err != nil {
					log.Debugf("subscription %d: failed to send update: %v", subID, err)
					return
				}
				// Go back around to see if there are any more updates to send.
				continue
			}
			select {
			case <-updateSignal:
			case <-ctx.Done():
			}
		}
	}()

	// Process incoming TRACK/UNTRACK requests from the client (receiver
	// goroutine). This is the current goroutine (gRPC handler).
	setTrackedClientsLocked := func() {
		n := 0
		for _, s := range s.mu.subscriptions.subs {
			n += len(s.trackedClients)
		}
		s.telemetryReporter.SetConfigSubscriptionClientsTracked(n)
	}
	track := func(runtimeID string, products pbgo.ConfigSubscriptionProducts) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		sub := s.mu.subscriptions.subs[subID]
		if len(sub.trackedClients) >= s.maxTrackedRuntimeIDsPerSubscription {
			return status.Errorf(
				codes.ResourceExhausted,
				"maximum number of runtime IDs per subscription reached (%d)",
				s.maxTrackedRuntimeIDsPerSubscription,
			)
		}
		sub.track(runtimeID, products)
		setTrackedClientsLocked()
		return nil
	}
	untrack := func(runtimeID string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.mu.subscriptions.subs[subID].untrack(runtimeID)
		setTrackedClientsLocked()
	}
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			log.Debugf("subscription %d: stream closed or errored: %v", subID, err)
			return err
		}

		// Validate the request.
		if err := validateSubscriptionRequest(req, s.subscriptionProductMappings); err != nil {
			log.Warnf("subscription %d: received invalid request: %v", subID, err)
			return err
		}

		switch req.Action {
		case pbgo.ConfigSubscriptionRequest_TRACK:
			if err := track(req.RuntimeId, req.Products); err != nil {
				log.Warnf("subscription %d: failed to track runtime ID %v: %v", subID, req.RuntimeId, err)
				return err
			}
		case pbgo.ConfigSubscriptionRequest_UNTRACK:
			untrack(req.RuntimeId)
		}
	}
}

func validateSubscriptionRequest(
	req *pbgo.ConfigSubscriptionRequest, pms productsMappings,
) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	if req.RuntimeId == "" {
		return status.Error(codes.InvalidArgument, "runtime_id is required")
	}
	switch req.Action {
	case pbgo.ConfigSubscriptionRequest_TRACK:
		_, ok := pms[req.Products]
		if !ok {
			return status.Errorf(codes.InvalidArgument, "invalid products %v", req.Products)
		}
	case pbgo.ConfigSubscriptionRequest_UNTRACK:
		// All good.
	default:
		return status.Error(codes.InvalidArgument, "action must be TRACK or UNTRACK")
	}

	return nil
}

func validateRequest(request *pbgo.ClientGetConfigsRequest) error {
	if request.Client == nil {
		return status.Error(codes.InvalidArgument, "client is a required field for client config update requests")
	}

	if request.Client.State == nil {
		return status.Error(codes.InvalidArgument, "client.state is a required field for client config update requests")
	}

	if request.Client.State.RootVersion <= 0 {
		return status.Error(codes.InvalidArgument, "client.state.root_version must be >= 1 (clients must start with the base TUF director root)")
	}

	if request.Client.IsAgent && request.Client.ClientAgent == nil {
		return status.Error(codes.InvalidArgument, "client.client_agent is a required field for agent client config update requests")
	}

	if request.Client.IsTracer && request.Client.ClientTracer == nil {
		return status.Error(codes.InvalidArgument, "client.client_tracer is a required field for tracer client config update requests")
	}

	if request.Client.IsUpdater && request.Client.ClientUpdater == nil {
		return status.Error(codes.InvalidArgument, "client.client_updater is a required field for updater client config update requests")
	}

	if (request.Client.IsTracer && request.Client.IsAgent) || (request.Client.IsTracer && request.Client.IsUpdater) || (request.Client.IsAgent && request.Client.IsUpdater) {
		return status.Error(codes.InvalidArgument, "client.is_tracer, client.is_agent, and client.is_updater are mutually exclusive")
	}

	if !request.Client.IsTracer && !request.Client.IsAgent && !request.Client.IsUpdater {
		return status.Error(codes.InvalidArgument, "agents only support remote config updates from tracer or agent or updater at this time")
	}

	if request.Client.Id == "" {
		return status.Error(codes.InvalidArgument, "client.id is a required field for client config update requests")
	}

	// Validate tracer-specific fields
	if request.Client.IsTracer {
		if request.Client.ClientTracer == nil {
			return status.Error(codes.InvalidArgument, "client.client_tracer must be set if client.is_tracer is true")
		}
		if request.Client.ClientAgent != nil {
			return status.Error(codes.InvalidArgument, "client.client_agent must not be set if client.is_tracer is true")
		}

		clientTracer := request.Client.ClientTracer

		if request.Client.Id == clientTracer.RuntimeId {
			return status.Error(codes.InvalidArgument, "client.id must be different from client.client_tracer.runtime_id")
		}

		if request.Client.ClientTracer.Language == "" {
			return status.Error(codes.InvalidArgument, "client.client_tracer.language is a required field for tracer client config update requests")
		}

	}

	// Validate agent-specific fields
	if request.Client.IsAgent {
		if request.Client.ClientAgent == nil {
			return status.Error(codes.InvalidArgument, "client.client_agent must be set if client.is_agent is true")
		}
		if request.Client.ClientTracer != nil {
			return status.Error(codes.InvalidArgument, "client.client_tracer must not be set if client.is_agent is true")
		}
	}

	// Validate cached target files fields
	for targetFileIndex, targetFile := range request.CachedTargetFiles {
		if targetFile.Path == "" {
			return status.Errorf(codes.InvalidArgument, "cached_target_files[%d].path is a required field for client config update requests", targetFileIndex)
		}
		_, err := rdata.ParseConfigPath(targetFile.Path)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "cached_target_files[%d].path is not a valid path: %s", targetFileIndex, err.Error())
		}
		if targetFile.Length == 0 {
			return status.Errorf(codes.InvalidArgument, "cached_target_files[%d].length must be >= 1 (no empty file allowed)", targetFileIndex)
		}
		if len(targetFile.Hashes) == 0 {
			return status.Error(codes.InvalidArgument, "cached_target_files[%d].hashes is a required field for client config update requests")
		}
		for hashIndex, hash := range targetFile.Hashes {
			if hash.Algorithm == "" {
				return status.Errorf(codes.InvalidArgument, "cached_target_files[%d].hashes[%d].algorithm is a required field for client config update requests", targetFileIndex, hashIndex)
			}
			if len(hash.Hash) == 0 {
				return status.Errorf(codes.InvalidArgument, "cached_target_files[%d].hashes[%d].hash is a required field for client config update requests", targetFileIndex, hashIndex)
			}
		}
	}

	return nil
}
