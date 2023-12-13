// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zk

package providers

import (
	"context"
	"fmt"
	"math"
	"path"
	"strings"
	"time"

	"github.com/samuel/go-zookeeper/zk"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const sessionTimeout = 1 * time.Second

type zkBackend interface {
	Get(key string) ([]byte, *zk.Stat, error)
	Children(key string) ([]string, *zk.Stat, error)
}

// ZookeeperConfigProvider implements the Config Provider interface It should
// be called periodically and returns templates from Zookeeper for AutoConf.
type ZookeeperConfigProvider struct {
	client      zkBackend
	templateDir string
	cache       *providerCache
}

// NewZookeeperConfigProvider returns a new Client connected to a Zookeeper backend.
func NewZookeeperConfigProvider(providerConfig *config.ConfigurationProviders) (ConfigProvider, error) {
	if providerConfig == nil {
		providerConfig = &config.ConfigurationProviders{}
	}

	urls := strings.Split(providerConfig.TemplateURL, ",")

	c, _, err := zk.Connect(urls, sessionTimeout)
	if err != nil {
		return nil, fmt.Errorf("ZookeeperConfigProvider: couldn't connect to %q (%s): %s", providerConfig.TemplateURL, strings.Join(urls, ", "), err)
	}
	cache := newProviderCache()
	return &ZookeeperConfigProvider{
		client:      c,
		templateDir: providerConfig.TemplateDir,
		cache:       cache,
	}, nil
}

// String returns a string representation of the ZookeeperConfigProvider
func (z *ZookeeperConfigProvider) String() string {
	return names.Zookeeper
}

// Collect retrieves templates from Zookeeper, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (z *ZookeeperConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	configs := make([]integration.Config, 0)
	identifiers, err := z.getIdentifiers(z.templateDir)
	if err != nil {
		return nil, err
	}
	for _, id := range identifiers {
		c := z.getTemplates(id)

		for idx := range c {
			c[idx].Source = "zookeeper:" + id
		}

		configs = append(configs, c...)
	}
	return configs, nil
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Zookeeper's data.
func (z *ZookeeperConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {

	identifiers, err := z.getIdentifiers(z.templateDir)
	if err != nil {
		return false, err
	}
	outdated := z.cache.mostRecentMod
	adListUpdated := false

	if z.cache.count != len(identifiers) {
		if z.cache.count == 0 {
			log.Infof("Initializing cache for %v", z.String())
		}
		log.Debugf("List of AD Template was modified, updating cache.")
		adListUpdated = true
		z.cache.count = len(identifiers)
	}

	for _, identifier := range identifiers {
		gChildren, _, err := z.client.Children(identifier)

		if err != nil {
			return false, err
		}
		for _, gcn := range gChildren {
			gcnPath := path.Join(identifier, gcn)
			_, stat, err := z.client.Get(gcnPath)
			if err != nil {
				return false, fmt.Errorf("couldn't get key '%s' from zookeeper: %s", identifier, err)
			}
			outdated = math.Max(float64(stat.Mtime), outdated)
		}
	}
	if outdated > z.cache.mostRecentMod || adListUpdated {
		log.Debugf("Idx was %v and is now %v", z.cache.mostRecentMod, outdated)
		z.cache.mostRecentMod = outdated
		log.Infof("cache updated for %v", z.String())
		return false, nil
	}
	log.Infof("cache up to date for %v", z.String())
	return true, nil
}

// getIdentifiers gets folders at the root of the template dir
// verifies they have the right content to be a valid template
// and return their names.
func (z *ZookeeperConfigProvider) getIdentifiers(key string) ([]string, error) {
	identifiers := []string{}

	children, _, err := z.client.Children(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to list '%s' to get identifiers from zookeeper: %s", key, err)
	}

	for _, child := range children {
		nodePath := path.Join(key, child)
		nodes, _, err := z.client.Children(nodePath)
		if err != nil {
			log.Warnf("could not list keys in '%s': %s", nodePath, err)
			continue
		} else if len(nodes) < 3 {
			continue
		}

		foundTpl := map[string]bool{instancePath: false, checkNamePath: false, initConfigPath: false}
		for _, tplKey := range nodes {
			if _, ok := foundTpl[tplKey]; ok {
				foundTpl[tplKey] = true
			}
		}
		if foundTpl[instancePath] && foundTpl[checkNamePath] && foundTpl[initConfigPath] {
			identifiers = append(identifiers, nodePath)
		}
	}
	return identifiers, nil
}

// getTemplates takes a path and returns a slice of templates if it finds
// sufficient data under this path to build one.
func (z *ZookeeperConfigProvider) getTemplates(key string) []integration.Config {
	checkNameKey := path.Join(key, checkNamePath)
	initKey := path.Join(key, initConfigPath)
	instanceKey := path.Join(key, instancePath)

	rawNames, _, err := z.client.Get(checkNameKey)
	if err != nil {
		log.Errorf("Couldn't get check names from key '%s' in zookeeper: %s", key, err)
		return nil
	}

	checkNames, err := utils.ParseCheckNames(string(rawNames))
	if err != nil {
		log.Errorf("Failed to retrieve check names at %s. Error: %s", checkNameKey, err)
		return nil
	}

	initConfigs, err := z.getJSONValue(initKey)
	if err != nil {
		log.Errorf("Failed to retrieve init configs at %s. Error: %s", initKey, err)
		return nil
	}

	instances, err := z.getJSONValue(instanceKey)
	if err != nil {
		log.Errorf("Failed to retrieve instances at %s. Error: %s", instanceKey, err)
		return nil
	}

	return utils.BuildTemplates(key, checkNames, initConfigs, instances)
}

func (z *ZookeeperConfigProvider) getJSONValue(key string) ([][]integration.Data, error) {
	rawValue, _, err := z.client.Get(key)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get key '%s' from zookeeper: %s", key, err)
	}

	return utils.ParseJSONValue(string(rawValue))
}

func init() {
	RegisterProvider(names.ZookeeperRegisterName, NewZookeeperConfigProvider)
}

// GetConfigErrors is not implemented for the ZookeeperConfigProvider
func (z *ZookeeperConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
