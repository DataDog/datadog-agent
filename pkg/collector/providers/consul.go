// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build consul

package providers

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	log "github.com/cihub/seelog"
	consul "github.com/hashicorp/consul/api"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Abstractions for testing
type consulKVBackend interface {
	Keys(prefix, separator string, q *consul.QueryOptions) ([]string, *consul.QueryMeta, error)
	Get(key string, q *consul.QueryOptions) (*consul.KVPair, *consul.QueryMeta, error)
	Txn(txn consul.KVTxnOps, q *consul.QueryOptions) (bool, *consul.KVTxnResponse, *consul.QueryMeta, error)
}

type consulBackend interface {
	KV() consulKVBackend
}

type consulWrapper struct {
	client *consul.Client
}

func (c *consulWrapper) KV() consulKVBackend {
	return c.client.KV()
}

// ConsulConfigProvider implements the Config Provider interface
// It should be called periodically and returns templates from consul for AutoConf.
type ConsulConfigProvider struct {
	Client      consulBackend
	Cache       map[string][]check.Config
	cacheIdx    map[string]ADEntryIndex
	TemplateDir string
}

// NewConsulConfigProvider creates a client connection to consul and create a new ConsulConfigProvider
func NewConsulConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	consulURL, err := url.Parse(config.TemplateURL)
	if err != nil {
		return nil, err
	}

	clientCfg := consul.DefaultConfig()
	clientCfg.Address = consulURL.Host
	clientCfg.Address = consulURL.Scheme
	clientCfg.Token = config.Token

	if consulURL.Scheme == "https" {
		clientCfg.TLSConfig = consul.TLSConfig{
			Address:            consulURL.Host,
			CAFile:             config.CAFile,
			CAPath:             config.CAPath,
			CertFile:           config.CertFile,
			KeyFile:            config.KeyFile,
			InsecureSkipVerify: false,
		}
	}

	if len(config.Username) > 0 && len(config.Password) > 0 {
		log.Infof("Using provided consul credentials (username): %s", config.Username)
		auth := &consul.HttpBasicAuth{
			Username: config.Username,
			Password: config.Password,
		}
		clientCfg.HttpAuth = auth
	}

	cli, err := consul.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to instantiate the consul client: %s", err)
	}

	c := &consulWrapper{
		client: cli,
	}

	return &ConsulConfigProvider{
		Client:      c,
		TemplateDir: config.TemplateDir,
		Cache:       make(map[string][]check.Config),
		cacheIdx:    make(map[string]ADEntryIndex),
	}, nil

}

// Collect retrieves templates from consul, builds Config objects and returns them
func (p *ConsulConfigProvider) Collect() ([]check.Config, error) {
	var templates []check.Config

	configs := make([]check.Config, 0)
	identifiers := p.getIdentifiers(p.TemplateDir)
	log.Debugf("identifiers found in backend: %v\n", identifiers)
	for _, id := range identifiers {
		current, err := p.isIndexCurrent(id)
		if err != nil {
			return configs, err
		}

		if err == nil && current {
			templates = p.Cache[id]
		} else {
			var index *ADEntryIndex
			templates, index = p.getTemplates(id)
			p.Cache[id] = templates
			p.cacheIdx[id] = *index
		}

		configs = append(configs, templates...)
	}
	return configs, nil
}

// getIdentifiers gets folders at the root of the TemplateDir
// verifies they have the right content to be a valid template
// and return their names.
func (p *ConsulConfigProvider) getIdentifiers(prefix string) []string {
	kv := p.Client.KV()

	identifiers := make([]string, 0)
	// TODO: decide on the query parameters.
	keys, _, err := kv.Keys(prefix, "", nil)
	if err != nil {
		log.Error("Can't get templates keys from consul: ", err)
		return identifiers
	}

	criteriaFound := make(map[string]int)
	for _, key := range keys {
		// not sure how we should go about this...
		splits := strings.SplitAfter(key, prefix)
		if len(splits) != 2 {
			continue
		}

		postfix := splits[1]
		if postfix[0] == '/' {
			postfix = strings.TrimLeft(postfix, "/")
		}

		dissect := strings.Split(postfix, "/")
		if len(dissect) != 2 {
			continue
		}

		if isTemplateField(dissect[1]) {
			// same key can't show up twice, so just add blindly
			criteriaFound[dissect[0]]++
		}
	}

	for identifier, met := range criteriaFound {
		if met == 3 {
			identifiers = append(identifiers, identifier)
		}
	}

	// this doesn't trigger often and list should be small
	sort.Strings(identifiers)
	return identifiers
}

// getTemplates takes a path and returns a slice of templates if it finds
// sufficient data under this path to build one.
func (p *ConsulConfigProvider) getTemplates(key string) ([]check.Config, *ADEntryIndex) {
	templates := make([]check.Config, 0)

	checkNameKey := buildStoreKey(key, checkNamePath)
	initKey := buildStoreKey(key, initConfigPath)
	instanceKey := buildStoreKey(key, instancePath)

	checkNames, namesIdx, err := p.getCheckNames(checkNameKey)
	if err != nil {
		log.Errorf("Failed to retrieve check names at %s. Error: %s", checkNameKey, err)
		return templates, nil
	}

	initConfigs, initIdx, err := p.getJSONValue(initKey)
	if err != nil {
		log.Errorf("Failed to retrieve init configs at %s. Error: %s", initKey, err)
		return templates, nil
	}

	instances, instancesIdx, err := p.getJSONValue(instanceKey)
	if err != nil {
		log.Errorf("Failed to retrieve instances at %s. Error: %s", instanceKey, err)
		return templates, nil
	}

	// sanity check
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Error("Template entries don't all have the same length in consul, not using them.")
		return templates, nil
	}

	for idx := range checkNames {
		instance := check.ConfigData(instances[idx])

		templates = append(templates, check.Config{
			Name:          checkNames[idx],
			InitConfig:    check.ConfigData(initConfigs[idx]),
			Instances:     []check.ConfigData{instance},
			ADIdentifiers: []string{key},
		})
	}
	index := &ADEntryIndex{
		NamesIdx:     namesIdx,
		InitIdx:      initIdx,
		InstancesIdx: instancesIdx,
	}
	return templates, index
}

// getValue returns value, idx, error
func (p *ConsulConfigProvider) getValue(key string) ([]byte, uint64, error) {
	kv := p.Client.KV()
	pair, _, err := kv.Get(key, nil)
	if err != nil || pair == nil {
		return nil, 0, err
	}

	return pair.Value, pair.ModifyIndex, err
}

// isIndexCurrent returns if the index for a key has changed since last checked.
func (p *ConsulConfigProvider) isIndexCurrent(key string) (bool, error) {
	checkNameKey := path.Join(p.TemplateDir, key, checkNamePath)
	initKey := path.Join(p.TemplateDir, key, initConfigPath)
	instanceKey := path.Join(p.TemplateDir, key, instancePath)

	idx, ok := p.cacheIdx[key]

	// I think these actually pull the the KV pairs :(
	ops := consul.KVTxnOps{
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   checkNameKey,
			Index: idx.NamesIdx,
		},
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   initKey,
			Index: idx.InitIdx,
		},
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   instanceKey,
			Index: idx.InstancesIdx,
		},
	}

	kv := p.Client.KV()
	ok, response, _, err := kv.Txn(ops, nil)
	if !ok || err != nil {
		return false, err
	}
	if len(response.Errors) > 0 {
		return false, nil
	}

	return true, nil
}

func (p *ConsulConfigProvider) getCheckNames(key string) ([]string, uint64, error) {
	raw, idx, err := p.getValue(key)
	if err != nil {
		err := fmt.Errorf("Couldn't get check names from consul: %s", err)
		return nil, 0, err
	}

	names := string(raw)
	if names == "" {
		err = fmt.Errorf("check_names is empty")
		return nil, 0, err
	}

	checks, err := parseCheckNames(names)
	return checks, idx, err
}

func (p *ConsulConfigProvider) getJSONValue(key string) ([]check.ConfigData, uint64, error) {
	rawValue, idx, err := p.getValue(key)
	if err != nil {
		err := fmt.Errorf("Couldn't get key %s from consul: %s", key, err)
		return nil, 0, err
	}

	r, err := parseJSONValue(string(rawValue))

	return r, idx, err
}

// isTemplateField verifies the key
// the needed information to build a config template
func isTemplateField(key string) bool {
	tplKeys := []string{instancePath, checkNamePath, initConfigPath}

	for _, tpl := range tplKeys {
		if key == tpl {
			return true
		}
	}
	return false
}

func init() {
	RegisterProvider("consul", NewConsulConfigProvider)
}
