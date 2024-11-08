// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package service represents the agent's core remoteconfig service
//
// The `Service` type provides a communication layer for downstream clients to request
// configuration, as well as the ability to track clients for requesting complete update
// payloads from the remote config backend.
package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/go-tuf/data"
	tufutil "github.com/DataDog/go-tuf/util"
	"github.com/benbjohnson/clock"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultRefreshInterval  = 1 * time.Minute
	minimalRefreshInterval  = 5 * time.Second
	defaultClientsTTL       = 30 * time.Second
	maxClientsTTL           = 60 * time.Second
	newClientBlockTTL       = 2 * time.Second
	defaultCacheBypassLimit = 5
	minCacheBypassLimit     = 1
	maxCacheBypassLimit     = 10
	orgStatusPollInterval   = 1 * time.Minute
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

const (
	// the minimum amount of time that must pass before a new cache
	// bypass request is allowed for the CDN client
	maxCDNUpdateFrequency = 50 * time.Second
)

var (
	exportedMapStatus = expvar.NewMap("remoteConfigStatus")
	// Status expvar exported
	exportedStatusOrgEnabled    = expvar.String{}
	exportedStatusKeyAuthorized = expvar.String{}
	exportedLastUpdateErr       = expvar.String{}
)

// Service defines the remote config management service responsible for fetching, storing
// and dispatching the configurations
type Service struct {
	sync.Mutex

	// rcType is used to differentiate multiple RC services running in a single agent.
	// Today, it is simply logged as a prefix in all log messages to help when triaging
	// via logs.
	rcType string

	db *bbolt.DB
}

func (s *Service) getNewDirectorRoots(uptane uptaneClient, currentVersion uint64, newVersion uint64) ([][]byte, error) {
	var roots [][]byte
	for i := currentVersion + 1; i <= newVersion; i++ {
		root, err := uptane.DirectorRoot(i)
		if err != nil {
			return nil, err
		}
		canonicalRoot, err := enforceCanonicalJSON(root)
		if err != nil {
			return nil, err
		}
		roots = append(roots, canonicalRoot)
	}
	return roots, nil
}

func (s *Service) getTargetFiles(uptane uptaneClient, products []rdata.Product, cachedTargetFiles []*pbgo.TargetFileMeta) ([]*pbgo.File, error) {
	productSet := make(map[rdata.Product]struct{})
	for _, product := range products {
		productSet[product] = struct{}{}
	}
	targets, err := uptane.Targets()
	if err != nil {
		return nil, err
	}
	cachedTargets := make(map[string]data.FileMeta)
	for _, cachedTarget := range cachedTargetFiles {
		hashes := make(data.Hashes)
		for _, hash := range cachedTarget.Hashes {
			h, err := hex.DecodeString(hash.Hash)
			if err != nil {
				return nil, err
			}
			hashes[hash.Algorithm] = h
		}
		cachedTargets[cachedTarget.Path] = data.FileMeta{
			Hashes: hashes,
			Length: cachedTarget.Length,
		}
	}
	var configFiles []*pbgo.File
	for targetPath, targetMeta := range targets {
		configPathMeta, err := rdata.ParseConfigPath(targetPath)
		if err != nil {
			return nil, err
		}
		if _, inClientProducts := productSet[rdata.Product(configPathMeta.Product)]; inClientProducts {
			if notEqualErr := tufutil.FileMetaEqual(cachedTargets[targetPath], targetMeta.FileMeta); notEqualErr == nil {
				continue
			}
			fileContents, err := uptane.TargetFile(targetPath)
			if err != nil {
				return nil, err
			}
			configFiles = append(configFiles, &pbgo.File{
				Path: targetPath,
				Raw:  fileContents,
			})
		}
	}
	return configFiles, nil
}

// CoreAgentService fetches  Remote Configurations from the RC backend
type CoreAgentService struct {
	Service
	firstUpdate bool

	defaultRefreshInterval         time.Duration
	refreshIntervalOverrideAllowed bool

	// The backoff policy used for retries when errors are encountered
	backoffPolicy backoff.Policy
	// The number of errors we're currently tracking within the context of our backoff policy
	backoffErrorCount int

	// Channels to stop the services main goroutines
	stopOrgPoller    chan struct{}
	stopConfigPoller chan struct{}

	clock         clock.Clock
	hostname      string
	tagsGetter    func() []string
	traceAgentEnv string
	uptane        coreAgentUptaneClient
	api           api.API

	products           map[rdata.Product]struct{}
	newProducts        map[rdata.Product]struct{}
	clients            *clients
	cacheBypassClients cacheBypassClients

	// Used to report metrics on cache bypass requests
	telemetryReporter RcTelemetryReporter

	lastUpdateErr error

	// Used to rate limit the 4XX error logs
	fetchErrorCount    uint64
	lastFetchErrorType error

	// Previous /status response
	previousOrgStatus *pbgo.OrgStatusResponse

	agentVersion string
}

// uptaneClient provides functions to get TUF/uptane repo data.
type uptaneClient interface {
	State() (uptane.State, error)
	DirectorRoot(version uint64) ([]byte, error)
	StoredOrgUUID() (string, error)
	Targets() (data.TargetFiles, error)
	TargetFile(path string) ([]byte, error)
	TargetsMeta() ([]byte, error)
	TargetsCustom() ([]byte, error)
	TUFVersionState() (uptane.TUFVersions, error)
}

// coreAgentUptaneClient provides functions to get TUF/uptane repo data and update the agent's state via the RC backend.
type coreAgentUptaneClient interface {
	uptaneClient
	Update(response *pbgo.LatestConfigsResponse) error
}

// cdnUptaneClient provides functions to get TUF/uptane repo data and update the agent's state via the CDN.
type cdnUptaneClient interface {
	uptaneClient
	Update(ctx context.Context) error
}

// RcTelemetryReporter should be implemented by the agent to publish metrics on exceptional cache bypass request events
type RcTelemetryReporter interface {
	// IncRateLimit is invoked when a cache bypass request is prevented due to rate limiting
	IncRateLimit()
	// IncTimeout is invoked when a cache bypass request is cancelled due to timeout or a previous cache bypass request is still pending
	IncTimeout()
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
}

var defaultOptions = options{
	rcKey:                          "",
	apiKey:                         "",
	traceAgentEnv:                  "",
	databaseFileName:               "remote-config.db",
	databaseFilePath:               "",
	configRootOverride:             "",
	directorRootOverride:           "",
	clientCacheBypassLimit:         defaultCacheBypassLimit,
	refresh:                        defaultRefreshInterval,
	refreshIntervalOverrideAllowed: true,
	maxBackoff:                     minimalMaxBackoffTime,
	clientTTL:                      defaultClientsTTL,
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

	authKeys, err := getRemoteConfigAuthKeys(options.apiKey, options.rcKey)
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
	dbPath := path.Join(databaseFilePath, options.databaseFileName)
	db, err := openCacheDB(dbPath, agentVersion, authKeys.apiKey)
	if err != nil {
		return nil, err
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
	uptaneClient, err := uptane.NewCoreAgentClient(
		db,
		newRCBackendOrgUUIDProvider(http),
		opt...,
	)
	if err != nil {
		db.Close()
		return nil, err
	}

	clock := clock.New()

	return &CoreAgentService{
		Service: Service{
			rcType: rcType,
			db:     db,
		},
		firstUpdate:                    true,
		defaultRefreshInterval:         options.refresh,
		refreshIntervalOverrideAllowed: options.refreshIntervalOverrideAllowed,
		backoffErrorCount:              0,
		backoffPolicy:                  backoffPolicy,
		products:                       make(map[rdata.Product]struct{}),
		newProducts:                    make(map[rdata.Product]struct{}),
		hostname:                       hostname,
		tagsGetter:                     tagsGetter,
		clock:                          clock,
		traceAgentEnv:                  options.traceAgentEnv,
		api:                            http,
		uptane:                         uptaneClient,
		clients:                        newClients(clock, options.clientTTL),
		cacheBypassClients: cacheBypassClients{
			clock:    clock,
			requests: make(chan chan struct{}),

			// By default, allows for 5 cache bypass every refreshInterval seconds
			// in addition to the usual refresh.
			currentWindow:  time.Now().UTC(),
			windowDuration: options.refresh,
			capacity:       options.clientCacheBypassLimit,
			allowance:      options.clientCacheBypassLimit,
		},
		telemetryReporter: telemetryReporter,
		agentVersion:      agentVersion,
		stopOrgPoller:     make(chan struct{}),
		stopConfigPoller:  make(chan struct{}),
	}, nil
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
	go func() {
		s.pollOrgStatus()
		for {
			select {
			case <-s.clock.After(orgStatusPollInterval):
				s.pollOrgStatus()
			case <-s.stopOrgPoller:
				log.Infof("[%s] Stopping Remote Config org status poller", s.rcType)
				return
			}
		}
	}()
	go func() {
		defer func() {
			close(s.stopOrgPoller)
		}()

		err := s.refresh()
		if err != nil {
			if s.previousOrgStatus != nil && s.previousOrgStatus.Enabled && s.previousOrgStatus.Authorized {
				log.Errorf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
			} else {
				log.Debugf("[%s] Could not refresh Remote Config (org is disabled or key is not authorized): %v", s.rcType, err)
			}
		}

		for {
			var err error
			refreshInterval := s.calculateRefreshInterval()
			select {
			case <-s.clock.After(refreshInterval):
				err = s.refresh()
			// New clients detected, request refresh
			case response := <-s.cacheBypassClients.requests:
				if !s.cacheBypassClients.Limit() {
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
				if s.previousOrgStatus != nil && s.previousOrgStatus.Enabled && s.previousOrgStatus.Authorized {
					exportedLastUpdateErr.Set(err.Error())
					log.Errorf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
				} else {
					log.Debugf("[%s] Could not refresh Remote Config (org is disabled or key is not authorized): %v", s.rcType, err)
				}
			}
		}
	}()
}

// Stop stops the refresh loop and closes the on-disk DB cache
func (s *CoreAgentService) Stop() error {
	if s.stopConfigPoller != nil {
		close(s.stopConfigPoller)
	}

	return s.db.Close()
}

func (s *CoreAgentService) pollOrgStatus() {
	response, err := s.api.FetchOrgStatus(context.Background())
	if err != nil {
		// Unauthorized and proxy error are caught by the main loop requesting the latest config,
		// and it limits the error log.
		if !errors.Is(err, api.ErrUnauthorized) && !errors.Is(err, api.ErrProxy) {
			log.Errorf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
		}
		return
	}

	// Print info log when the new status is different from the previous one, or if it's the first run
	if s.previousOrgStatus == nil ||
		s.previousOrgStatus.Enabled != response.Enabled ||
		s.previousOrgStatus.Authorized != response.Authorized {
		if response.Enabled {
			if response.Authorized {
				log.Infof("[%s] Remote Configuration is enabled for this organization and agent.", s.rcType)
			} else {
				log.Infof(
					"[%s] Remote Configuration is enabled for this organization but disabled for this agent. Add the Remote Configuration Read permission to its API key to enable it for this agent.", s.rcType)
			}
		} else {
			if response.Authorized {
				log.Infof("[%s] Remote Configuration is disabled for this organization.", s.rcType)
			} else {
				log.Infof("[%s] Remote Configuration is disabled for this organization and agent.", s.rcType)
			}
		}
	}
	s.previousOrgStatus = &pbgo.OrgStatusResponse{
		Enabled:    response.Enabled,
		Authorized: response.Authorized,
	}
	exportedStatusOrgEnabled.Set(strconv.FormatBool(response.Enabled))
	exportedStatusKeyAuthorized.Set(strconv.FormatBool(response.Authorized))
}

func (s *CoreAgentService) calculateRefreshInterval() time.Duration {
	backoffTime := s.backoffPolicy.GetBackoffDuration(s.backoffErrorCount)

	return s.defaultRefreshInterval + backoffTime
}

func (s *CoreAgentService) refresh() error {
	s.Lock()
	activeClients := s.clients.activeClients()
	s.refreshProducts(activeClients)
	previousState, err := s.uptane.TUFVersionState()
	if err != nil {
		log.Warnf("[%s] could not get previous TUF version state: %v", s.rcType, err)
	}
	if s.forceRefresh() || err != nil {
		previousState = uptane.TUFVersions{}
	}
	clientState, err := s.getClientState()
	if err != nil {
		log.Warnf("[%s] could not get previous backend client state: %v", s.rcType, err)
	}
	orgUUID, err := s.uptane.StoredOrgUUID()
	if err != nil {
		s.Unlock()
		return err
	}

	request := buildLatestConfigsRequest(s.hostname, s.agentVersion, s.tagsGetter(), s.traceAgentEnv, orgUUID, previousState, activeClients, s.products, s.newProducts, s.lastUpdateErr, clientState)
	s.Unlock()
	ctx := context.Background()
	response, err := s.api.Fetch(ctx, request)
	s.Lock()
	defer s.Unlock()
	s.lastUpdateErr = nil
	if err != nil {
		s.backoffErrorCount = s.backoffPolicy.IncError(s.backoffErrorCount)
		s.lastUpdateErr = fmt.Errorf("api: %v", err)
		if s.lastFetchErrorType != err {
			s.lastFetchErrorType = err
			s.fetchErrorCount = 0
		}

		if errors.Is(err, api.ErrUnauthorized) || errors.Is(err, api.ErrProxy) {
			if s.fetchErrorCount < initialFetchErrorLog {
				s.fetchErrorCount++
				return err
			}
			// If we saw the error enough time, we consider that RC not working is a normal behavior
			// And we only log as DEBUG
			// The agent will eventually log this error as DEBUG every maximalMaxBackoffTime
			log.Debugf("[%s] Could not refresh Remote Config: %v", s.rcType, err)
			return nil
		}
		return err
	}
	s.fetchErrorCount = 0
	err = s.uptane.Update(response)
	if err != nil {
		s.backoffErrorCount = s.backoffPolicy.IncError(s.backoffErrorCount)
		s.lastUpdateErr = fmt.Errorf("tuf: %v", err)
		return err
	}

	// If a user hasn't explicitly set the refresh interval, allow the backend to override it based
	// on the contents of our update request
	if s.refreshIntervalOverrideAllowed {
		ri, err := s.getRefreshInterval()
		if err == nil && ri > 0 && s.defaultRefreshInterval != ri {
			s.defaultRefreshInterval = ri
			s.cacheBypassClients.windowDuration = ri
			log.Infof("[%s] Overriding agent's base refresh interval to %v due to backend recommendation", s.rcType, ri)
		}
	}

	s.firstUpdate = false
	for product := range s.newProducts {
		s.products[product] = struct{}{}
	}
	s.newProducts = make(map[rdata.Product]struct{})

	s.backoffErrorCount = s.backoffPolicy.DecError(s.backoffErrorCount)

	exportedLastUpdateErr.Set("")

	return nil
}

func (s *CoreAgentService) forceRefresh() bool {
	return s.firstUpdate
}

func (s *CoreAgentService) refreshProducts(activeClients []*pbgo.Client) {
	for _, client := range activeClients {
		for _, product := range client.Products {
			if _, hasProduct := s.products[rdata.Product(product)]; !hasProduct {
				s.newProducts[rdata.Product(product)] = struct{}{}
			}
		}
	}
}

func (s *CoreAgentService) getClientState() ([]byte, error) {
	rawTargetsCustom, err := s.uptane.TargetsCustom()
	if err != nil {
		return nil, err
	}
	custom, err := parseTargetsCustom(rawTargetsCustom)
	if err != nil {
		return nil, err
	}
	return custom.OpaqueBackendState, nil
}

func (s *CoreAgentService) getRefreshInterval() (time.Duration, error) {
	rawTargetsCustom, err := s.uptane.TargetsCustom()
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

// ClientGetConfigs is the polling API called by tracers and agents to get the latest configurations
//
//nolint:revive // TODO(RC) Fix revive linter
func (s *CoreAgentService) ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	s.Lock()
	defer s.Unlock()
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
		s.Unlock()

		response := make(chan struct{})
		bypassStart := time.Now()

		// Timeout in case the previous request is still pending
		// and we can't request another one
		select {
		case s.cacheBypassClients.requests <- response:
		case <-time.After(newClientBlockTTL):
			// No need to add telemetry here, it'll be done in the second
			// timeout case that will automatically be triggered
		}

		partialNewClientBlockTTL := newClientBlockTTL - time.Since(bypassStart)

		// Timeout if the response is taking too long
		select {
		case <-response:
		case <-time.After(partialNewClientBlockTTL):
			s.telemetryReporter.IncTimeout()
		}

		s.Lock()
	}

	s.clients.seen(request.Client)
	tufVersions, err := s.uptane.TUFVersionState()
	if err != nil {
		return nil, err
	}
	if tufVersions.DirectorTargets == request.Client.State.TargetsVersion {
		return &pbgo.ClientGetConfigsResponse{}, nil
	}
	roots, err := s.getNewDirectorRoots(s.uptane, request.Client.State.RootVersion, tufVersions.DirectorRoot)
	if err != nil {
		return nil, err
	}
	targetsRaw, err := s.uptane.TargetsMeta()
	if err != nil {
		return nil, err
	}
	targetFiles, err := s.getTargetFiles(s.uptane, rdata.StringListToProduct(request.Client.Products), request.CachedTargetFiles)
	if err != nil {
		return nil, err
	}

	directorTargets, err := s.uptane.Targets()
	if err != nil {
		return nil, err
	}
	matchedClientConfigs, err := executeTracerPredicates(request.Client, directorTargets)
	if err != nil {
		return nil, err
	}

	// filter files to only return the ones that predicates marked for this client
	matchedConfigsMap := make(map[string]interface{})
	for _, configPointer := range matchedClientConfigs {
		matchedConfigsMap[configPointer] = struct{}{}
	}
	filteredFiles := make([]*pbgo.File, 0, len(matchedClientConfigs))
	for _, targetFile := range targetFiles {
		if _, ok := matchedConfigsMap[targetFile.Path]; ok {
			filteredFiles = append(filteredFiles, targetFile)
		}
	}

	canonicalTargets, err := enforceCanonicalJSON(targetsRaw)
	if err != nil {
		return nil, err
	}

	return &pbgo.ClientGetConfigsResponse{
		Roots:         roots,
		Targets:       canonicalTargets,
		TargetFiles:   filteredFiles,
		ClientConfigs: matchedClientConfigs,
	}, nil
}

// ConfigGetState returns the state of the configuration and the director repos in the local store
func (s *CoreAgentService) ConfigGetState() (*pbgo.GetStateConfigResponse, error) {
	state, err := s.uptane.State()
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

	for targetName, targetHash := range state.TargetFilenames {
		response.TargetFilenames[targetName] = targetHash
	}

	return response, nil
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

func enforceCanonicalJSON(raw []byte) ([]byte, error) {
	canonical, err := cjson.EncodeCanonical(json.RawMessage(raw))
	if err != nil {
		return nil, err
	}

	return canonical, nil
}

// HTTPClient fetches Remote Configurations from an HTTP(s)-based backend
type HTTPClient struct {
	Service
	lastUpdate time.Time
	uptane     cdnUptaneClient
}

// NewHTTPClient creates a new HTTPClient that can be used to fetch Remote Configurations from an HTTP(s)-based backend
// It uses a local db to cache the fetched configurations. Only one HTTPClient should be created per agent.
// An HTTPClient must be closed via HTTPClient.Close() before creating a new one.
func NewHTTPClient(runPath, site, apiKey, agentVersion string) (*HTTPClient, error) {
	dbPath := path.Join(runPath, "remote-config-cdn.db")
	db, err := openCacheDB(dbPath, agentVersion, apiKey)
	if err != nil {
		return nil, err
	}

	uptaneCDNClient, err := uptane.NewCDNClient(db, site, apiKey)
	if err != nil {
		return nil, err
	}

	return &HTTPClient{
		Service: Service{
			rcType: "CDN",
			db:     db,
		},
		uptane: uptaneCDNClient,
	}, nil
}

// Close closes the HTTPClient and cleans up any resources. Close must be called
// before any other HTTPClients are instantiated via NewHTTPClient
func (c *HTTPClient) Close() error {
	return c.db.Close()
}

// GetCDNConfigUpdate returns any updated configs. If multiple requests have been made
// in a short amount of time, a cached response is returned. If RC has been disabled,
// an error is returned. If there is no update (the targets version is up-to-date) nil
// is returned for both the update and error.
func (c *HTTPClient) GetCDNConfigUpdate(
	ctx context.Context,
	products []string,
	currentTargetsVersion, currentRootVersion uint64,
	cachedTargetFiles []*pbgo.TargetFileMeta,
) (*state.Update, error) {
	var err error
	span, ctx := tracer.StartSpanFromContext(ctx, "HTTPClient.GetCDNConfigUpdate")
	defer span.Finish(tracer.WithError(err))
	if !c.shouldUpdate() {
		span.SetTag("use_cache", true)
		return c.getUpdate(ctx, products, currentTargetsVersion, currentRootVersion, cachedTargetFiles)
	}

	err = c.update(ctx)
	if err != nil {
		span.SetTag("cache_update_error", true)
		_ = log.Warn(fmt.Sprintf("Error updating CDN config repo: %v", err))
	}

	u, err := c.getUpdate(ctx, products, currentTargetsVersion, currentRootVersion, cachedTargetFiles)
	return u, err
}

func (c *HTTPClient) update(ctx context.Context) error {
	var err error
	span, ctx := tracer.StartSpanFromContext(ctx, "HTTPClient.update")
	defer span.Finish(tracer.WithError(err))
	c.Lock()
	defer c.Unlock()

	err = c.uptane.Update(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *HTTPClient) shouldUpdate() bool {
	c.Lock()
	defer c.Unlock()
	if time.Since(c.lastUpdate) > maxCDNUpdateFrequency {
		c.lastUpdate = time.Now()
		return true
	}
	return false
}

func (c *HTTPClient) getUpdate(
	ctx context.Context,
	products []string,
	currentTargetsVersion, currentRootVersion uint64,
	cachedTargetFiles []*pbgo.TargetFileMeta,
) (*state.Update, error) {
	c.Lock()
	defer c.Unlock()
	span, _ := tracer.StartSpanFromContext(ctx, "HTTPClient.getUpdate")
	defer span.Finish()
	span.SetTag("products", products)
	span.SetTag("current_targets_version", currentTargetsVersion)
	span.SetTag("current_root_version", currentRootVersion)
	span.SetTag("cached_target_files", cachedTargetFiles)

	tufVersions, err := c.uptane.TUFVersionState()
	if err != nil {
		return nil, err
	}
	if tufVersions.DirectorTargets == currentTargetsVersion {
		return nil, nil
	}
	roots, err := c.getNewDirectorRoots(c.uptane, currentRootVersion, tufVersions.DirectorRoot)
	if err != nil {
		return nil, err
	}
	targetsRaw, err := c.uptane.TargetsMeta()
	if err != nil {
		return nil, err
	}
	targetFiles, err := c.getTargetFiles(c.uptane, rdata.StringListToProduct(products), cachedTargetFiles)
	if err != nil {
		return nil, err
	}

	canonicalTargets, err := enforceCanonicalJSON(targetsRaw)
	if err != nil {
		return nil, err
	}

	directorTargets, err := c.uptane.Targets()
	if err != nil {
		return nil, err
	}

	productsMap := make(map[string]struct{})
	for _, product := range products {
		productsMap[product] = struct{}{}
	}
	configs := make([]string, 0)
	expiredConfigs := make([]string, 0)
	for path, meta := range directorTargets {
		pathMeta, err := rdata.ParseConfigPath(path)
		if err != nil {
			return nil, err
		}
		if _, productRequested := productsMap[pathMeta.Product]; !productRequested {
			continue
		}
		configMetadata, err := parseFileMetaCustom(meta.Custom)
		if err != nil {
			return nil, err
		}
		if configExpired(configMetadata.Expires) {
			expiredConfigs = append(expiredConfigs, path)
			continue
		}

		configs = append(configs, path)
	}

	fileMap := make(map[string][]byte, len(targetFiles))
	for _, f := range targetFiles {
		fileMap[f.Path] = f.Raw
	}

	span.SetTag("configs.returned", configs)
	span.SetTag("configs.expired", expiredConfigs)
	return &state.Update{
		TUFRoots:      roots,
		TUFTargets:    canonicalTargets,
		TargetFiles:   fileMap,
		ClientConfigs: configs,
	}, nil
}
