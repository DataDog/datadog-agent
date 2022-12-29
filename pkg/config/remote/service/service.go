// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/DataDog/go-tuf/data"
	tufutil "github.com/DataDog/go-tuf/util"
	"github.com/benbjohnson/clock"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	"github.com/DataDog/datadog-agent/pkg/config/remote/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultRefreshInterval = 1 * time.Minute
	minimalRefreshInterval = 5 * time.Second
	defaultClientsTTL      = 30 * time.Second
	maxClientsTTL          = 60 * time.Second
	newClientBlockTTL      = 2 * time.Second
)

// Constraints on the maximum backoff time when errors occur
const (
	minimalMaxBackoffTime = 2 * time.Minute
	maximalMaxBackoffTime = 5 * time.Minute
)

// Service defines the remote config management service responsible for fetching, storing
// and dispatching the configurations
type Service struct {
	sync.Mutex
	firstUpdate bool

	defaultRefreshInterval         time.Duration
	refreshIntervalOverrideAllowed bool

	// The backoff policy used for retries when errors are encountered
	backoffPolicy backoff.Policy
	// The number of errors we're currently tracking within the context of our backoff policy
	backoffErrorCount int

	clock         clock.Clock
	hostname      string
	traceAgentEnv string
	db            *bbolt.DB
	uptane        uptaneClient
	api           api.API

	products         map[rdata.Product]struct{}
	newProducts      map[rdata.Product]struct{}
	clients          *clients
	newActiveClients newActiveClients

	lastUpdateErr error
}

// uptaneClient is used to mock the uptane component for testing
type uptaneClient interface {
	Update(response *pbgo.LatestConfigsResponse) error
	State() (uptane.State, error)
	DirectorRoot(version uint64) ([]byte, error)
	Targets() (data.TargetFiles, error)
	TargetFile(path string) ([]byte, error)
	TargetsMeta() ([]byte, error)
	TargetsCustom() ([]byte, error)
	TUFVersionState() (uptane.TUFVersions, error)
}

// NewService instantiates a new remote configuration management service
func NewService() (*Service, error) {
	refreshIntervalOverrideAllowed := false // If a user provides a value we don't want to override
	refreshInterval := config.Datadog.GetDuration("remote_configuration.refresh_interval")
	if refreshInterval == 0 {
		refreshInterval = defaultRefreshInterval
		refreshIntervalOverrideAllowed = true
	}

	if refreshInterval < minimalRefreshInterval {
		log.Warnf("remote_configuration.refresh_interval is set to %v which is below the minimum of %v", refreshInterval, minimalRefreshInterval)
		refreshInterval = minimalRefreshInterval
		refreshIntervalOverrideAllowed = true
	}

	maxBackoffTime := config.Datadog.GetDuration("remote_configuration.max_backoff_interval")
	if maxBackoffTime < minimalMaxBackoffTime {
		log.Warnf("remote_configuration.max_backoff_time is set to %v which is below the minimum of %v - setting value to %v", maxBackoffTime, minimalMaxBackoffTime, minimalMaxBackoffTime)
		maxBackoffTime = minimalMaxBackoffTime
	} else if maxBackoffTime > maximalMaxBackoffTime {
		log.Warnf("remote_configuration.max_backoff_time is set to %v which is above the maximum of %v - setting value to %v", maxBackoffTime, maximalMaxBackoffTime, maximalMaxBackoffTime)
		maxBackoffTime = maximalMaxBackoffTime
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

	backoffPolicy := backoff.NewPolicy(minBackoffFactor, baseBackoffTime,
		maxBackoffTime.Seconds(), recoveryInterval, recoveryReset)

	apiKey := config.Datadog.GetString("api_key")
	if config.Datadog.IsSet("remote_configuration.api_key") {
		apiKey = config.Datadog.GetString("remote_configuration.api_key")
	}
	apiKey = config.SanitizeAPIKey(apiKey)
	rcKey := config.Datadog.GetString("remote_configuration.key")
	authKeys, err := getRemoteConfigAuthKeys(apiKey, rcKey)
	if err != nil {
		return nil, err
	}
	http, err := api.NewHTTPClient(authKeys.apiAuth())
	if err != nil {
		return nil, err
	}
	hname, err := hostname.Get(context.Background())
	if err != nil {
		return nil, err
	}
	dbPath := path.Join(config.Datadog.GetString("run_path"), "remote-config.db")
	db, err := openCacheDB(dbPath)
	if err != nil {
		return nil, err
	}
	cacheKey := generateCacheKey(apiKey)
	opt := []uptane.ClientOption{}
	if authKeys.rcKeySet {
		opt = append(opt, uptane.WithOrgIDCheck(authKeys.rcKey.OrgID))
	}
	uptaneClient, err := uptane.NewClient(
		db,
		cacheKey,
		newRCBackendOrgUUIDProvider(http),
		opt...,
	)
	if err != nil {
		return nil, err
	}

	clientsTTL := config.Datadog.GetDuration("remote_configuration.clients.ttl_seconds")
	if clientsTTL < minimalRefreshInterval || clientsTTL > maxClientsTTL {
		log.Warnf("Configured clients ttl is not within accepted range (%s - %s): %s. Defaulting to %s", minimalRefreshInterval, maxClientsTTL, clientsTTL, defaultClientsTTL)
		clientsTTL = defaultClientsTTL
	}
	clock := clock.New()

	return &Service{
		firstUpdate:                    true,
		defaultRefreshInterval:         refreshInterval,
		refreshIntervalOverrideAllowed: refreshIntervalOverrideAllowed,
		backoffErrorCount:              0,
		backoffPolicy:                  backoffPolicy,
		products:                       make(map[rdata.Product]struct{}),
		newProducts:                    make(map[rdata.Product]struct{}),
		hostname:                       hname,
		traceAgentEnv:                  config.GetTraceAgentDefaultEnv(),
		clock:                          clock,
		db:                             db,
		api:                            http,
		uptane:                         uptaneClient,
		clients:                        newClients(clock, clientsTTL),
		newActiveClients: newActiveClients{
			clock:    clock,
			requests: make(chan chan struct{}),
			until:    time.Now().UTC(),
		},
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
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		err := s.refresh()
		if err != nil {
			log.Errorf("could not refresh remote-config: %v", err)
		}

		for {
			var err error
			refreshInterval := s.calculateRefreshInterval()
			select {
			case <-s.clock.After(refreshInterval):
				err = s.refresh()
			// New clients detected, request refresh
			case response := <-s.newActiveClients.requests:
				if time.Now().UTC().After(s.newActiveClients.until) {
					s.newActiveClients.setRateLimit(refreshInterval)
					err = s.refresh()
				} else {
					telemetry.CacheBypassRateLimit.Inc()
				}
				close(response)
			case <-ctx.Done():
				return
			}

			if err != nil {
				log.Errorf("could not refresh remote-config: %v", err)
			}
		}
	}()
	return nil
}

func (s *Service) calculateRefreshInterval() time.Duration {
	backoffTime := s.backoffPolicy.GetBackoffDuration(s.backoffErrorCount)

	return s.defaultRefreshInterval + backoffTime
}

func (s *Service) refresh() error {
	s.Lock()
	activeClients := s.clients.activeClients()
	s.refreshProducts(activeClients)
	previousState, err := s.uptane.TUFVersionState()
	if err != nil {
		log.Warnf("could not get previous TUF version state: %v", err)
	}
	if s.forceRefresh() || err != nil {
		previousState = uptane.TUFVersions{}
	}
	clientState, err := s.getClientState()
	if err != nil {
		log.Warnf("could not get previous backend client state: %v", err)
	}
	request := buildLatestConfigsRequest(s.hostname, s.traceAgentEnv, previousState, activeClients, s.products, s.newProducts, s.lastUpdateErr, clientState)
	s.Unlock()
	ctx := context.Background()
	response, err := s.api.Fetch(ctx, request)
	s.Lock()
	defer s.Unlock()
	s.lastUpdateErr = nil
	if err != nil {
		s.backoffErrorCount = s.backoffPolicy.IncError(s.backoffErrorCount)
		s.lastUpdateErr = fmt.Errorf("api: %v", err)
		return err
	}
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
			log.Info("Overriding agent's base refresh interval to %v due to backend recommendation", ri)
		}
	}

	s.firstUpdate = false
	for product := range s.newProducts {
		s.products[product] = struct{}{}
	}
	s.newProducts = make(map[rdata.Product]struct{})

	s.backoffErrorCount = s.backoffPolicy.DecError(s.backoffErrorCount)

	return nil
}

func (s *Service) forceRefresh() bool {
	return s.firstUpdate
}

func (s *Service) refreshProducts(activeClients []*pbgo.Client) {
	for _, client := range activeClients {
		for _, product := range client.Products {
			if _, hasProduct := s.products[rdata.Product(product)]; !hasProduct {
				s.newProducts[rdata.Product(product)] = struct{}{}
			}
		}
	}
}

func (s *Service) getClientState() ([]byte, error) {
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

func (s *Service) getRefreshInterval() (time.Duration, error) {
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
func (s *Service) ClientGetConfigs(ctx context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	s.Lock()
	defer s.Unlock()
	err := validateRequest(request)
	if err != nil {
		return nil, err
	}

	if !s.clients.active(request.Client) {
		s.clients.seen(request.Client)
		s.Unlock()
		response := make(chan struct{})
		s.newActiveClients.requests <- response
		select {
		case <-response:
		case <-time.After(newClientBlockTTL):
			telemetry.CacheBypassTimeout.Inc()
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
	roots, err := s.getNewDirectorRoots(request.Client.State.RootVersion, tufVersions.DirectorRoot)
	if err != nil {
		return nil, err
	}
	targetsRaw, err := s.uptane.TargetsMeta()
	if err != nil {
		return nil, err
	}
	targetFiles, err := s.getTargetFiles(rdata.StringListToProduct(request.Client.Products), request.CachedTargetFiles)
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
func (s *Service) ConfigGetState() (*pbgo.GetStateConfigResponse, error) {
	state, err := s.uptane.State()
	if err != nil {
		return nil, err
	}

	response := &pbgo.GetStateConfigResponse{
		ConfigState:     map[string]*pbgo.FileMetaState{},
		DirectorState:   map[string]*pbgo.FileMetaState{},
		TargetFilenames: map[string]string{},
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

func (s *Service) getNewDirectorRoots(currentVersion uint64, newVersion uint64) ([][]byte, error) {
	var roots [][]byte
	for i := currentVersion + 1; i <= newVersion; i++ {
		root, err := s.uptane.DirectorRoot(i)
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

func (s *Service) getTargetFiles(products []rdata.Product, cachedTargetFiles []*pbgo.TargetFileMeta) ([]*pbgo.File, error) {
	productSet := make(map[rdata.Product]struct{})
	for _, product := range products {
		productSet[product] = struct{}{}
	}
	targets, err := s.uptane.Targets()
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
			fileContents, err := s.uptane.TargetFile(targetPath)
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

	if request.Client.IsTracer && request.Client.IsAgent {
		return status.Error(codes.InvalidArgument, "client.is_tracer and client.is_agent cannot both be true")
	}

	if !request.Client.IsTracer && !request.Client.IsAgent {
		return status.Error(codes.InvalidArgument, "agents only support remote config updates from tracer or agent at this time")
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

func generateCacheKey(apiKey string) string {
	h := sha256.New()
	h.Write([]byte(apiKey))

	// Hash the API Key with the initial root. This prevents the agent from being locked
	// to a root chain if a developer accidentally forgets to use the development roots
	// in a testing environment
	embeddedRoots := meta.RootsConfig()
	if r, ok := embeddedRoots[1]; ok {
		h.Write(r)
	}

	hash := h.Sum(nil)

	return fmt.Sprintf("%x/", hash)
}
