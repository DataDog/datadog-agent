// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build etcd

package providers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"go.etcd.io/etcd/client/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type etcdBackend interface {
	Get(ctx context.Context, key string, opts *client.GetOptions) (*client.Response, error)
}

// EtcdConfigProvider implements the Config Provider interface
// It should be called periodically and returns templates from etcd for AutoConf.
type EtcdConfigProvider struct {
	Client      etcdBackend
	templateDir string
	cache       *providerCache
}

// NewEtcdConfigProvider creates a client connection to etcd and create a new EtcdConfigProvider
func NewEtcdConfigProvider(providerConfig *pkgconfigsetup.ConfigurationProviders, _ *telemetry.Store) (ConfigProvider, error) {
	if providerConfig == nil {
		providerConfig = &pkgconfigsetup.ConfigurationProviders{}
	}

	clientCfg := client.Config{
		Endpoints:               []string{providerConfig.TemplateURL},
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}
	if len(providerConfig.Username) > 0 && len(providerConfig.Password) > 0 {
		log.Info("Using provided etcd credentials: username ", providerConfig.Username)
		clientCfg.Username = providerConfig.Username
		clientCfg.Password = providerConfig.Password
	}

	cl, err := client.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to instantiate the etcd client: %s", err)
	}
	cache := newProviderCache()
	c := client.NewKeysAPI(cl)
	return &EtcdConfigProvider{Client: c, templateDir: providerConfig.TemplateDir, cache: cache}, nil
}

// Collect retrieves templates from etcd, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (p *EtcdConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	configs := make([]integration.Config, 0)
	identifiers := p.getIdentifiers(ctx, p.templateDir)
	for _, id := range identifiers {
		templates := p.getTemplates(ctx, id)

		for idx := range templates {
			templates[idx].Source = "etcd:" + id
		}

		configs = append(configs, templates...)
	}
	return configs, nil
}

// getIdentifiers gets folders at the root of the TemplateDir
// verifies they have the right content to be a valid template
// and return their names.
func (p *EtcdConfigProvider) getIdentifiers(ctx context.Context, key string) []string {
	identifiers := make([]string, 0)
	resp, err := p.Client.Get(ctx, key, &client.GetOptions{Recursive: true})
	if err != nil {
		log.Error("Can't get templates keys from etcd: ", err)
		return identifiers
	}
	children := resp.Node.Nodes
	for _, node := range children {
		if node.Dir && hasTemplateFields(node.Nodes) {
			split := strings.Split(node.Key, "/")
			identifiers = append(identifiers, split[len(split)-1])
		}
	}
	return identifiers
}

// getTemplates takes a path and returns a slice of templates if it finds
// sufficient data under this path to build one.
func (p *EtcdConfigProvider) getTemplates(ctx context.Context, key string) []integration.Config {
	checkNameKey := buildStoreKey(key, checkNamePath)
	initKey := buildStoreKey(key, initConfigPath)
	instanceKey := buildStoreKey(key, instancePath)

	checkNames, err := p.getCheckNames(ctx, checkNameKey)
	if err != nil {
		log.Errorf("Failed to retrieve check names at %s. Error: %s", checkNameKey, err)
		return nil
	}

	initConfigs, err := p.getJSONValue(ctx, initKey)
	if err != nil {
		log.Errorf("Failed to retrieve init configs at %s. Error: %s", initKey, err)
		return nil
	}

	instances, err := p.getJSONValue(ctx, instanceKey)
	if err != nil {
		log.Errorf("Failed to retrieve instances at %s. Error: %s", instanceKey, err)
		return nil
	}

	return utils.BuildTemplates(key, checkNames, initConfigs, instances, false, "")
}

// getEtcdValue retrieves content from etcd
func (p *EtcdConfigProvider) getEtcdValue(ctx context.Context, key string) (string, error) {
	resp, err := p.Client.Get(ctx, key, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve %s from etcd: %s", key, err)
	}

	return resp.Node.Value, nil
}

func (p *EtcdConfigProvider) getCheckNames(ctx context.Context, key string) ([]string, error) {
	rawNames, err := p.getEtcdValue(ctx, key)
	if err != nil {
		err := fmt.Errorf("Couldn't get check names from etcd: %s", err)
		return nil, err
	}

	return utils.ParseCheckNames(rawNames)
}

func (p *EtcdConfigProvider) getJSONValue(ctx context.Context, key string) ([][]integration.Data, error) {
	rawValue, err := p.getEtcdValue(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get key %s from etcd: %s", key, err)
	}

	return utils.ParseJSONValue(rawValue)
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to ETCD's data.
func (p *EtcdConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {

	adListUpdated := false
	dateIdx := p.cache.mostRecentMod

	resp, err := p.Client.Get(ctx, p.templateDir, &client.GetOptions{Recursive: true})
	if err != nil {
		return false, err
	}
	identifiers := resp.Node.Nodes

	// When a node is deleted the Modified time of the children processed isn't changed.
	if p.cache.count != len(identifiers) {
		if p.cache.count != 0 {
			log.Debugf("List of AD Template was modified, updating cache.")
			adListUpdated = true
		}
		log.Debugf("Initializing cache for %v", p.String())
		p.cache.count = len(identifiers)
	}

	for _, identifier := range identifiers {
		if len(identifier.Nodes) != 3 {
			log.Infof("%v does not have a correct format to be considered in the cache", identifier.Key)
			continue
		}
		for _, tplkey := range identifier.Nodes {
			dateIdx = math.Max(float64(tplkey.ModifiedIndex), dateIdx)
		}
	}
	if dateIdx > p.cache.mostRecentMod || adListUpdated {
		log.Debugf("Idx was %v and is now %v", p.cache.mostRecentMod, dateIdx)
		p.cache.mostRecentMod = dateIdx
		log.Infof("cache updated for %v", p.String())
		return false, nil
	}
	log.Infof("cache up to date for %v", p.String())
	return true, nil
}

// String returns a string representation of the EtcdConfigProvider
func (p *EtcdConfigProvider) String() string {
	return names.Etcd
}

// hasTemplateFields verifies that a node array contains
// the needed information to build a config template
func hasTemplateFields(nodes client.Nodes) bool {
	tplKeys := []string{instancePath, checkNamePath, initConfigPath}
	if len(nodes) < 3 {
		return false
	}

	for _, tpl := range tplKeys {
		has := false
		for _, k := range nodes {
			split := strings.Split(k.Key, "/")
			if split[len(split)-1] == tpl {
				has = true
			}
		}
		if !has {
			return false
		}
	}
	return true
}

// GetConfigErrors is not implemented for the EtcdConfigProvider
func (p *EtcdConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
