// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package integration contains the type that represents a configuration.
package integration

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/twmb/murmur3"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

const (
	// FakeConfigHash is used in unit tests
	FakeConfigHash = 1
)

// Data contains YAML code
type Data []byte

// RawMap is the generic type to hold YAML configurations
type RawMap map[interface{}]interface{}

// JSONMap is the generic type to hold JSON configurations
type JSONMap map[string]interface{}

// Config is a generic container for configuration data specific to an
// integration.  It contains snippets of configuration for various agent
// components, in fields of type Data.
//
// The Data fields contain YAML data except when config.Provider is
// names.Container or names.Kubernetes, in which case the configuration is in
// JSON.
type Config struct {
	// When a new field is added to this struct, please evaluate whether it
	// should be computed in the config Digest and update the field's
	// documentation and the Digest method accordingly

	// Name of the integration
	Name string `json:"check_name"` // (include in digest: true)

	// Instances is the list of instances in YAML or JSON.
	Instances []Data `json:"instances"` // (include in digest: true)

	// InitConfig is the init_config in YAML or JSON
	InitConfig Data `json:"init_config"` // (include in digest: true)

	// MetricConfig is the metric config in YAML or JSON (jmx check only)
	MetricConfig Data `json:"metric_config"` // (include in digest: false)

	// LogsConfig is the logs config in YAML or JSON (logs-agent only)
	LogsConfig Data `json:"logs"` // (include in digest: true)

	// ADIdentifiers is the list of AutoDiscovery identifiers for this
	// integration.  If either ADIdentifiers or AdvancedADIdentifiers are
	// present, then this config is a template and will be resolved when a
	// matching service is discovered. Otherwise, the config will be scheduled
	// immediately. (optional)
	ADIdentifiers []string `json:"ad_identifiers"` // (include in digest: true)

	// AdvancedADIdentifiers is the list of advanced AutoDiscovery identifiers;
	// see ADIdentifiers.  (optional)
	AdvancedADIdentifiers []AdvancedADIdentifier `json:"advanced_ad_identifiers"` // (include in digest: false)

	// Provider is the name of the config provider that issued the config.  If
	// this is "", then the config is a service config, representing a service
	// discovered by a listener.
	Provider string `json:"provider"` // (include in digest: false)

	// ServiceID is the ID of the service (set only for resolved templates and
	// for service configs)
	ServiceID string `json:"service_id"` // (include in digest: true)

	// TaggerEntity is the tagger entity ID
	TaggerEntity string `json:"-"` // (include in digest: false)

	// ClusterCheck is cluster-check configuration flag
	ClusterCheck bool `json:"cluster_check"` // (include in digest: false)

	// NodeName is node name in case of an endpoint check backed by a pod
	NodeName string `json:"node_name"` // (include in digest: true)

	// Source is the source of the configuration
	Source string `json:"source"` // (include in digest: false)

	// IgnoreAutodiscoveryTags is used to ignore tags coming from autodiscovery
	IgnoreAutodiscoveryTags bool `json:"ignore_autodiscovery_tags"` // (include in digest: true)

	// CheckTagCardinality is used to override the default tag cardinality in the agent configuration
	CheckTagCardinality string `json:"check_tag_cardinality"` // (include in digest: false)

	// MetricsExcluded is whether metrics collection is disabled (set by
	// container listeners only)
	MetricsExcluded bool `json:"metrics_excluded"` // (include in digest: false)

	// LogsExcluded is whether logs collection is disabled (set by container
	// listeners only)
	LogsExcluded bool `json:"logs_excluded"` // (include in digest: false)
}

// CommonInstanceConfig holds the reserved fields for the yaml instance data
type CommonInstanceConfig struct {
	MinCollectionInterval int      `yaml:"min_collection_interval"`
	EmptyDefaultHostname  bool     `yaml:"empty_default_hostname"`
	Tags                  []string `yaml:"tags"`
	Service               string   `yaml:"service"`
	Name                  string   `yaml:"name"`
	Namespace             string   `yaml:"namespace"`
	NoIndex               bool     `yaml:"no_index"`
}

// CommonGlobalConfig holds the reserved fields for the yaml init_config data
type CommonGlobalConfig struct {
	Service string `yaml:"service"`
}

// AdvancedADIdentifier contains user-defined autodiscovery information
// It replaces ADIdentifiers for advanced use-cases. Typically, file-based k8s service and endpoint checks.
type AdvancedADIdentifier struct {
	KubeService   KubeNamespacedName `yaml:"kube_service,omitempty"`
	KubeEndpoints KubeNamespacedName `yaml:"kube_endpoints,omitempty"`
}

// KubeNamespacedName identifies a kubernetes object.
type KubeNamespacedName struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// IsEmpty returns true if the KubeNamespacedName is empty
func (k KubeNamespacedName) IsEmpty() bool {
	return k.Name == "" && k.Namespace == ""
}

// Equal determines whether the passed config is the same
func (c *Config) Equal(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	return c.Digest() == cfg.Digest()
}

// String constructs the YAML representation of the config.
func (c *Config) String() string {
	rawConfig := make(map[interface{}]interface{})
	var initConfig interface{}
	var instances []interface{}
	var logsConfig interface{}

	rawConfig["check_name"] = c.Name

	yaml.Unmarshal(c.InitConfig, &initConfig) //nolint:errcheck
	rawConfig["init_config"] = initConfig

	for _, i := range c.Instances {
		var instance interface{}
		yaml.Unmarshal(i, &instance) //nolint:errcheck
		instances = append(instances, instance)
	}
	rawConfig["instances"] = instances

	yaml.Unmarshal(c.LogsConfig, &logsConfig) //nolint:errcheck
	rawConfig["logs_config"] = logsConfig

	buffer, err := yaml.Marshal(&rawConfig)
	if err != nil {
		log.Error(err)
	}

	return string(buffer)
}

// ScrubbedString returns the YAML representation of the config with secrets scrubbed
func (c *Config) ScrubbedString() string {
	scrubbed, err := scrubber.ScrubYaml([]byte(c.String()))
	if err != nil {
		log.Errorf("error scrubbing config: %s", err)
		return ""
	}
	return string(scrubbed)
}

// IsTemplate returns if the config has AD identifiers
func (c *Config) IsTemplate() bool {
	return len(c.ADIdentifiers) > 0 || len(c.AdvancedADIdentifiers) > 0
}

// IsCheckConfig returns true if the config is a node-agent check configuration,
func (c *Config) IsCheckConfig() bool {
	return !c.ClusterCheck && len(c.Instances) > 0
}

// IsLogConfig returns true if config contains a logs config.
func (c *Config) IsLogConfig() bool {
	return c.LogsConfig != nil
}

// HasFilter returns true if metrics or logs collection must be disabled for this config.
func (c *Config) HasFilter(filter containers.FilterType) bool {
	// no containers.GlobalFilter case here because we don't create services
	// that are globally excluded in AD
	switch filter {
	case containers.MetricsFilter:
		return c.MetricsExcluded
	case containers.LogsFilter:
		return c.LogsExcluded
	}
	return false
}

// AddMetrics adds metrics to a check configuration
func (c *Config) AddMetrics(metrics Data) error {
	var rawInitConfig RawMap
	err := yaml.Unmarshal(c.InitConfig, &rawInitConfig)
	if err != nil {
		return err
	}

	var rawMetricsConfig []interface{}
	err = yaml.Unmarshal(metrics, &rawMetricsConfig)
	if err != nil {
		return err
	}

	// Grab any metrics currently in init_config
	var conf []interface{}
	currMetrics := make(map[string]bool)
	if _, ok := rawInitConfig["conf"]; ok {
		if currentMetrics, ok := rawInitConfig["conf"].([]interface{}); ok {
			for _, metric := range currentMetrics {
				conf = append(conf, metric)

				if metricS, e := yaml.Marshal(metric); e == nil {
					currMetrics[string(metricS)] = true
				}
			}
		}
	}

	// Add new metrics, skip duplicates
	for _, metric := range rawMetricsConfig {
		if metricS, e := yaml.Marshal(metric); e == nil {
			if !currMetrics[string(metricS)] {
				conf = append(conf, metric)
			}
		}
	}

	// JMX fetch expects the metrics to be a part of init_config, under "conf"
	rawInitConfig["conf"] = conf
	initConfig, err := yaml.Marshal(rawInitConfig)
	if err != nil {
		return err
	}

	c.InitConfig = initConfig
	return nil
}

// GetTemplateVariablesForInstance returns a slice of raw template variables
// it found in a config instance template.
func (c *Config) GetTemplateVariablesForInstance(i int) []tmplvar.TemplateVar {
	if len(c.Instances) < i {
		return nil
	}
	return tmplvar.Parse(c.Instances[i])
}

// GetNameForInstance returns the name from an instance if specified, fallback on namespace
func (c *Data) GetNameForInstance() string {
	commonOptions := CommonInstanceConfig{}
	err := yaml.Unmarshal(*c, &commonOptions)
	if err != nil {
		log.Errorf("invalid instance section: %s", err)
		return ""
	}

	if commonOptions.Name != "" {
		return commonOptions.Name
	}

	// Fallback on `namespace` if we don't find `name`, can be empty
	return commonOptions.Namespace
}

// SetNameForInstance set name for instance
func (c *Data) SetNameForInstance(name string) error {
	commonOptions := CommonInstanceConfig{}
	err := yaml.Unmarshal(*c, &commonOptions)
	if err != nil {
		return fmt.Errorf("invalid instance section: %s", err)
	}
	commonOptions.Name = name

	// modify original config
	out, err := yaml.Marshal(&commonOptions)
	if err != nil {
		return err
	}
	*c = Data(out)

	return nil
}

// MergeAdditionalTags merges additional tags to possible existing config tags
func (c *Data) MergeAdditionalTags(tags []string) error {
	rawConfig := RawMap{}
	err := yaml.Unmarshal(*c, &rawConfig)
	if err != nil {
		return err
	}
	rTags, _ := rawConfig["tags"].([]interface{})
	// convert raw tags to string
	cTags := make([]string, len(rTags))
	for i, t := range rTags {
		cTags[i] = fmt.Sprint(t)
	}
	tagList := append(cTags, tags...)
	if len(tagList) == 0 {
		return nil
	}
	// use set keys to remove duplicate
	tagSet := make(map[string]struct{})
	for _, t := range tagList {
		tagSet[t] = struct{}{}
	}
	// override config tags
	rawConfig["tags"] = []string{}
	for k := range tagSet {
		rawConfig["tags"] = append(rawConfig["tags"].([]string), k)
	}
	// modify original config
	out, err := yaml.Marshal(&rawConfig)
	if err != nil {
		return err
	}
	*c = Data(out)

	return nil
}

// SetField allows to set an arbitrary field to a given value,
// overriding the existing value if present
func (c *Data) SetField(key string, value interface{}) error {
	rawConfig := RawMap{}
	err := yaml.Unmarshal(*c, &rawConfig)
	if err != nil {
		return err
	}

	rawConfig[key] = value
	out, err := yaml.Marshal(&rawConfig)
	if err != nil {
		return err
	}
	*c = Data(out)

	return nil
}

// Digest returns an hash value representing the data stored in this configuration.
// The ClusterCheck field is intentionally left out to keep a stable digest
// between the cluster-agent and the node-agents
func (c *Config) Digest() string {
	return strconv.FormatUint(c.IntDigest(), 16)
}

// IntDigest returns a hash value representing the data stored in the configuration.
func (c *Config) IntDigest() uint64 {
	h := murmur3.New64()
	_, _ = h.Write([]byte(c.Name))
	for _, i := range c.Instances {
		inst := RawMap{}
		err := yaml.Unmarshal(i, &inst)
		if err != nil {
			log.Debugf("Error while calculating config digest for %s, skipping: %v", c.Name, err)
			continue
		}
		if val, found := inst["tags"]; found {
			// sort the list of tags so the digest stays stable for
			// identical configs with the same tags but with different order
			tagsInterface, ok := val.([]interface{})
			if !ok {
				log.Debugf("Error while calculating config digest for %s, skipping: cannot read tags from config", c.Name)
				continue
			}
			tags := make([]string, len(tagsInterface))
			for i, tag := range tagsInterface {
				tags[i] = fmt.Sprint(tag)
			}
			sort.Strings(tags)
			inst["tags"] = tags
		}
		out, err := yaml.Marshal(&inst)
		if err != nil {
			log.Debugf("Error while calculating config digest for %s, skipping: %v", c.Name, err)
			continue
		}
		_, _ = h.Write(out)
	}
	_, _ = h.Write([]byte(c.InitConfig))
	for _, i := range c.ADIdentifiers {
		_, _ = h.Write([]byte(i))
	}
	_, _ = h.Write([]byte(c.NodeName))
	_, _ = h.Write([]byte(c.LogsConfig))
	_, _ = h.Write([]byte(c.ServiceID))
	_, _ = h.Write([]byte(strconv.FormatBool(c.IgnoreAutodiscoveryTags)))

	return h.Sum64()
}

// FastDigest returns an hash value representing the data stored in this configuration.
// Difference with Digest is that FastDigest does not consider that difference may appear inside Instances
// allowing to remove costly YAML Marshal/UnMarshal operations
// The ClusterCheck field is intentionally left out to keep a stable digest
// between the cluster-agent and the node-agents
func (c *Config) FastDigest() uint64 {
	h := murmur3.New64()
	_, _ = h.Write([]byte(c.Name))
	for _, i := range c.Instances {
		_, _ = h.Write([]byte(i))
	}
	_, _ = h.Write([]byte(c.InitConfig))
	for _, i := range c.ADIdentifiers {
		_, _ = h.Write([]byte(i))
	}
	_, _ = h.Write([]byte(c.NodeName))
	_, _ = h.Write([]byte(c.LogsConfig))
	_, _ = h.Write([]byte(c.ServiceID))
	_, _ = h.Write([]byte(strconv.FormatBool(c.IgnoreAutodiscoveryTags)))

	return h.Sum64()
}

// Dump returns a string representing this Config value, for debugging purposes.  If multiline is true,
// then it contains newlines; otherwise, it is comma-separated.
func (c *Config) Dump(multiline bool) string {
	var b strings.Builder
	dataField := func(data Data) string {
		if data == nil {
			return "nil"
		}
		return fmt.Sprintf("[]byte(%#v)", string(data))
	}
	ws := func(fmt string) string {
		if multiline {
			return "\n\t" + fmt
		}
		return " " + fmt
	}

	fmt.Fprint(&b, "integration.Config = {")
	fmt.Fprintf(&b, ws("Name: %#v,"), c.Name)
	if c.Instances == nil {
		fmt.Fprint(&b, ws("Instances: nil,"))
	} else {
		fmt.Fprint(&b, ws("Instances: {"))
		for _, inst := range c.Instances {
			fmt.Fprintf(&b, ws("%s,"), dataField(inst))
		}
		fmt.Fprint(&b, ws("}"))
	}
	fmt.Fprintf(&b, ws("InitConfig: %s,"), dataField(c.InitConfig))
	fmt.Fprintf(&b, ws("MetricConfig: %s,"), dataField(c.MetricConfig))
	fmt.Fprintf(&b, ws("LogsConfig: %s,"), dataField(c.LogsConfig))
	fmt.Fprintf(&b, ws("ADIdentifiers: %#v,"), c.ADIdentifiers)
	fmt.Fprintf(&b, ws("AdvancedADIdentifiers: %#v,"), c.AdvancedADIdentifiers)
	fmt.Fprintf(&b, ws("Provider: %#v,"), c.Provider)
	fmt.Fprintf(&b, ws("ServiceID: %#v,"), c.ServiceID)
	fmt.Fprintf(&b, ws("TaggerEntity: %#v,"), c.TaggerEntity)
	fmt.Fprintf(&b, ws("ClusterCheck: %t,"), c.ClusterCheck)
	fmt.Fprintf(&b, ws("NodeName: %#v,"), c.NodeName)
	fmt.Fprintf(&b, ws("Source: %s,"), c.Source)
	fmt.Fprintf(&b, ws("IgnoreAutodiscoveryTags: %t,"), c.IgnoreAutodiscoveryTags)
	fmt.Fprintf(&b, ws("MetricsExcluded: %t,"), c.MetricsExcluded)
	fmt.Fprintf(&b, ws("LogsExcluded: %t} (digest %s)"), c.LogsExcluded, c.Digest())
	return b.String()
}

// ConfigChanges contains the changes that occurred due to an event in
// AutoDiscovery.
type ConfigChanges struct {
	// Schedule contains configs that should be scheduled as a result of
	// this event.
	Schedule []Config

	// Unschedule contains configs that should be unscheduled as a result of
	// this event.
	Unschedule []Config
}

// ScheduleConfig adds a config to `Schedule`
func (c *ConfigChanges) ScheduleConfig(config Config) {
	c.Schedule = append(c.Schedule, config)
}

// UnscheduleConfig adds a config to `Unschedule`
func (c *ConfigChanges) UnscheduleConfig(config Config) {
	c.Unschedule = append(c.Unschedule, config)
}

// IsEmpty determines whether this set of changes is empty
func (c *ConfigChanges) IsEmpty() bool {
	return len(c.Schedule) == 0 && len(c.Unschedule) == 0
}

// Merge merges the given ConfigChanges into this one.
func (c *ConfigChanges) Merge(other ConfigChanges) {
	c.Schedule = append(c.Schedule, other.Schedule...)
	c.Unschedule = append(c.Unschedule, other.Unschedule...)
}
