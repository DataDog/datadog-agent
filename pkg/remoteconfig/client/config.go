// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrNoConfigVersion is returned when a config is missing its version in its custom meta
	ErrNoConfigVersion = errors.New("config has no version in its meta")
)

type errUnknownProduct struct {
	product string
}

func (e *errUnknownProduct) Error() string {
	return fmt.Sprintf("unknown product %s", e.product)
}

type configCustom struct {
	Version *uint64  `json:"v"`
	Clients []string `json:"c"`
	Expire  int64    `json:"e"`
}

func parseConfigCustom(rawCustom []byte) (configCustom, error) {
	var custom configCustom
	err := json.Unmarshal(rawCustom, &custom)
	if err != nil {
		return configCustom{}, err
	}
	if custom.Version == nil {
		return configCustom{}, ErrNoConfigVersion
	}
	return custom, nil
}

type configMeta struct {
	hash   [32]byte
	custom configCustom
	path   configPath
}

func (c *configMeta) expired(time int64) bool {
	return c.custom.Expire != 0 && time > c.custom.Expire
}

func (c *configMeta) scopedToClient(clientID string) bool {
	if len(c.custom.Clients) == 0 {
		return true
	}
	for _, client := range c.custom.Clients {
		if clientID == client {
			return true
		}
	}
	return false
}

func configMetaHash(path string, custom []byte) [32]byte {
	b := bytes.Buffer{}
	b.WriteString(path)
	metaHash := sha256.Sum256(custom)
	b.Write(metaHash[:])
	return sha256.Sum256(b.Bytes())
}

func parseConfigMeta(path string, custom []byte) (configMeta, error) {
	configPath, err := parseConfigPath(path)
	if err != nil {
		return configMeta{}, fmt.Errorf("could not parse config path: %v", err)
	}
	configCustom, err := parseConfigCustom(custom)
	if err != nil {
		return configMeta{}, fmt.Errorf("could not parse config meta: %v", err)
	}
	return configMeta{
		custom: configCustom,
		path:   configPath,
		hash:   configMetaHash(path, custom),
	}, nil
}

type config struct {
	hash     [32]byte
	meta     configMeta
	contents []byte
}

func configHash(meta configMeta, contents []byte) [32]byte {
	b := bytes.Buffer{}
	b.Write(meta.hash[:])
	configHash := sha256.Sum256(contents)
	b.Write(configHash[:])
	return sha256.Sum256(b.Bytes())
}

// configList is a config list
type configList struct {
	configs    map[string]map[string]struct{}
	apmConfigs []ConfigAPMSamling
}

func newConfigList() *configList {
	return &configList{
		configs: make(map[string]map[string]struct{}),
	}
}

func (cl *configList) addConfig(c config) error {
	if _, productExists := cl.configs[c.meta.path.Product]; !productExists {
		cl.configs[c.meta.path.Product] = make(map[string]struct{})
	}
	if _, exist := cl.configs[c.meta.path.Product][c.meta.path.ConfigID]; exist {
		return fmt.Errorf("duplicated config id: %s", c.meta.path.ConfigID)
	}
	switch c.meta.path.Product {
	case ProductAPMSampling:
		pc, err := parseConfigAPMSampling(c)
		if err != nil {
			return err
		}
		cl.apmConfigs = append(cl.apmConfigs, pc)
	default:
		return &errUnknownProduct{product: c.meta.path.Product}
	}
	cl.configs[c.meta.path.Product][c.meta.path.ConfigID] = struct{}{}
	return nil
}

func (cl *configList) getCurrentConfigs(clientID string, time int64) Configs {
	apmSamplingHash := bytes.Buffer{}
	configs := Configs{
		APMSamplingConfigs: make([]ConfigAPMSamling, 0, len(cl.apmConfigs)),
	}
	for _, pc := range cl.apmConfigs {
		if !pc.c.meta.expired(time) && pc.c.meta.scopedToClient(clientID) {
			configs.APMSamplingConfigs = append(configs.APMSamplingConfigs, pc)
			apmSamplingHash.Write(pc.c.hash[:])
		}
	}
	configs.apmSamplingConfigsHash = sha256.Sum256(apmSamplingHash.Bytes())
	return configs
}

// Configs is a list of configs
type Configs struct {
	APMSamplingConfigs     []ConfigAPMSamling
	apmSamplingConfigsHash [32]byte
}

// ConfigsUpdated contains the info about which config got updated
type ConfigsUpdated struct {
	APMSampling bool
}

// Diff compares two config lists and returns which configs got updated
func (c *Configs) Diff(oldConfigs Configs) ConfigsUpdated {
	return ConfigsUpdated{
		APMSampling: c.apmSamplingConfigsHash != oldConfigs.apmSamplingConfigsHash,
	}
}
