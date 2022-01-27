// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/theupdateframework/go-tuf/data"
	tufutil "github.com/theupdateframework/go-tuf/util"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.etcd.io/bbolt"
)

const (
	minimalRefreshInterval = time.Second * 5
	defaultClientsTTL      = 10 * time.Second
)

// Service defines the remote config management service responsible for fetching, storing
// and dispatching the configurations
type Service struct {
	sync.Mutex
	firstUpdate bool

	refreshInterval time.Duration

	ctx      context.Context
	clock    clock.Clock
	hostname string
	db       *bbolt.DB
	uptane   uptaneClient
	api      api.API

	products    map[rdata.Product]struct{}
	newProducts map[rdata.Product]struct{}
	clients     *clients
}

// uptaneClient is used to mock the uptane component for testing
type uptaneClient interface {
	Update(response *pbgo.LatestConfigsResponse) error
	State() (uptane.State, error)
	DirectorRoot(version uint64) ([]byte, error)
	Targets() (data.TargetFiles, error)
	TargetFile(path string) ([]byte, error)
	TargetsMeta() ([]byte, error)
}

// NewService instantiates a new remote configuration management service
func NewService() (*Service, error) {
	refreshInterval := config.Datadog.GetDuration("remote_configuration.refresh_interval")
	if refreshInterval < minimalRefreshInterval {
		log.Warnf("remote_configuration.refresh_interval is set to %v which is bellow the minimum of %v", refreshInterval, minimalRefreshInterval)
		refreshInterval = minimalRefreshInterval
	}

	rawRemoteConfigKey := config.Datadog.GetString("remote_configuration.key")
	remoteConfigKey, err := parseRemoteConfigKey(rawRemoteConfigKey)
	if err != nil {
		return nil, err
	}

	apiKey := config.Datadog.GetString("api_key")
	if config.Datadog.IsSet("remote_configuration.api_key") {
		apiKey = config.Datadog.GetString("remote_configuration.api_key")
	}
	apiKey = config.SanitizeAPIKey(apiKey)
	hostname, err := util.GetHostname(context.Background())
	if err != nil {
		return nil, err
	}
	backendURL := config.Datadog.GetString("remote_configuration.endpoint")
	http := api.NewHTTPClient(backendURL, apiKey, remoteConfigKey.AppKey)

	dbPath := path.Join(config.Datadog.GetString("run_path"), "remote-config.db")
	db, err := openCacheDB(dbPath)
	if err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("%s/%d/", remoteConfigKey.Datacenter, remoteConfigKey.OrgID)
	uptaneClient, err := uptane.NewClient(db, cacheKey, remoteConfigKey.OrgID)
	if err != nil {
		return nil, err
	}

	clientsTTL := time.Second * config.Datadog.GetDuration("remote_configuration.clients.ttl_seconds")
	if clientsTTL <= 5*time.Second || clientsTTL >= 60*time.Second {
		log.Warnf("Configured clients ttl is not within accepted range (%ds - %ds): %s. Defaulting to %s", 5, 10, clientsTTL, defaultClientsTTL)
		clientsTTL = defaultClientsTTL
	}
	clock := clock.New()
	return &Service{
		ctx:             context.Background(),
		firstUpdate:     true,
		refreshInterval: refreshInterval,
		products:        make(map[rdata.Product]struct{}),
		newProducts:     make(map[rdata.Product]struct{}),
		hostname:        hostname,
		clock:           clock,
		db:              db,
		api:             http,
		uptane:          uptaneClient,
		clients:         newClients(clock, clientsTTL),
	}, nil
}

// Start the remote configuration management service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		for {
			select {
			case <-s.clock.After(s.refreshInterval):
				err := s.refresh()
				if err != nil {
					log.Errorf("could not refresh remote-config: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (s *Service) refresh() error {
	s.Lock()
	defer s.Unlock()
	activeClients := s.clients.activeClients()
	s.refreshProducts(activeClients)
	previousState, err := s.uptane.State()
	if err != nil {
		log.Warnf("could not get previous state: %v", err)
	}
	if s.forceRefresh() || err != nil {
		previousState = uptane.State{}
	}
	response, err := s.api.Fetch(s.ctx, buildLatestConfigsRequest(s.hostname, previousState, activeClients, s.products, s.newProducts))
	if err != nil {
		return err
	}
	err = s.uptane.Update(response)
	if err != nil {
		return err
	}
	s.firstUpdate = false
	for product := range s.newProducts {
		s.products[product] = struct{}{}
	}
	s.newProducts = make(map[rdata.Product]struct{})
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

// ClientGetConfigs is the polling API called by tracers and agents to get the latest configurations
func (s *Service) ClientGetConfigs(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	s.Lock()
	defer s.Unlock()
	s.clients.seen(request.Client)
	state, err := s.uptane.State()
	if err != nil {
		return nil, err
	}
	if state.DirectorTargetsVersion == request.Client.State.TargetsVersion {
		return &pbgo.ClientGetConfigsResponse{}, nil
	}
	roots, err := s.getNewDirectorRoots(request.Client.State.RootVersion, state.DirectorRootVersion)
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
	return &pbgo.ClientGetConfigsResponse{
		Roots: roots,
		Targets: &pbgo.TopMeta{
			Version: state.DirectorTargetsVersion,
			Raw:     targetsRaw,
		},
		TargetFiles: targetFiles,
	}, nil
}

func (s *Service) getNewDirectorRoots(currentVersion uint64, newVersion uint64) ([]*pbgo.TopMeta, error) {
	var roots []*pbgo.TopMeta
	for i := currentVersion + 1; i <= newVersion; i++ {
		root, err := s.uptane.DirectorRoot(i)
		if err != nil {
			return nil, err
		}
		roots = append(roots, &pbgo.TopMeta{
			Raw:     root,
			Version: i,
		})
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
			hashes[hash.Algorithm] = hash.Hash
		}
		cachedTargets[cachedTarget.Path] = data.FileMeta{
			Hashes: hashes,
			Length: cachedTarget.Length,
		}
	}
	var configFiles []*pbgo.File
	for targetPath, targetMeta := range targets {
		configFileMeta, err := rdata.ParseFilePathMeta(targetPath)
		if err != nil {
			return nil, err
		}
		if _, inClientProducts := productSet[configFileMeta.Product]; inClientProducts {
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
