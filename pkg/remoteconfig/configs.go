// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remoteconfig

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/products/apmsampling"
	"github.com/theupdateframework/go-tuf/data"
)

/*
	To add support for a new product:

	1. Add the definition of the product to the const() block of products and the `allProducts` list.
	2. Define the serialized configuration struct as well as a function to parse the config from a []byte.
	3. Add the product to the `parseConfig` function
	4. Add a method on the `Repository` to retrieved typed configs for the product.
*/

var allProducts = []string{ProductAPMSampling, ProductCWSDD, ProductFeatures, ProductLiveDebugging}

const (
	// ProductAPMSampling is the apm sampling product
	ProductAPMSampling = "APM_SAMPLING"
	// ProductCWSDD is the cloud workload security product managed by datadog employees
	ProductCWSDD = "CWS_DD"
	// ProductFeatures is a pseudo-product that lists whether or not a product should be enabled in a tracer
	ProductFeatures = "FEATURES"
	// ProductLiveDebugging is the live debugging product
	ProductLiveDebugging = "LIVE_DEBUGGING"
)

var ErrNoConfigVersion = errors.New("version missing in custom file meta")

func parseConfig(product string, raw []byte, metadata Metadata) (interface{}, error) {
	var c interface{}
	var err error
	switch product {
	case ProductAPMSampling:
		c, err = parseConfigAPMSampling(raw, metadata)
	case ProductFeatures:
		c, err = parseFeaturesConfing(raw, metadata)
	case ProductLiveDebugging:
		c, err = parseLDConfig(raw, metadata)
	case ProductCWSDD:
		c, err = parseConfigCWSDD(raw, metadata)
	default:
		return nil, fmt.Errorf("unknown product - %s", product)
	}

	return c, err
}

// APMSamplingConfig is a deserialized APM Sampling configuration file
// along with its associated remote config metadata.
type APMSamplingConfig struct {
	Config   apmsampling.APMSampling
	Metadata Metadata
}

func parseConfigAPMSampling(data []byte, metadata Metadata) (APMSamplingConfig, error) {
	var apmConfig apmsampling.APMSampling
	_, err := apmConfig.UnmarshalMsg(data)
	if err != nil {
		return APMSamplingConfig{}, fmt.Errorf("could not parse apm sampling config: %v", err)
	}
	return APMSamplingConfig{
		Config:   apmConfig,
		Metadata: metadata,
	}, nil
}

func (r *Repository) APMConfigs() map[string]APMSamplingConfig {
	typedConfigs := make(map[string]APMSamplingConfig)

	configs := r.getConfigs(ProductAPMSampling)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(APMSamplingConfig)
		if !ok {
			panic("unexpected config stored as APMSamplingConfig")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}

// ConfigCWSDD is a deserialized CWS DD configuration file along with its
// associated remote config metadata
type ConfigCWSDD struct {
	Config   []byte
	Metadata Metadata
}

func parseConfigCWSDD(data []byte, metadata Metadata) (ConfigCWSDD, error) {
	return ConfigCWSDD{
		Config:   data,
		Metadata: metadata,
	}, nil
}

func (r *Repository) CWSDDConfigs() map[string]ConfigCWSDD {
	typedConfigs := make(map[string]ConfigCWSDD)

	configs := r.getConfigs(ProductCWSDD)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(ConfigCWSDD)
		if !ok {
			panic("unexpected config stored as CWSDD Config")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}

// LDConfig is a deserailized Live Debugging configuration file along with its associated
// remote config metadata.
type LDConfig struct {
	Config   []byte
	Metadata Metadata
}

func parseLDConfig(data []byte, metadata Metadata) (LDConfig, error) {
	return LDConfig{
		Config:   data,
		Metadata: metadata,
	}, nil
}

// FeaturesConfig is a deserialized configuration file that indicates what features should be enabled
// within a tracer, along with its associated remote config metadata.
type FeaturesConfig struct {
	Config   []byte
	Metadata Metadata
}

func parseFeaturesConfing(data []byte, metadata Metadata) (FeaturesConfig, error) {
	return FeaturesConfig{
		Config:   data,
		Metadata: metadata,
	}, nil
}

// Metadata stores remote config metadata for a given configuration
type Metadata struct {
	Product   string
	ID        string
	Name      string
	Version   uint64
	RawLength uint64
	Hashes    map[string][]byte
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
