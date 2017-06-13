package providers

import (
	"encoding/json"
	"fmt"
	"path"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	instancePath   string = "instances"
	checkNamePath  string = "check_names"
	initConfigPath string = "init_configs"
)

func getConnInfo() (url string, user string, password string) {
	return config.Datadog.GetString("autoconf_template_url"),
		config.Datadog.GetString("autoconf_template_username"),
		config.Datadog.GetString("autoconf_template_password")
}

// Client implements the common logic for pulling configuration template
// from a backend. This class should be embedding in the final provider class.
// The final provider class should provide the List and Get method to access
// the underlying storage.
type Client struct {
	tplDir  string
	backend Provider
}

// Init inits the fields required by the Client
func (c *Client) Init(backend Provider) {
	c.tplDir = config.Datadog.GetString("autoconf_template_dir")
	c.backend = backend
}

func (c *Client) buildStoreKey(keys ...string) string {
	keys = append([]string{c.tplDir}, keys...)
	return path.Join(keys...)
}

// Collect retrieves templates from a storage backend and builds Config objects
//  cache templates and last-modified index to avoid future full crawl if no template changed.
func (c *Client) Collect() ([]check.Config, error) {
	configs := []check.Config{}
	identifiers, err := c.getIdentifiers(c.tplDir)
	if err != nil {
		return nil, err
	}

	for _, id := range identifiers {
		templates := c.getTemplates(id)
		configs = append(configs, templates...)
	}
	return configs, nil
}

// getIdentifiers gets folders at the root of the TemplateDir
// verifies they have the right content to be a valid template
// and return their names.
func (c *Client) getIdentifiers(key string) ([]string, error) {
	identifiers := []string{}
	children, err := c.backend.List(key)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		nodes, err := c.backend.ListName(child)
		if err != nil {
			log.Warnf("could not list keys in '%s': %s", child, err)
			continue
		} else if len(nodes) < 3 {
			continue
		}

		neededTpl := map[string]bool{instancePath: false, checkNamePath: false, initConfigPath: false}
		for _, tplKey := range nodes {
			if _, ok := neededTpl[tplKey]; ok {
				neededTpl[tplKey] = true
			}
		}
		if neededTpl[instancePath] && neededTpl[checkNamePath] && neededTpl[initConfigPath] {
			identifiers = append(identifiers, child)
		}
	}
	return identifiers, nil
}

// getTemplates takes a path and returns a slice of templates if it finds
// sufficient data under this path to build one.
func (c *Client) getTemplates(key string) []check.Config {
	templates := []check.Config{}

	checkNameKey := path.Join(key, checkNamePath)
	initKey := path.Join(key, initConfigPath)
	instanceKey := path.Join(key, instancePath)

	checkNames, err := c.getCheckNames(checkNameKey)
	if err != nil {
		log.Errorf("Failed to retrieve check names at %s. Error: %s", checkNameKey, err)
		return nil
	}

	initConfigs, err := c.getJSONValue(initKey)
	if err != nil {
		log.Errorf("Failed to retrieve init configs at %s. Error: %s", initKey, err)
		return nil
	}

	instances, err := c.getJSONValue(instanceKey)
	if err != nil {
		log.Errorf("Failed to retrieve instances at %s. Error: %s", instanceKey, err)
		return nil
	}

	// sanity check
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Error("Template entries don't all have the same length, not using them.")
		return nil
	}

	for idx := range checkNames {
		instance := check.ConfigData(instances[idx])

		templates = append(templates, check.Config{
			ID:         check.ID(key),
			Name:       checkNames[idx],
			InitConfig: check.ConfigData(initConfigs[idx]),
			Instances:  []check.ConfigData{instance},
		})
	}
	return templates
}

func (c *Client) getCheckNames(key string) ([]string, error) {
	rawNames, err := c.backend.Get(key)
	if err != nil {
		return nil, err
	}
	if len(rawNames) == 0 {
		return nil, fmt.Errorf("check_names is empty")
	}

	var res []string
	if err = json.Unmarshal(rawNames, &res); err != nil {
		return nil, err
	}

	return res, nil
}

func (c *Client) getJSONValue(key string) ([]check.ConfigData, error) {
	rawValue, err := c.backend.Get(key)
	if err != nil {
		return nil, err
	}

	if len(rawValue) == 0 {
		return nil, fmt.Errorf("Value at %s is empty", key)
	}

	var rawRes interface{}
	var result []check.ConfigData

	err = json.Unmarshal(rawValue, &rawRes)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal JSON at key '%s'. Error: %s", key, err)
	}

	for _, r := range rawRes.([]interface{}) {
		switch r.(type) {
		case map[string]interface{}:
			init, _ := json.Marshal(r)
			result = append(result, init)
		default:
			return nil, fmt.Errorf("found non JSON object type at key '%s', value is: '%s'", key, r)
		}

	}
	return result, nil
}
