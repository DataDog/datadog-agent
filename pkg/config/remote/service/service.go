// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service/tuf"
	"github.com/DataDog/datadog-agent/pkg/config/remote/store"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	minimalRefreshInterval = time.Second * 5
	defaultMaxBucketSize   = 10
	defaultURL             = ""
)

// Opts defines the remote config service options
type Opts struct {
	URL                    string
	APIKey                 string
	RemoteConfigurationKey string
	Hostname               string
	DBPath                 string
	RefreshInterval        time.Duration
	MaxBucketSize          int
	ReadOnly               bool
}

// Service defines the remote config management service responsible for fetching, storing
// and dispatching the configurations
type Service struct {
	sync.RWMutex
	ctx      context.Context
	opts     Opts
	store    *store.Store
	client   Client
	director *tuf.DirectorClient
	config   *tuf.ConfigClient

	subscribers           []*Subscriber
	configSnapshotVersion uint64
	configRootVersion     uint64
	directorRootVersion   uint64
	orgID                 string
}

// Refresh configurations by:
// - collecting the new subscribers or the one whose configuration has expired
// - create a query
// - send the query to the backend
//
func (s *Service) refresh() {
	log.Debug("Refreshing configurations")

	request := pbgo.ClientLatestConfigsRequest{
		AgentVersion:                 version.AgentVersion,
		Hostname:                     s.opts.Hostname,
		CurrentConfigSnapshotVersion: s.configSnapshotVersion,
		CurrentConfigRootVersion:     s.configRootVersion,
		CurrentDirectorRootVersion:   s.directorRootVersion,
	}

	// determine which configuration we need to refresh
	var refreshSubscribers = map[string][]*Subscriber{}

	s.RLock()
	defer s.RUnlock()

	now := time.Now()

	for _, subscriber := range s.subscribers {
		product := subscriber.product
		if subscriber.lastUpdate.Add(subscriber.refreshRate).Before(now) {
			log.Debugf("Add '%s' to the list of configurations to refresh", product)

			if subscriber.lastUpdate.IsZero() {
				request.NewProducts = append(request.NewProducts, product)
			} else {
				request.Products = append(request.Products, product)
			}

			refreshSubscribers[product.String()] = append(refreshSubscribers[product.String()], subscriber)
		}
	}

	if len(refreshSubscribers) == 0 {
		log.Debugf("Nothing to fetch")
		return
	}

	// Fetch the configuration from the backend
	response, err := s.client.Fetch(s.ctx, &request)
	if err != nil {
		log.Errorf("Failed to fetch remote configuration: %s", err)
		return
	}

	if response.DirectorMetas == nil {
		log.Debugf("No new configuration")
		return
	}

	if err := s.verifyResponseMetadata(response); err != nil {
		log.Errorf("Failed to verify configuration: %s", err)
		return
	}

	if err := s.verifyTargetFiles(response.TargetFiles); err != nil {
		log.Errorf("Failed to verify target files: %s", err)
		return
	}

	refreshedProducts := make(map[*pbgo.DelegatedMeta][]*pbgo.File)

TARGETFILE:
	for _, targetFile := range response.TargetFiles {
		product, err := getTargetProduct(targetFile.Path)
		if err != nil {
			log.Error(err)
			continue
		}

		for _, delegatedTarget := range response.ConfigMetas.DelegatedTargets {
			if delegatedTarget.Role == product {
				log.Debugf("Received configuration for product %s", product)
				refreshedProducts[delegatedTarget] = append(refreshedProducts[delegatedTarget], targetFile)
				continue TARGETFILE
			}
		}

		log.Errorf("Failed to find delegated target for %s", product)
		return
	}

	log.Debugf("Possibly notify subscribers")
	for delegatedTarget, targetFiles := range refreshedProducts {
		configResponse := &pbgo.ConfigResponse{
			ConfigSnapshotVersion:        response.DirectorMetas.Snapshot.Version,
			ConfigDelegatedTargetVersion: delegatedTarget.Version,
			DirectoryRoots:               response.DirectorMetas.Roots,
			DirectoryTargets:             response.DirectorMetas.Targets,
			TargetFiles:                  targetFiles,
		}

		product := delegatedTarget.GetRole()
	SUBSCRIBER:
		for _, subscriber := range refreshSubscribers[product] {
			if response.DirectorMetas.Snapshot.Version <= subscriber.lastVersion {
				log.Debugf("Nothing to do, subscriber version %d > %d", subscriber.lastVersion, response.DirectorMetas.Snapshot.Version)
				continue
			}

			if err := s.notifySubscriber(subscriber, configResponse); err != nil {
				log.Errorf("failed to notify subscriber: %s", err)
				continue SUBSCRIBER
			}
		}

		if err := s.store.StoreConfig(product, configResponse); err != nil {
			log.Errorf("failed to persistent config for product %s: %s", product, err)
		}
	}

	if response.ConfigMetas != nil {
		if rootCount := len(response.ConfigMetas.Roots); rootCount > 0 {
			s.configRootVersion = response.ConfigMetas.Roots[rootCount-1].Version
		}
		if response.ConfigMetas.Snapshot != nil {
			s.configSnapshotVersion = response.ConfigMetas.Snapshot.Version
		}
	}

	if response.DirectorMetas != nil {
		if rootCount := len(response.DirectorMetas.Roots); rootCount > 0 {
			s.directorRootVersion = response.DirectorMetas.Roots[rootCount-1].Version
		}
	}

	log.Debugf("Stored last known version to config snapshot %d, config root %d, snapshot root %d", s.configSnapshotVersion, s.configRootVersion, s.directorRootVersion)
}

func getTargetProduct(path string) (string, error) {
	splits := strings.SplitN(path, "/", 3)
	if len(splits) < 3 {
		return "", fmt.Errorf("Failed to determine product for target file %s", path)
	}

	return splits[1], nil
}

type targetCustom struct {
	OrgID string `json:"org_id"`
}

// Verify that both director and config metadata from the response and
// that the target files provided in the response match the ones specified
// in the director and config repositories
func (s *Service) verifyResponseMetadata(response *pbgo.LatestConfigsResponse) error {
	if err := s.director.Update(response); err != nil {
		return err
	}

	if err := s.config.Update(response); err != nil {
		return err
	}

	log.Debugf("Response successfully verified")

	if response.DirectorMetas != nil && response.DirectorMetas.Snapshot != nil &&
		response.ConfigMetas.Snapshot.Version <= s.configSnapshotVersion {
		return fmt.Errorf("snapshot version %d is older than current version %d", response.ConfigMetas.Snapshot.Version, s.configSnapshotVersion)
	}

	for _, target := range response.TargetFiles {
		name := tuf.TrimHash(target.Path)

		log.Debugf("Considering director target %s", name)
		directorTarget, err := s.director.Target(name)
		if err != nil {
			return fmt.Errorf("failed to find target '%s' in director repository", name)
		}

		configTarget, err := s.config.Target(name)
		if err != nil {
			return fmt.Errorf("failed to find target '%s' in config repository", name)
		}

		if configTarget.Length != directorTarget.Length {
			return fmt.Errorf("target '%s' has size %d in directory repository and %d in config repository", name, configTarget.Length, directorTarget.Length)
		}

		for kind, directorHash := range directorTarget.Hashes {
			configHash, found := configTarget.Hashes[kind]
			if !found {
				return fmt.Errorf("hash '%s' found in directory repository and not in config repository", directorHash)
			}

			if !bytes.Equal([]byte(directorHash), []byte(configHash)) {
				return fmt.Errorf("directory hash '%s' is not equal to config repository '%s'", string(directorHash), string(configHash))
			}
		}

		/*
			// TODO(lebauce): remove this when backend is ready

			// Backend does not return customs for config repository.
			var directorCustom, configCustom []byte
			if directorTarget.Custom != nil {
				directorCustom = *directorTarget.Custom
			}

			if configTarget.Custom != nil {
				configCustom = *configTarget.Custom
			}

			if bytes.Compare(directorCustom, configCustom) != 0 {
				return fmt.Errorf("directory custom '%s' is not equal to config custom '%s'", string(directorCustom), string(configCustom))
			}
		*/

		if directorTarget.Custom == nil {
			return fmt.Errorf("director target %s has no custom field", name)
		}

		var custom targetCustom
		if err := json.Unmarshal([]byte(*directorTarget.Custom), &custom); err != nil {
			return fmt.Errorf("failed to decode target custom for %s: %w", name, err)
		}

		if custom.OrgID != s.orgID {
			return fmt.Errorf("unexpected custom organization id: %s", custom.OrgID)
		}
	}

	return nil
}

// Verify the target files checksum provided in the response with
// the one specified in the config repository
func (s *Service) verifyTargetFiles(targetFiles []*pbgo.File) error {
	for _, targetFile := range targetFiles {
		path := tuf.TrimHash(targetFile.Path)
		buffer := &bufferDestination{}
		if err := s.config.Download(path, buffer); err != nil {
			return fmt.Errorf("failed to download target file %s: %w", targetFile.Path, err)
		}

		targetMeta, err := s.config.Target(path)
		if err != nil {
			return err
		}

		if len(targetMeta.HashAlgorithms()) == 0 {
			return fmt.Errorf("target file %s has no hash", path)
		}

		for _, algorithm := range targetMeta.HashAlgorithms() {
			var checksum []byte
			switch algorithm {
			case "sha256":
				sha256Checksum := sha256.Sum256(targetFile.Raw)
				checksum = sha256Checksum[:]
			case "sha512":
				sha512Checksum := sha512.Sum512(targetFile.Raw)
				checksum = sha512Checksum[:]
			default:
				return fmt.Errorf("unsupported checksum %s", algorithm)
			}

			if !bytes.Equal(checksum, targetMeta.Hashes[algorithm]) {
				return fmt.Errorf("target file %s has invalid checksum", string(checksum))
			}
		}
	}

	return nil
}

type bufferDestination struct {
	bytes.Buffer
}

func (d *bufferDestination) Delete() error {
	return nil
}

func (s *Service) notifySubscriber(subscriber *Subscriber, configResponse *pbgo.ConfigResponse) error {
	log.Debugf("Notifying subscriber %s with version %d", subscriber.product, configResponse.DirectoryTargets.Version)

	if err := subscriber.callback(configResponse); err != nil {
		return err
	}

	subscriber.lastUpdate = time.Now()
	subscriber.lastVersion = configResponse.DirectoryTargets.Version

	return nil
}

// RegisterSubscriber registers a new subscriber for a product's configurations
func (s *Service) RegisterSubscriber(subscriber *Subscriber) {
	s.Lock()
	s.subscribers = append(s.subscribers, subscriber)
	s.Unlock()

	product := subscriber.product
	log.Debugf("New registered subscriber for %s", product.String())

	config, err := s.store.GetLastConfig(product.String())
	if err == nil {
		log.Debugf("Found cached configuration for product %s", product)
		if err := s.notifySubscriber(subscriber, config); err != nil {
			log.Error(err)
		}
	} else {
		log.Debugf("No stored configuration for product %s", product)
	}
}

// UnregisterSubscriber unregisters a subscriber for a product's configurations
func (s *Service) UnregisterSubscriber(unregister *Subscriber) {
	s.Lock()
	for i, subscriber := range s.subscribers {
		if subscriber == unregister {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
		}
	}
	s.Unlock()
}

// Start the remote configuration management service
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		for {
			select {
			case <-time.After(s.opts.RefreshInterval):
				s.refresh()
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// GetConfigs returns config for the given product
func (s *Service) GetConfigs(product string) ([]*pbgo.ConfigResponse, error) {
	return s.store.GetConfigs(product)
}

// GetStore returns the configuration store
func (s *Service) GetStore() *store.Store {
	return s.store
}

// NewService instantiates a new remote configuration management service
func NewService(opts Opts) (*Service, error) {
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = config.Datadog.GetDuration("remote_configuration.refresh_interval")
	}

	if opts.RefreshInterval < minimalRefreshInterval {
		opts.RefreshInterval = minimalRefreshInterval
	}

	if opts.DBPath == "" {
		opts.DBPath = path.Join(config.Datadog.GetString("run_path"), "remote-config.db")
	}

	if opts.APIKey == "" {
		apiKey := config.Datadog.GetString("api_key")
		if config.Datadog.IsSet("remote_configuration.api_key") {
			apiKey = config.Datadog.GetString("remote_configuration.api_key")
		}
		opts.APIKey = config.SanitizeAPIKey(apiKey)
	}

	if opts.RemoteConfigurationKey == "" {
		opts.RemoteConfigurationKey = config.Datadog.GetString("remote_configuration.key")
	}

	if opts.URL == "" {
		opts.URL = config.Datadog.GetString("remote_configuration.endpoint")
	}

	if opts.Hostname == "" {
		hostname, err := util.GetHostname(context.Background())
		if err != nil {
			return nil, err
		}
		opts.Hostname = hostname
	}

	if opts.MaxBucketSize <= 0 {
		opts.MaxBucketSize = defaultMaxBucketSize
	}

	if opts.URL == "" {
		opts.URL = defaultURL
	}

	split := strings.SplitN(opts.RemoteConfigurationKey, "/", 3)
	if len(split) < 3 {
		return nil, fmt.Errorf("invalid remote configuration key format, should be datacenter/org_id/app_key")
	}

	datacenter, org, appKey := split[0], split[1], split[2]

	store, err := store.NewStore(opts.DBPath, !opts.ReadOnly, opts.MaxBucketSize, datacenter+"/"+org)
	if err != nil {
		return nil, err
	}

	return &Service{
		ctx:      context.Background(),
		client:   NewHTTPClient(opts.URL, opts.APIKey, appKey, opts.Hostname),
		store:    store,
		director: tuf.NewDirectorClient(store),
		config:   tuf.NewConfigClient(store),
		opts:     opts,
		orgID:    org,
	}, nil
}

// dumpResponse dumps the response to the standard output and
// should only be used for debugging purposes
// nolint:deadcode,unused
func dumpResponse(response *pbgo.LatestConfigsResponse) {
	fmt.Printf("Response:\n")
	if response.DirectorMetas != nil {
		fmt.Printf(" Directory:\n")
		fmt.Printf("  Roots:\n")
		for _, root := range response.DirectorMetas.Roots {
			fmt.Printf("  - %s\n", string(root.Raw))
		}
		if response.DirectorMetas.Timestamp != nil {
			fmt.Printf("  Timestamp: %+v\n", string(response.DirectorMetas.Timestamp.Raw))
		}
		if response.DirectorMetas.Snapshot != nil {
			fmt.Printf("  Snapshot: %+v\n", string(response.DirectorMetas.Snapshot.Raw))
		}
		if response.DirectorMetas.Targets != nil {
			fmt.Printf("  Targets: %+v\n", string(response.DirectorMetas.Targets.Raw))
		}
	}
	if response.ConfigMetas != nil {
		fmt.Printf(" Config:\n")
		fmt.Printf("  Roots: %+v\n", response.ConfigMetas.Roots)
		for _, root := range response.ConfigMetas.Roots {
			fmt.Printf("  - %s\n", string(root.Raw))
		}
		if response.ConfigMetas.Timestamp != nil {
			fmt.Printf("  Timestamp: %+v\n", string(response.ConfigMetas.Timestamp.Raw))
		}
		if response.ConfigMetas.Snapshot != nil {
			fmt.Printf("  Snapshot: %+v\n", string(response.ConfigMetas.Snapshot.Raw))
		}
		if response.ConfigMetas.TopTargets != nil {
			fmt.Printf("  TopTargets: %+v\n", string(response.ConfigMetas.TopTargets.Raw))
		}
		if response.ConfigMetas.DelegatedTargets != nil {
			fmt.Printf("  Delegated targets:\n")
			for _, delegatedTarget := range response.ConfigMetas.DelegatedTargets {
				fmt.Printf("   - %v\n", string(delegatedTarget.Raw))
			}
		}
	}
	fmt.Printf("Target files:\n")
	for _, targetFile := range response.TargetFiles {
		fmt.Printf("   - %s\n", targetFile.Path)
	}
}
