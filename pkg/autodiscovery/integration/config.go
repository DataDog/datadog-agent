// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package integration contains the type that represents a configuration.
package integration

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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
	// this is "", then the config is a service config, representing a serivce
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
	panic("not called")
}

// Equal determines whether the passed config is the same
func (c *Config) Equal(cfg *Config) bool {
	panic("not called")
}

// String constructs the YAML representation of the config.
func (c *Config) String() string {
	panic("not called")
}

// IsTemplate returns if the config has AD identifiers
func (c *Config) IsTemplate() bool {
	panic("not called")
}

// IsCheckConfig returns true if the config is a node-agent check configuration,
func (c *Config) IsCheckConfig() bool {
	panic("not called")
}

// IsLogConfig returns true if config contains a logs config.
func (c *Config) IsLogConfig() bool {
	panic("not called")
}

// HasFilter returns true if metrics or logs collection must be disabled for this config.
func (c *Config) HasFilter(filter containers.FilterType) bool {
	panic("not called")
}

// AddMetrics adds metrics to a check configuration
func (c *Config) AddMetrics(metrics Data) error {
	panic("not called")
}

// GetTemplateVariablesForInstance returns a slice of raw template variables
// it found in a config instance template.
func (c *Config) GetTemplateVariablesForInstance(i int) []tmplvar.TemplateVar {
	panic("not called")
}

// GetNameForInstance returns the name from an instance if specified, fallback on namespace
func (c *Data) GetNameForInstance() string {
	panic("not called")
}

// SetNameForInstance set name for instance
func (c *Data) SetNameForInstance(name string) error {
	panic("not called")
}

// MergeAdditionalTags merges additional tags to possible existing config tags
func (c *Data) MergeAdditionalTags(tags []string) error {
	panic("not called")
}

// SetField allows to set an arbitrary field to a given value,
// overriding the existing value if present
func (c *Data) SetField(key string, value interface{}) error {
	panic("not called")
}

// Digest returns an hash value representing the data stored in this configuration.
// The ClusterCheck field is intentionally left out to keep a stable digest
// between the cluster-agent and the node-agents
func (c *Config) Digest() string {
	panic("not called")
}

// IntDigest returns a hash value representing the data stored in the configuration.
func (c *Config) IntDigest() uint64 {
	panic("not called")
}

// FastDigest returns an hash value representing the data stored in this configuration.
// Difference with Digest is that FastDigest does not consider that difference may appear inside Instances
// allowing to remove costly YAML Marshal/UnMarshal operations
// The ClusterCheck field is intentionally left out to keep a stable digest
// between the cluster-agent and the node-agents
func (c *Config) FastDigest() uint64 {
	panic("not called")
}

// Dump returns a string representing this Config value, for debugging purposes.  If multiline is true,
// then it contains newlines; otherwise, it is comma-separated.
func (c *Config) Dump(multiline bool) string {
	panic("not called")
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
	panic("not called")
}

// UnscheduleConfig adds a config to `Unschedule`
func (c *ConfigChanges) UnscheduleConfig(config Config) {
	panic("not called")
}

// IsEmpty determines whether this set of changes is empty
func (c *ConfigChanges) IsEmpty() bool {
	panic("not called")
}

// Merge merges the given ConfigChanges into this one.
func (c *ConfigChanges) Merge(other ConfigChanges) {
	panic("not called")
}
