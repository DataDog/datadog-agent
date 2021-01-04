// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build consul

package providers

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	consul "github.com/hashicorp/consul/api"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Abstractions for testing
type consulKVBackend interface {
	Keys(prefix, separator string, q *consul.QueryOptions) ([]string, *consul.QueryMeta, error)
	Get(key string, q *consul.QueryOptions) (*consul.KVPair, *consul.QueryMeta, error)
	List(prefix string, q *consul.QueryOptions) (consul.KVPairs, *consul.QueryMeta, error)
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
	TemplateDir string
	cache       *ProviderCache
}

// NewConsulConfigProvider creates a client connection to consul and create a new ConsulConfigProvider
func NewConsulConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	consulURL, err := url.Parse(config.TemplateURL)
	if err != nil {
		return nil, err
	}

	clientCfg := consul.DefaultConfig()
	clientCfg.Address = consulURL.Host
	clientCfg.Scheme = consulURL.Scheme
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
	cache := NewCPCache()
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
		cache:       cache,
	}, nil

}

// String returns a string representation of the ConsulConfigProvider
func (p *ConsulConfigProvider) String() string {
	return names.Consul
}

// Collect retrieves templates from consul, builds Config objects and returns them
func (p *ConsulConfigProvider) Collect() ([]integration.Config, error) {
	configs := make([]integration.Config, 0)
	identifiers := p.getIdentifiers(p.TemplateDir)
	log.Debugf("identifiers found in backend: %v", identifiers)
	for _, id := range identifiers {
		templates := p.getTemplates(id)

		for idx := range templates {
			templates[idx].Source = "consul:" + id
		}

		configs = append(configs, templates...)
	}
	return configs, nil
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Consul's data.
func (p *ConsulConfigProvider) IsUpToDate() (bool, error) {
	kv := p.Client.KV()
	adListUpdated := false
	dateIdx := p.cache.LatestTemplateIdx

	identifiers, _, err := kv.List(p.TemplateDir, nil)
	if err != nil {
		return false, err
	}
	if p.cache.NumAdTemplates != len(identifiers) {
		if p.cache.NumAdTemplates == 0 {
			log.Infof("Initializing cache for %v", p.String())
		}
		log.Debugf("List of AD Template was modified, updating cache.")
		p.cache.NumAdTemplates = len(identifiers)
		adListUpdated = true
	}

	for _, identifier := range identifiers {
		dateIdx = math.Max(float64(identifier.ModifyIndex), dateIdx)
	}
	if dateIdx > p.cache.LatestTemplateIdx || adListUpdated {
		log.Debugf("Cache Index was %v and is now %v", p.cache.LatestTemplateIdx, dateIdx)
		p.cache.LatestTemplateIdx = dateIdx
		log.Infof("Cache updated for %v", p.String())
		return false, nil
	}
	return true, nil
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
func (p *ConsulConfigProvider) getTemplates(key string) []integration.Config {
	templates := make([]integration.Config, 0)

	checkNameKey := buildStoreKey(key, checkNamePath)
	initKey := buildStoreKey(key, initConfigPath)
	instanceKey := buildStoreKey(key, instancePath)

	checkNames, err := p.getCheckNames(checkNameKey)
	if err != nil {
		log.Errorf("Failed to retrieve check names at %s. Error: %s", checkNameKey, err)
		return templates
	}

	initConfigs, err := p.getJSONValue(initKey)
	if err != nil {
		log.Errorf("Failed to retrieve init configs at %s. Error: %s", initKey, err)
		return templates
	}

	instances, err := p.getJSONValue(instanceKey)
	if err != nil {
		log.Errorf("Failed to retrieve instances at %s. Error: %s", instanceKey, err)
		return templates
	}
	return buildTemplates(key, checkNames, initConfigs, instances)
}

// getValue returns value, error
func (p *ConsulConfigProvider) getValue(key string) ([]byte, error) {
	kv := p.Client.KV()
	pair, _, err := kv.Get(key, nil)
	if err != nil || pair == nil {
		return nil, err
	}

	return pair.Value, err
}

func (p *ConsulConfigProvider) getCheckNames(key string) ([]string, error) {
	raw, err := p.getValue(key)
	if err != nil {
		err := fmt.Errorf("couldn't get check names from consul: %s", err)
		return nil, err
	}

	names := string(raw)
	if names == "" {
		err = fmt.Errorf("check_names is empty")
		return nil, err
	}

	checks, err := parseCheckNames(names)
	return checks, err
}

func (p *ConsulConfigProvider) getJSONValue(key string) ([][]integration.Data, error) {
	rawValue, err := p.getValue(key)
	if err != nil {
		err := fmt.Errorf("Couldn't get key %s from consul: %s", key, err)
		return nil, err
	}

	r, err := parseJSONValue(string(rawValue))

	return r, err
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
