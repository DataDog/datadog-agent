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

type zkBackend interface {
	Get(key string) ([]byte, *zk.Stat, error)
	Children(key string) ([]string, *zk.Stat, error)
}

// ZookeeperConfigProvider implements the Config Provider interface It should
// be called periodically and returns templates from Zookeeper for AutoConf.
type ZookeeperConfigProvider struct {
	client zkBackend
}

// NewZookeeperConfigProvider returns a new Client connected to a Zookeeper backend.
func NewZookeeperConfigProvider() (*ZookeeperConfigProvider, error) {
	tplURL := config.Datadog.GetString("autoconf_template_url")
	tplTimeout := config.Datadog.GetInt("autoconf_template_url_timeout")

	urls := strings.Split(tplURL, ",")
	c, _, err := zk.Connect(urls, time.Duration(tplTimeout)*time.Second)
	if err != nil {
		return nil, fmt.Errorf("ZookeeperConfigProvider: couldn't connect to '%s': %s", tplURL, err)
	}

	return &ZookeeperConfigProvider{client: c}, nil
}

func (z *ZookeeperConfigProvider) String() string {
	return "zookeeper Configuration Provider"
}

// Collect retrieves templates from Zookeeper, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (z *ZookeeperConfigProvider) Collect() ([]check.Config, error) {
	configs := make([]check.Config, 0)
	identifiers, err := z.getIdentifiers(config.Datadog.GetString("autoconf_template_dir"))
	if err != nil {
		return nil, err
	}

	for _, id := range identifiers {
		c := z.getTemplates(id)
		configs = append(configs, c...)
	}
	return configs, nil
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
	provider, err := NewZookeeperConfigProvider()
	if err == nil {
		RegisterProvider("zk", provider)
	}
}
