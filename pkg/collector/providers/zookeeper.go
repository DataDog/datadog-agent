// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build zk

package providers

import (
	"fmt"
	"path"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/samuel/go-zookeeper/zk"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
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
	cache       *CacheProvider
}

// NewZookeeperConfigProvider returns a new Client connected to a Zookeeper backend.
func NewZookeeperConfigProvider(cfg config.ConfigurationProviders) (ConfigProvider, error) {
	urls := strings.Split(cfg.TemplateURL, ",")

	c, _, err := zk.Connect(urls, sessionTimeout)
	if err != nil {
		return nil, fmt.Errorf("ZookeeperConfigProvider: couldn't connect to %q (%s): %s", cfg.TemplateURL, strings.Join(urls, ", "), err)
	}
	cacheProvider := NewCPCache()
	return &ZookeeperConfigProvider{
		client:      c,
		templateDir: cfg.TemplateDir,
		cache:       cacheProvider,
	}, nil
}

func (z *ZookeeperConfigProvider) String() string {
	return "zookeeper Configuration Provider"
}

// Collect retrieves templates from Zookeeper, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (z *ZookeeperConfigProvider) Collect() ([]check.Config, error) {
	configs := make([]check.Config, 0)
	identifiers, err := z.getIdentifiers(z.templateDir)
	if err != nil {
		return nil, err
	}
	log.Debug("the identifiers are", identifiers)
	for _, id := range identifiers {
		c := z.getTemplates(id)
		configs = append(configs, c...)
	}
	return configs, nil
}

// Updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Zookeeper's data.
func (z *ZookeeperConfigProvider) IsUpToDate() (bool, error) {

	updates := 0
	var adTempAdded bool

	identifiers, err := z.getIdentifiers(z.templateDir)
	if err != nil {
		return false, nil
	}

	// We want to be specifically notified if a template was added or removed. Then we flush the cache.
	if len(identifiers) != len(z.cache.Adids2Node){
		log.Infof("list of ADTemplates was modified. Cache is being updated...")
		z.cache.Adids2Node = make(map[string]AdIdentfier2stats)
		adTempAdded = true
	}

	for _, identifier := range identifiers {
		// This supposes that we keep the /datadog/check_config/ad_identifiers_id/{check_names|init_configs|instances} format.
		gChildren, _, err := z.client.Children(identifier)
		if err != nil {
			return false, fmt.Errorf("couldn't get key '%s' from zookeeper: %s", identifier, err)
		}
		newStats := AdIdentfier2stats{Stats: make(map[string]int32)}
		for _, gcn := range gChildren {
			gcnPath := path.Join(identifier, gcn)
			_, stat, err := z.client.Get(gcnPath)
			if err != nil {
				return false, fmt.Errorf("couldn't get key '%s' from zookeeper: %s", identifier, err)
			}
			// Here we get the Version as opposed to the cVersion as we process the last child
			newStats.Stats[gcn] = stat.Version //["check_config":1]
		}
		if len(z.cache.Adids2Node) == 0 {
			log.Infof("Populating cache for %v.", z.String())
			z.cache.Adids2Node[identifier] = newStats
			continue
		}

		value, ok := z.cache.Adids2Node[identifier]

		// if the template is not in the cache, add its up to date version.
		if !ok {
			updates++
			z.cache.Adids2Node[identifier] = newStats
			continue
		}

		// check if the version of the template in the cache is outdated.
		equal, err := sameSlice(newStats.Stats, value.Stats)

		if err != nil {
			updates++
			log.Infof("cache update failed for %v because: ", identifier, err)
			continue
		}
		if !equal {
			updates++
			z.cache.Adids2Node[identifier] = newStats
		}
		log.Infof("cache up to date for %v", identifier)
	}
	if adTempAdded || updates > 0{
		return false, nil
	}
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
func (z *ZookeeperConfigProvider) getTemplates(key string) []check.Config {
	checkNameKey := path.Join(key, checkNamePath)
	initKey := path.Join(key, initConfigPath)
	instanceKey := path.Join(key, instancePath)

	rawNames, _, err := z.client.Get(checkNameKey)
	if err != nil {
		log.Errorf("Couldn't get check names from key '%s' in zookeeper: %s", key, err)
		return nil
	}

	checkNames, err := parseCheckNames(string(rawNames))
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

	return buildTemplates(key, checkNames, initConfigs, instances)
}

func (z *ZookeeperConfigProvider) getJSONValue(key string) ([]check.ConfigData, error) {
	rawValue, _, err := z.client.Get(key)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get key '%s' from zookeeper: %s", key, err)
	}

	return parseJSONValue(string(rawValue))
}

func init() {
	RegisterProvider("zookeeper", NewZookeeperConfigProvider)
}
