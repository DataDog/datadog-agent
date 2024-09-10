// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"net/url"
	"path"
	"sync"
	"time"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/go-tuf/data"
	tufutil "github.com/DataDog/go-tuf/util"
)

const maxUpdateFrequency = 50 * time.Second

// httpUptaneClient is used to mock the uptane component for testing
type httpUptaneClient interface {
	Update() error
	State() (uptane.State, error)
	DirectorRoot(version uint64) ([]byte, error)
	StoredOrgUUID() (string, error)
	Targets() (data.TargetFiles, error)
	TargetFile(path string) ([]byte, error)
	TargetsMeta() ([]byte, error)
	TargetsCustom() ([]byte, error)
	TUFVersionState() (uptane.TUFVersions, error)
}

// HTTPClient defines a client that can be used to fetch Remote Configurations from an HTTP(s)-based backend
type HTTPClient struct {
	sync.Mutex

	lastUpdate time.Time

	// rcType is used to differentiate multiple RC services running in a single agent.
	// Today, it is simply logged as a prefix in all log messages to help when triaging
	// via logs.
	rcType string

	api    api.API
	uptane httpUptaneClient
}

// NewHTTPClient creates a new HTTPClient that can be used to fetch Remote Configurations from an HTTP(s)-based backend
func NewHTTPClient(cfg model.Reader, baseRawURL, host, site, apiKey, rcKey, agentVersion string) (*HTTPClient, error) {

	dbPath := path.Join(cfg.GetString("run_path"), "remote-config-cdn.db")
	db, err := openCacheDB(dbPath, agentVersion, apiKey)
	if err != nil {
		return nil, err
	}
	uptaneClientOptions := []uptane.ClientOption{
		uptane.WithConfigRootOverride(site, ""),
		uptane.WithDirectorRootOverride(site, ""),
	}
	baseURL, err := url.Parse(baseRawURL)
	if err != nil {
		return nil, err
	}
	authKeys, err := getRemoteConfigAuthKeys(apiKey, rcKey)
	if err != nil {
		return nil, err
	}
	http, err := api.NewHTTPClient(authKeys.apiAuth(), cfg, baseURL)
	if err != nil {
		return nil, err
	}
	uptaneHTTPClient, err := uptane.NewHTTPClient(
		db,
		host,
		site,
		apiKey,
		newRCBackendOrgUUIDProvider(http),
		uptaneClientOptions...,
	)
	if err != nil {
		return nil, err
	}

	return &HTTPClient{
		rcType: "CDN",
		uptane: uptaneHTTPClient,
		api:    http,
	}, nil
}

func (s *HTTPClient) update() error {
	s.Lock()
	defer s.Unlock()

	err := s.uptane.Update()
	if err != nil {
		return err
	}

	return nil
}

func (s *HTTPClient) shouldUpdate() bool {
	s.Lock()
	defer s.Unlock()
	if time.Since(s.lastUpdate) > maxUpdateFrequency {
		s.lastUpdate = time.Now()
		return true
	}
	return false
}

// GetCDNConfigUpdate returns any updated configs. If multiple requests have been made
// in a short amount of time, a cached response is returned. If RC has been disabled,
// an error is returned. If there is no update (the targets version is up-to-date) nil
// is returned for both the update and error.
func (s *HTTPClient) GetCDNConfigUpdate(
	products []string,
	currentTargetsVersion, currentRootVersion uint64,
	cachedTargetFiles []*pbgo.TargetFileMeta,
) (*state.Update, error) {

	if !s.shouldUpdate() {
		return s.getUpdate(products, currentTargetsVersion, currentRootVersion, cachedTargetFiles)
	}

	// check org status in the backend. If RC is disabled, return current state.
	response, err := s.api.FetchOrgStatus(context.Background())
	if err != nil || !response.Enabled || !response.Authorized {
		return s.getUpdate(products, currentTargetsVersion, currentRootVersion, cachedTargetFiles)
	}

	err = s.update()
	if err != nil {
		_ = log.Warn(fmt.Sprintf("Error updating CDN config repo: %v", err))
	}

	return s.getUpdate(products, currentTargetsVersion, currentRootVersion, cachedTargetFiles)
}

func (s *HTTPClient) getUpdate(
	products []string,
	currentTargetsVersion, currentRootVersion uint64,
	cachedTargetFiles []*pbgo.TargetFileMeta,
) (*state.Update, error) {
	s.Lock()
	defer s.Unlock()

	tufVersions, err := s.uptane.TUFVersionState()
	if err != nil {
		return nil, err
	}
	if tufVersions.DirectorTargets == currentTargetsVersion {
		return nil, nil
	}
	roots, err := s.getNewDirectorRoots(currentRootVersion, tufVersions.DirectorRoot)
	if err != nil {
		return nil, err
	}
	targetsRaw, err := s.uptane.TargetsMeta()
	if err != nil {
		return nil, err
	}
	targetFiles, err := s.getTargetFiles(rdata.StringListToProduct(products), cachedTargetFiles)
	if err != nil {
		return nil, err
	}

	canonicalTargets, err := enforceCanonicalJSON(targetsRaw)
	if err != nil {
		return nil, err
	}

	directorTargets, err := s.uptane.Targets()
	if err != nil {
		return nil, err
	}

	productsMap := make(map[string]struct{})
	for _, product := range products {
		productsMap[product] = struct{}{}
	}
	configs := make([]string, 0)
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
			continue
		}

		configs = append(configs, path)
	}

	fileMap := make(map[string][]byte, len(targetFiles))
	for _, f := range targetFiles {
		fileMap[f.Path] = f.Raw
	}

	return &state.Update{
		TUFRoots:      roots,
		TUFTargets:    canonicalTargets,
		TargetFiles:   fileMap,
		ClientConfigs: configs,
	}, nil
}

func (s *HTTPClient) getNewDirectorRoots(currentVersion uint64, newVersion uint64) ([][]byte, error) {
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

func (s *HTTPClient) getTargetFiles(products []rdata.Product, cachedTargetFiles []*pbgo.TargetFileMeta) ([]*pbgo.File, error) {
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
