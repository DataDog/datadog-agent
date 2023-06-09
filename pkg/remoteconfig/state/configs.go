// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DataDog/go-tuf/data"
)

// ErrNoConfigVersion occurs when a target file's custom meta is missing the config version
var ErrNoConfigVersion = errors.New("version missing in custom file meta")

func parseConfig(product string, raw []byte, metadata Metadata) (interface{}, error) {
	if _, validProduct := validProducts[product]; !validProduct {
		return nil, fmt.Errorf("unknown product: %s", product)
	}

	switch product {
	// ASM products are parsed directly in this client
	case ProductASMFeatures:
		return parseASMFeaturesConfig(raw, metadata)
	case ProductASMDD:
		return parseConfigASMDD(raw, metadata)
	case ProductASMData:
		return parseConfigASMData(raw, metadata)
	// case ProductAgentTask:
	// 	return ParseConfigAgentTask(raw, metadata)
	// Other products are parsed separately
	default:
		return RawConfig{
			Config:   raw,
			Metadata: metadata,
		}, nil
	}
}

// RawConfig holds a config that will be parsed separately
type RawConfig struct {
	Config   []byte
	Metadata Metadata
}

// GetConfigs returns the current configs of a given product
func (r *Repository) GetConfigs(product string) map[string]RawConfig {
	typedConfigs := make(map[string]RawConfig)
	configs := r.getConfigs(product)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(RawConfig)
		if !ok {
			panic("unexpected config stored as RawConfig")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}

// Metadata stores remote config metadata for a given configuration
type Metadata struct {
	Product     string
	ID          string
	Name        string
	Version     uint64
	RawLength   uint64
	Hashes      map[string][]byte
	ApplyStatus ApplyStatus
}

func newConfigMetadata(parsedPath configPath, tfm data.TargetFileMeta) (Metadata, error) {
	var m Metadata
	m.ID = parsedPath.ConfigID
	m.Product = parsedPath.Product
	m.Name = parsedPath.Name
	m.RawLength = uint64(tfm.Length)
	m.Hashes = make(map[string][]byte)
	for k, v := range tfm.Hashes {
		m.Hashes[k] = []byte(v)
	}
	v, err := fileMetaVersion(tfm)
	if err != nil {
		return Metadata{}, err
	}
	m.Version = v

	return m, nil
}

type fileMetaCustom struct {
	Version *uint64 `json:"v"`
}

func fileMetaVersion(fm data.TargetFileMeta) (uint64, error) {
	if fm.Custom == nil {
		return 0, ErrNoConfigVersion
	}
	fmc, err := parseFileMetaCustom(*fm.Custom)
	if err != nil {
		return 0, err
	}

	return *fmc.Version, nil
}

func parseFileMetaCustom(rawCustom []byte) (fileMetaCustom, error) {
	var custom fileMetaCustom
	err := json.Unmarshal(rawCustom, &custom)
	if err != nil {
		return fileMetaCustom{}, err
	}
	if custom.Version == nil {
		return fileMetaCustom{}, ErrNoConfigVersion
	}
	return custom, nil
}
