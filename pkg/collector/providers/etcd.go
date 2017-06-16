package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/coreos/etcd/client"
	"github.com/ghodss/yaml"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	instancePath   string = "instances"
	checkNamePath  string = "check_names"
	initConfigPath string = "init_configs"
)

// EtcdConfigProvider implements the Config Provider interface
// It should be called periodically and returns templates from etcd for AutoConf.
type EtcdConfigProvider struct {
	Client      client.KeysAPI
	TemplateDir string
}

// NewEtcdConfigProvider creates a client connection to etcd and create a new EtcdConfigProvider
func NewEtcdConfigProvider() (*EtcdConfigProvider, error) {
	tplDir := config.Datadog.GetString("autoconf_template_dir")
	tplURL := config.Datadog.GetString("autoconf_template_url")

	clientCfg := client.Config{
		Endpoints:               []string{tplURL},
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}

	etcdUsername := config.Datadog.GetString("autoconf_template_username")
	etcdPassword := config.Datadog.GetString("autoconf_template_password")

	if len(etcdUsername) > 0 && len(etcdPassword) > 0 {
		log.Info("Using provided etcd credentials: username ", etcdUsername)
		clientCfg.Username = etcdUsername
		clientCfg.Password = etcdPassword
	}

	cl, err := client.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to instantiate the etcd client: %s", err)
	}

	c := client.NewKeysAPI(cl)
	return &EtcdConfigProvider{c, tplDir}, nil
}

// Collect retrieves templates from etcd, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (p *EtcdConfigProvider) Collect() ([]check.Config, error) {
	configs := make([]check.Config, 0)
	identifiers := p.getIdentifiers(p.TemplateDir)
	for _, id := range identifiers {
		templates := p.getTemplates(id)

		for _, template := range templates {
			c := check.Config{
				ID:         check.ID(id),
				Name:       template.Name,
				InitConfig: template.InitConfig,
				Instances:  template.Instances,
			}
			configs = append(configs, c)
		}
	}
	return configs, nil
}

// getIdentifiers gets folders at the root of the TemplateDir
// verifies they have the right content to be a valid template
// and return their names.
func (p *EtcdConfigProvider) getIdentifiers(key string) []string {
	identifiers := make([]string, 0)
	resp, err := p.Client.Get(context.Background(), key, &client.GetOptions{Recursive: true})
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
func (p *EtcdConfigProvider) getTemplates(key string) []check.Config {
	templates := make([]check.Config, 0)

	checkNameKey := buildStoreKey(key, checkNamePath)
	initKey := buildStoreKey(key, initConfigPath)
	instanceKey := buildStoreKey(key, instancePath)

	checkNames, err := p.getCheckNames(checkNameKey)
	if err != nil {
		log.Errorf("Failed to retrieve check names at %s. Error: %s", checkNameKey, err)
		return nil
	}

	initConfigs, err := p.getJSONValue(initKey)
	if err != nil {
		log.Errorf("Failed to retrieve init configs at %s. Error: %s", initKey, err)
		return nil
	}

	instances, err := p.getJSONValue(instanceKey)
	if err != nil {
		log.Errorf("Failed to retrieve instances at %s. Error: %s", instanceKey, err)
		return nil
	}

	// sanity check
	if len(checkNames) != len(initConfigs) || len(checkNames) != len(instances) {
		log.Error("Template entries don't all have the same length in etcd, not using them.")
		return templates
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

// getEtcdValue retrieves content from etcd
func (p *EtcdConfigProvider) getEtcdValue(key string) (string, error) {

	resp, err := p.Client.Get(context.Background(), key, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve %s from etcd: %s", key, err)
	}

	return resp.Node.Value, nil
}

func (p *EtcdConfigProvider) getCheckNames(key string) ([]string, error) {
	rawNames, err := p.getEtcdValue(key)
	if err != nil {
		err := fmt.Errorf("Couldn't get check names from etcd: %s", err)
		return nil, err
	}
	if rawNames == "" {
		err = fmt.Errorf("check_names is empty")
		return nil, err
	}

	var res []string

	if err = json.Unmarshal([]byte(rawNames), &res); err != nil {
		return nil, err
	}

	return res, nil
}

func (p *EtcdConfigProvider) getJSONValue(key string) ([]check.ConfigData, error) {
	rawValue, err := p.getEtcdValue(key)
	if err != nil {
		err := fmt.Errorf("Couldn't get key %s from etcd: %s", key, err)
		return nil, err
	}

	if rawValue == "" {
		err = fmt.Errorf("Value at %s is empty in etcd", key)
		return nil, err
	}

	yamlValue, err := yaml.JSONToYAML([]byte(rawValue))
	if err != nil {
		err := fmt.Errorf("Couldn't decode JSON value at %s. Error: %s", key, err)
		return nil, err
	}

	var rawRes interface{}
	var result []check.ConfigData

	err = yaml.Unmarshal(yamlValue, &rawRes)
	if err != nil {
		err := fmt.Errorf("Failed to unmarshal value at %s. Error: %s", key, err)
		return nil, err
	}

	for _, r := range rawRes.([]interface{}) {
		switch r.(type) {
		case []byte:
			result = append(result, r.([]byte))
		case map[string]interface{}:
			init, _ := yaml.Marshal(r)
			result = append(result, init)
		}

	}
	return result, nil
}

// getIdx gets the last-modified index of a key
// it's useful if you want to verify something changed
// before triggering a full read
func (p *EtcdConfigProvider) getIdx(key string) int {
	// TODO
	return 0
}

func (p *EtcdConfigProvider) String() string {
	return "etcd Configuration Provider"
}

func buildStoreKey(key ...string) string {
	parts := []string{config.Datadog.GetString("autoconf_template_dir")}
	parts = append(parts, key...)
	return path.Join(parts...)
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
