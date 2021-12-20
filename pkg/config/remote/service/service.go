// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.etcd.io/bbolt"
)

const (
	minimalRefreshInterval = time.Second * 5
	defaultTracerCacheSize = 1000
	defaultTracerCacheTTL  = 10 * time.Second
)

// Service defines the remote config management service responsible for fetching, storing
// and dispatching the configurations
type Service struct {
	sync.Mutex
	firstUpdate bool

	refreshInterval time.Duration
	remoteConfigKey remoteConfigKey

	ctx    context.Context
	db     *bbolt.DB
	uptane *uptane.Client
	client *client.HTTPClient

	products    map[pbgo.Product]struct{}
	newProducts map[pbgo.Product]struct{}

	subscribers []*Subscriber
	TracerInfos *TracerCache
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
	client := client.NewHTTPClient(backendURL, apiKey, remoteConfigKey.appKey, hostname)

	dbPath := path.Join(config.Datadog.GetString("run_path"), "remote-config.db")
	db, err := openCacheDB(dbPath)
	if err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("%s/%d/", remoteConfigKey.datacenter, remoteConfigKey.orgID)
	uptaneClient, err := uptane.NewClient(db, cacheKey, remoteConfigKey.orgID)
	if err != nil {
		return nil, err
	}

	tracerCacheSize := config.Datadog.GetInt("remote_configuration.tracer_cache.size")
	if tracerCacheSize <= 0 {
		tracerCacheSize = defaultTracerCacheSize
	}

	tracerCacheTTL := time.Second * config.Datadog.GetDuration("remote_configuration.tracer_cache.ttl_seconds")
	if tracerCacheTTL <= 0 {
		tracerCacheTTL = defaultTracerCacheTTL
	}

	if tracerCacheTTL <= 5*time.Second || tracerCacheTTL >= 60*time.Second {
		log.Warnf("Configured tracer cache ttl is not within accepted range (%ds - %ds): %s. Defaulting to %s", 5, 10, tracerCacheTTL, defaultTracerCacheTTL)
		tracerCacheTTL = defaultTracerCacheTTL
	}

	return &Service{
		ctx:             context.Background(),
		firstUpdate:     true,
		refreshInterval: refreshInterval,
		remoteConfigKey: remoteConfigKey,
		products:        make(map[pbgo.Product]struct{}),
		newProducts:     make(map[pbgo.Product]struct{}),
		db:              db,
		client:          client,
		uptane:          uptaneClient,
		TracerInfos:     NewTracerCache(tracerCacheSize, tracerCacheTTL, time.Second),
	}, nil
}

// Start the remote configuration management service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		for {
			select {
			case <-time.After(s.refreshInterval):
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
	previousState, err := s.uptane.State()
	if err != nil {
		return err
	}
	if s.forceRefresh() {
		previousState = uptane.State{}
	}
	response, err := s.client.Fetch(s.ctx, previousState, s.TracerInfos.Tracers(), s.products, s.newProducts)
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
	s.newProducts = make(map[pbgo.Product]struct{})
	for _, subscriber := range s.subscribers {
		err := s.refreshSubscriber(subscriber)
		if err != nil {
			log.Errorf("could not notify a remote-config subscriber: %v", err)
		}
	}
	return nil
}

func (s *Service) forceRefresh() bool {
	return s.firstUpdate
}

// TODO(RCM-34): rework the subscribers API

func getTargetProduct(path string) (pbgo.Product, error) {
	splits := strings.SplitN(path, "/", 3)
	if len(splits) < 3 {
		return pbgo.Product(0), fmt.Errorf("failed to determine product for target file %s", path)
	}
	product, found := pbgo.Product_value[splits[1]]
	if !found {
		return pbgo.Product(0), fmt.Errorf("failed to determine product for target file %s", path)
	}
	return pbgo.Product(product), nil
}

func (s *Service) refreshSubscriber(subscriber *Subscriber) error {
	configResponse, err := s.GetConfigs(subscriber.product)
	if err != nil {
		return err
	}
	if err := subscriber.callback(configResponse); err != nil {
		return err
	}

	subscriber.lastUpdate = time.Now()

	return nil
}

// GetConfigs returns the current config files
func (s *Service) GetConfigs(product pbgo.Product) (*pbgo.ConfigResponse, error) {
	currentTargets, err := s.uptane.Targets()
	if err != nil {
		return nil, err
	}
	var targetFiles []*pbgo.File
	for targetPath := range currentTargets {
		p, err := getTargetProduct(targetPath)
		if err != nil {
			return nil, err
		}
		if product == p {
			targetContent, err := s.uptane.TargetFile(targetPath)
			if err != nil {
				return nil, err
			}
			targetFiles = append(targetFiles, &pbgo.File{
				Path: targetPath,
				Raw:  targetContent,
			})
		}
	}
	return &pbgo.ConfigResponse{
		TargetFiles: targetFiles,
	}, nil
}

// RegisterSubscriber registers a subscriber
func (s *Service) RegisterSubscriber(subscriber *Subscriber) {
	s.Lock()
	defer s.Unlock()
	s.subscribers = append(s.subscribers, subscriber)
	if _, ok := s.products[subscriber.product]; ok {
		return
	}
	s.newProducts[subscriber.product] = struct{}{}
}

// UnregisterSubscriber unregisters a subscriber
func (s *Service) UnregisterSubscriber(subscriber *Subscriber) {
	s.Lock()
	defer s.Unlock()
	var subscribers []*Subscriber
	for _, sub := range s.subscribers {
		if sub != subscriber {
			subscribers = append(subscribers, sub)
		}
	}
	s.subscribers = subscribers
}

// HasSubscriber returns true if the product already registered a subscriber
func (s *Service) HasSubscriber(product pbgo.Product) bool {
	s.Lock()
	defer s.Unlock()
	for _, s := range s.subscribers {
		if s.product == product {
			return true
		}
	}
	return false
}
