// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remoteconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/theupdateframework/go-tuf/data"
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
//
// This does not imply the raw file is cached by this client.
type ConfigState struct {
	Product string
	ID      string
	Version uint64
}

// CachedFile describes a cached file stored by the agent client
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
	roots         [][]byte
	latestTargets *data.Targets

	// Config file storage
	apmConfigs      map[string]APMSamplingConfig
	cwsDDConfigs    map[string]ConfigCWSDD
	ldConfigs       map[string]LDConfig
	featuresConfigs map[string]FeaturesConfig
}

// NewRepository creates a new remote config respository that will track
// both TUF metadata and raw config files for a client.
func NewRepository(embeddedRoot []byte) *Repository {
	var roots [][]byte
	if embeddedRoot != nil {
		roots = [][]byte{embeddedRoot}
	}

	return &Repository{
		roots:           roots,
		latestTargets:   data.NewTargets(),
		apmConfigs:      make(map[string]APMSamplingConfig),
		cwsDDConfigs:    make(map[string]ConfigCWSDD),
		ldConfigs:       make(map[string]LDConfig),
		featuresConfigs: make(map[string]FeaturesConfig),
	}
}

func (r *Repository) APMConfigs() map[string]APMSamplingConfig {
	return r.apmConfigs
}

// Update processes the ClientGetConfigsResponse from the Agent and updates the
// configuration state
func (r *Repository) Update(update Update) error {
	// 1: Deserialize the TUF Targets
	updatedTargets, err := decodeTargets(update.TUFTargets)
	if err != nil {
		return err
	}

	result := newUpdateResult()

	clientConfigsMap := make(map[string]struct{})
	for _, f := range update.ClientConfigs {
		clientConfigsMap[f] = struct{}{}
	}

	// 2: Check the config list and mark any missing configs as "to be removed"
	for f := range r.apmConfigs {
		if _, ok := clientConfigsMap[f]; !ok {
			result.removedAPM = append(result.removedAPM, f)
		}
	}
	for f := range r.cwsDDConfigs {
		if _, ok := clientConfigsMap[f]; !ok {
			result.removedCWSDD = append(result.removedCWSDD, f)
		}
	}
	for f := range r.ldConfigs {
		if _, ok := clientConfigsMap[f]; !ok {
			result.removedLD = append(result.removedLD, f)
		}
	}
	for f := range r.featuresConfigs {
		if _, ok := clientConfigsMap[f]; !ok {
			result.removedFeatures = append(result.removedFeatures, f)
		}
	}

	log.Printf("Removed APM: %v", result.removedAPM)
	log.Printf("Removed CWSDD: %v", result.removedCWSDD)
	log.Printf("Removed LD: %v", result.removedLD)
	log.Printf("Removed Features: %v", result.removedFeatures)

	// 3: For all the files referenced in this update
	for _, path := range update.ClientConfigs {
		meta, ok := updatedTargets.Targets[path]
		if !ok {
			return fmt.Errorf("missing config file in TUF targets - %s", path)
		}

		// 3.a: Extract the product and ID from the path
		parsedPath, err := parseConfigPath(path)
		if err != nil {
			return err
		}

		// 3.b/3.retermine if the configuration is new or updated
		var exists bool
		switch parsedPath.Product {
		case ProductAPMSampling:
			var asc APMSamplingConfig
			asc, exists = r.apmConfigs[path]
			if exists && hashesEqual(meta.Hashes, asc.Metadata.Hashes) {
				continue
			}
		case ProductCWSDD:
			var cddc ConfigCWSDD
			cddc, exists = r.cwsDDConfigs[path]
			if exists && hashesEqual(meta.Hashes, cddc.Metadata.Hashes) {
				continue
			}
		case ProductFeatures:
			var fc FeaturesConfig
			fc, exists = r.featuresConfigs[path]
			if exists && hashesEqual(meta.Hashes, fc.Metadata.Hashes) {
				continue
			}
		case ProductLiveDebugging:
			var ldc LDConfig
			ldc, exists = r.ldConfigs[path]
			if exists && hashesEqual(meta.Hashes, ldc.Metadata.Hashes) {
				continue
			}
		}

		// 3.d: Ensure that the raw configuration file is present in the
		// update payload.
		if _, ok := update.TargetFiles[path]; !ok {
			return fmt.Errorf("missing update file - %s", path)
		}

		// 3.e: Deserialize the configuration.
		// 3.f: Store the update details for application later
		//
		// Note: We don't have to worry about extra fields as mentioned
		// in the RFC because the encoding/json library handles that for us.
		m, err := newConfigMetadata(parsedPath, meta)
		if err != nil {
			return err
		}

		switch parsedPath.Product {
		case ProductAPMSampling:
			c, err := parseConfigAPMSampling(update.TargetFiles[path])
			if err != nil {
				return err
			}
			c.Metadata = m
			if exists {
				result.updatedAPM[path] = c
			} else {
				result.newAPM[path] = c
			}
		case ProductCWSDD:
			c, err := parseConfigCWSDD(update.TargetFiles[path])
			if err != nil {
				return err
			}
			c.Metadata = m
			if exists {
				result.updatedCWSDD[path] = c
			} else {
				result.newCWSDD[path] = c
			}
		case ProductFeatures:
			c, err := parseFeaturesConfing(update.TargetFiles[path])
			if err != nil {
				return err
			}
			c.Metadata = m
			if exists {
				result.updatedFeatures[path] = c
			} else {
				result.newFeatures[path] = c
			}
		case ProductLiveDebugging:
			c, err := parseLDConfig(update.TargetFiles[path])
			if err != nil {
				return err
			}
			c.Metadata = m
			if exists {
				result.updatedLD[path] = c
			} else {
				result.newLD[path] = c
			}
		}
	}

	// 4.a: Store the new targets.signed.custom.client_state
	// This data is contained within the TUF Targets file so storing that
	// covers this as well.
	r.latestTargets = updatedTargets

	// 4.b/4.rave the new state and apply cleanups
	err = r.applyUpdateResult(update, result)
	if err != nil {
		return err
	}

	return nil
}

// applyUpdateResult changes the state of the client based on the given update.
//
// The update is guaranteed to succeed at this point, having been vetted and the details
// needed to apply the update stored in the `updateResult`.
func (r *Repository) applyUpdateResult(update Update, result updateResult) error {
	// 4.b Save all the updated and new config files
	for path, config := range result.updatedAPM {
		log.Printf("Applying updated APM file %s", path)
		r.apmConfigs[path] = config
	}
	for path, config := range result.updatedCWSDD {
		log.Printf("Applying updated CWSDD file %s", path)
		r.cwsDDConfigs[path] = config
	}
	for path, config := range result.updatedLD {
		log.Printf("Applying updated LD file %s", path)
		r.ldConfigs[path] = config
	}
	for path, config := range result.updatedFeatures {
		log.Printf("Applying updated features file %s", path)
		r.featuresConfigs[path] = config
	}

	for path, config := range result.newAPM {
		log.Printf("Applying new APM file %s", path)
		r.apmConfigs[path] = config
	}
	for path, config := range result.newCWSDD {
		log.Printf("Applying new CWSDD file %s", path)
		r.cwsDDConfigs[path] = config
	}
	for path, config := range result.newLD {
		log.Printf("Applying new LD file %s", path)
		r.ldConfigs[path] = config
	}
	for path, config := range result.newFeatures {
		log.Printf("Applying new features file %s", path)
		r.featuresConfigs[path] = config
	}

	// 5.b Clean up the cache of any removed configs
	for _, file := range result.removedAPM {
		log.Printf("Removing old APM file %s", file)
		delete(r.apmConfigs, file)
	}
	for _, file := range result.removedCWSDD {
		log.Printf("Remove old CWSDD file %s", file)
		delete(r.cwsDDConfigs, file)
	}
	for _, file := range result.removedLD {
		log.Printf("Removing old LD file %s", file)
		delete(r.ldConfigs, file)
	}
	for _, file := range result.removedFeatures {
		log.Printf("Removing old features file %s", file)
		delete(r.featuresConfigs, file)
	}

	return nil
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

// CurrentState returns all of the information needed to
// make an update for new configurations.
func (r *Repository) CurrentState() RepositoryState {
	var configs []ConfigState
	var cached []CachedFile

	for path, config := range r.apmConfigs {
		configs = append(configs, configStateFromMetadata(config.Metadata))
		cached = append(cached, cachedFileFromMetadata(path, config.Metadata))
	}

	for path, config := range r.cwsDDConfigs {
		configs = append(configs, configStateFromMetadata(config.Metadata))
		cached = append(cached, cachedFileFromMetadata(path, config.Metadata))
	}

	for path, config := range r.ldConfigs {
		configs = append(configs, configStateFromMetadata(config.Metadata))
		cached = append(cached, cachedFileFromMetadata(path, config.Metadata))
	}

	for path, config := range r.featuresConfigs {
		configs = append(configs, configStateFromMetadata(config.Metadata))
		cached = append(cached, cachedFileFromMetadata(path, config.Metadata))
	}

	return RepositoryState{
		Configs:        configs,
		CachedFiles:    cached,
		TargetsVersion: r.latestTargets.Version,
		RootsVersion:   1,
	}
}

// An updateResult allows the client to apply the update as a transaction
// after validating all required preconditions
type updateResult struct {
	removedAPM      []string
	removedCWSDD    []string
	removedLD       []string
	removedFeatures []string

	updatedAPM      map[string]APMSamplingConfig
	updatedCWSDD    map[string]ConfigCWSDD
	updatedLD       map[string]LDConfig
	updatedFeatures map[string]FeaturesConfig

	newAPM      map[string]APMSamplingConfig
	newCWSDD    map[string]ConfigCWSDD
	newLD       map[string]LDConfig
	newFeatures map[string]FeaturesConfig
}

func newUpdateResult() updateResult {
	return updateResult{
		updatedAPM:      make(map[string]APMSamplingConfig),
		updatedCWSDD:    make(map[string]ConfigCWSDD),
		updatedLD:       make(map[string]LDConfig),
		updatedFeatures: make(map[string]FeaturesConfig),

		newAPM:      make(map[string]APMSamplingConfig),
		newCWSDD:    make(map[string]ConfigCWSDD),
		newLD:       make(map[string]LDConfig),
		newFeatures: make(map[string]FeaturesConfig),
	}
}

func decodeTargets(raw []byte) (*data.Targets, error) {
	var signed data.Signed

	err := json.Unmarshal(raw, &signed)
	if err != nil {
		return nil, err
	}

	var targets data.Targets
	err = json.Unmarshal(signed.Signed, &targets)
	if err != nil {
		return nil, err
	}

	return &targets, nil
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
