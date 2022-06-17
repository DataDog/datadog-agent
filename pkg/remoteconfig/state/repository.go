// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/theupdateframework/go-tuf/data"
)

var (
	ErrMalformedEmbeddedRoot = errors.New("malformed embedded TUF root file provided")
)

// RepositoryState contains all of the information about the current config files
// stored by the client to be able to make an update request to an Agent
type RepositoryState struct {
	Configs        []ConfigState
	CachedFiles    []CachedFile
	TargetsVersion int64
	RootsVersion   int64
}

// ConfigState describes an applied config by the agent client.
type ConfigState struct {
	Product string
	ID      string
	Version uint64
}

// CachedFile describes a cached file stored by the agent client
//
// Note: You may be wondering why this exists when `ConfigState` exists
// as well. The API for requesting updates does not mandate that a client
// cache config files. This implementation just happens to do so.
type CachedFile struct {
	Path   string
	Length uint64
	Hashes map[string][]byte
}

// An Update contains all the data needed to update a client's remote config repository state
type Update struct {
	// TUFRoots contains, in order, updated roots that this repository needs to keep up with TUF validation
	TUFRoots [][]byte
	// TUFTargets is the latest TUF Targets file and is used to validate raw config files
	TUFTargets []byte
	// TargetFiles stores the raw config files by their full TUF path
	TargetFiles map[string][]byte
	// ClientcConfigs is a list of TUF path's corresponding to config files designated for this repository
	ClientConfigs []string
}

// Repository is a remote config client used in a downstream process to retrieve
// remote config updates from an Agent.
type Repository struct {
	// TUF related data
	latestTargets  *data.Targets
	tufRootsClient *tufRootsClient

	// Config file storage
	metadata map[string]Metadata
	configs  map[string]map[string]interface{}
}

// NewRepository creates a new remote config repository that will track
// both TUF metadata and raw config files for a client.
func NewRepository(embeddedRoot []byte) (*Repository, error) {
	if embeddedRoot == nil {
		return nil, ErrMalformedEmbeddedRoot
	}

	configs := make(map[string]map[string]interface{})
	for _, product := range allProducts {
		configs[product] = make(map[string]interface{})
	}

	tufRootsClient, err := newTufRootsClient(embeddedRoot)
	if err != nil {
		return nil, err
	}

	return &Repository{
		latestTargets:  data.NewTargets(),
		tufRootsClient: tufRootsClient,
		metadata:       make(map[string]Metadata),
		configs:        configs,
	}, nil
}

// Update processes the ClientGetConfigsResponse from the Agent and updates the
// configuration state
func (r *Repository) Update(update Update) ([]string, error) {
	// TUF: Update the roots
	//
	// NWe don't want to partially update the state, so we need a temporary client to hold the new root
	// data until we know it's valid
	tmpRootClient, err := r.tufRootsClient.clone()
	if err != nil {
		return nil, err
	}
	err = tmpRootClient.updateRoots(update.TUFRoots)
	if err != nil {
		return nil, err
	}

	// 1: Validate and Deserialize the TUF Targets
	//
	// Note: This goes further than the RFC requires and validates the TUF targets metadata's signatures.
	// This is NOT required for most clients per the RFC.
	updatedTargets, err := tmpRootClient.validateTargets(update.TUFTargets)
	if err != nil {
		return nil, err
	}

	clientConfigsMap := make(map[string]struct{})
	for _, f := range update.ClientConfigs {
		clientConfigsMap[f] = struct{}{}
	}

	result := newUpdateResult()

	// 2: Check the config list and mark any missing configs as "to be removed"
	for _, configs := range r.configs {
		for path := range configs {
			if _, ok := clientConfigsMap[path]; !ok {
				result.removed = append(result.removed, path)
			}
		}
	}

	// 3: For all the files referenced in this update
	for _, path := range update.ClientConfigs {
		targetFileMetadata, ok := updatedTargets.Targets[path]
		if !ok {
			return nil, fmt.Errorf("missing config file in TUF targets - %s", path)
		}

		// 3.a: Extract the product and ID from the path
		parsedPath, err := parseConfigPath(path)
		if err != nil {
			return nil, err
		}

		storedMetadata, exists := r.metadata[path]
		if exists && hashesEqual(targetFileMetadata.Hashes, storedMetadata.Hashes) {
			continue
		}

		// 3.d: Ensure that the raw configuration file is present in the
		// update payload.
		raw, ok := update.TargetFiles[path]
		if !ok {
			return nil, fmt.Errorf("missing update file - %s", path)
		}

		// TUF: Validate the hash of the raw target file and ensure that it matches
		// the TUF metadata
		err = validateTargetFileHash(targetFileMetadata, raw)
		if err != nil {
			return nil, fmt.Errorf("error validating %s hash with TUF metadata - %v", path, err)
		}

		// 3.e: Deserialize the configuration.
		// 3.f: Store the update details for application later
		//
		// Note: We don't have to worry about extra fields as mentioned
		// in the RFC because the encoding/json library handles that for us.
		m, err := newConfigMetadata(parsedPath, targetFileMetadata)
		if err != nil {
			return nil, err
		}
		config, err := parseConfig(parsedPath.Product, raw, m)
		if err != nil {
			return nil, err
		}
		result.metadata[path] = m
		result.changed[parsedPath.Product][path] = config
	}

	// 4.a: Store the new targets.signed.custom.client_state
	// TUF: Store the updated roots now that everything has validated
	// This data is contained within the TUF Targets file so storing that
	// covers this as well.
	r.latestTargets = updatedTargets
	r.tufRootsClient = tmpRootClient

	// Upstream may not want to take any actions if the update result doesn't
	// change any configs.
	if result.isEmpty() {
		return nil, nil
	}

	changedProducts := make([]string, 0)
	for product, configs := range result.changed {
		if len(configs) > 0 {
			changedProducts = append(changedProducts, product)
		}
	}

	// 4.b/4.rave the new state and apply cleanups
	r.applyUpdateResult(update, result)

	return changedProducts, nil
}

func (r *Repository) getConfigs(product string) map[string]interface{} {
	configs, ok := r.configs[product]
	if !ok {
		return nil
	}

	return configs
}

// applyUpdateResult changes the state of the client based on the given update.
//
// The update is guaranteed to succeed at this point, having been vetted and the details
// needed to apply the update stored in the `updateResult`.
func (r *Repository) applyUpdateResult(update Update, result updateResult) {
	// 4.b Save all the updated and new config files
	for product, configs := range result.changed {
		for path, config := range configs {
			m := r.configs[product]
			m[path] = config
		}
	}
	for path, metadata := range result.metadata {
		r.metadata[path] = metadata
	}

	// 5.b Clean up the cache of any removed configs
	for _, path := range result.removed {
		delete(r.metadata, path)
		for _, configs := range r.configs {
			delete(configs, path)
		}
	}
}

// CurrentState returns all of the information needed to
// make an update for new configurations.
func (r *Repository) CurrentState() (RepositoryState, error) {
	var configs []ConfigState
	var cached []CachedFile

	for path, metadata := range r.metadata {
		configs = append(configs, configStateFromMetadata(metadata))
		cached = append(cached, cachedFileFromMetadata(path, metadata))
	}

	latestRoot, err := r.tufRootsClient.latestRoot()
	if err != nil {
		return RepositoryState{}, err
	}

	return RepositoryState{
		Configs:        configs,
		CachedFiles:    cached,
		TargetsVersion: r.latestTargets.Version,
		RootsVersion:   latestRoot.Version,
	}, nil
}

// An updateResult allows the client to apply the update as a transaction
// after validating all required preconditions
type updateResult struct {
	removed  []string
	metadata map[string]Metadata
	changed  map[string]map[string]interface{}
}

func newUpdateResult() updateResult {
	changed := make(map[string]map[string]interface{})

	for _, p := range allProducts {
		changed[p] = make(map[string]interface{})
	}

	return updateResult{
		removed:  make([]string, 0),
		metadata: make(map[string]Metadata),
		changed:  changed,
	}
}

func (ur updateResult) Log() {
	log.Printf("Removed Configs: %v", ur.removed)

	var b strings.Builder
	b.WriteString("Changed configs: [")
	for path := range ur.metadata {
		b.WriteString(path)
		b.WriteString(" ")
	}
	b.WriteString("]")

	log.Println(b.String())
}

func (ur updateResult) isEmpty() bool {
	return len(ur.removed) == 0 && len(ur.metadata) == 0
}

func configStateFromMetadata(m Metadata) ConfigState {
	return ConfigState{
		Product: m.Product,
		ID:      m.ID,
		Version: m.Version,
	}
}

func cachedFileFromMetadata(path string, m Metadata) CachedFile {
	return CachedFile{
		Path:   path,
		Length: m.RawLength,
		Hashes: m.Hashes,
	}
}

// hashesEqual checks if the hash values in the TUF metadata file match the stored
// hash values for a given config
func hashesEqual(tufHashes data.Hashes, storedHashes map[string][]byte) bool {
	for algorithm, value := range tufHashes {
		v, ok := storedHashes[algorithm]
		if !ok {
			continue
		}

		if !bytes.Equal(value, v) {
			return false
		}
	}

	return true
}
