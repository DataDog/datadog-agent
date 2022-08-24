// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	coreutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// Using high oid batch size might lead to snmp calls timing out.
// For some devices, the default oid_batch_size of 5 might be high (leads to timeouts),
// and require manual setting oid_batch_size to a lower value.
const defaultOidBatchSize = 5

const defaultPort = uint16(161)
const defaultRetries = 3
const defaultTimeout = 2
const defaultWorkers = 5
const defaultDiscoveryWorkers = 5
const defaultDiscoveryAllowedFailures = 3
const defaultDiscoveryInterval = 3600

// subnetTagKey is the prefix used for subnet tag
const subnetTagKey = "autodiscovery_subnet"
const deviceNamespaceTagKey = "device_namespace"
const deviceIPTagKey = "snmp_device"

// DefaultBulkMaxRepetitions is the default max rep
// Using too high max repetitions might lead to tooBig SNMP error messages.
// - Java SNMP and gosnmp (gosnmp.defaultMaxRepetitions) uses 50
// - snmp-net uses 10
const DefaultBulkMaxRepetitions = uint32(10)

var uptimeMetricConfig = MetricsConfig{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}

// DeviceDigest is the digest of a minimal config used for autodiscovery
type DeviceDigest string

// InitConfig is used to deserialize integration init config
type InitConfig struct {
	Profiles              profileConfigMap `yaml:"profiles"`
	GlobalMetrics         []MetricsConfig  `yaml:"global_metrics"`
	OidBatchSize          Number           `yaml:"oid_batch_size"`
	BulkMaxRepetitions    Number           `yaml:"bulk_max_repetitions"`
	CollectDeviceMetadata Boolean          `yaml:"collect_device_metadata"`
	UseDeviceIDAsHostname Boolean          `yaml:"use_device_id_as_hostname"`
	MinCollectionInterval int              `yaml:"min_collection_interval"`
	Namespace             string           `yaml:"namespace"`
}

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	Name                  string            `yaml:"name"`
	IPAddress             string            `yaml:"ip_address"`
	Port                  Number            `yaml:"port"`
	CommunityString       string            `yaml:"community_string"`
	SnmpVersion           string            `yaml:"snmp_version"`
	Timeout               Number            `yaml:"timeout"`
	Retries               Number            `yaml:"retries"`
	User                  string            `yaml:"user"`
	AuthProtocol          string            `yaml:"authProtocol"`
	AuthKey               string            `yaml:"authKey"`
	PrivProtocol          string            `yaml:"privProtocol"`
	PrivKey               string            `yaml:"privKey"`
	ContextName           string            `yaml:"context_name"`
	Metrics               []MetricsConfig   `yaml:"metrics"`     // SNMP metrics definition
	MetricTags            []MetricTagConfig `yaml:"metric_tags"` // SNMP metric tags definition
	Profile               string            `yaml:"profile"`
	UseGlobalMetrics      bool              `yaml:"use_global_metrics"`
	CollectDeviceMetadata *Boolean          `yaml:"collect_device_metadata"`
	UseDeviceIDAsHostname *Boolean          `yaml:"use_device_id_as_hostname"`

	// ExtraTags is a workaround to pass tags from snmp listener to snmp integration via AD template
	// (see cmd/agent/dist/conf.d/snmp.d/auto_conf.yaml) that only works with strings.
	// TODO: deprecated extra tags in favour of using autodiscovery listener Service.GetTags()
	ExtraTags string `yaml:"extra_tags"` // comma separated tags

	// Tags are just static tags from the instance that is common to all integrations.
	// Normally, the Agent will enrich metrics with the metrics with those tags.
	// See https://github.com/DataDog/datadog-agent/blob/1e8321ff089d04ccce3987b84f8b75630d7a18c0/pkg/collector/corechecks/checkbase.go#L131-L139
	// But we need to deserialize here since we need them for NDM metadata.
	Tags []string `yaml:"tags"` // used for device metadata

	// The oid_batch_size indicates how many OIDs are retrieved in a single Get or GetBulk call
	OidBatchSize Number `yaml:"oid_batch_size"`
	// The bulk_max_repetitions config indicates how many rows of the table are to be retrieved in a single GetBulk call
	BulkMaxRepetitions Number `yaml:"bulk_max_repetitions"`

	MinCollectionInterval int `yaml:"min_collection_interval"`
	// To accept min collection interval from snmp_listener, we need to accept it as string.
	// Using extra_min_collection_interval, we can accept both string and integer value.
	ExtraMinCollectionInterval Number `yaml:"extra_min_collection_interval"`

	Network                  string   `yaml:"network_address"`
	IgnoredIPAddresses       []string `yaml:"ignored_ip_addresses"`
	DiscoveryInterval        int      `yaml:"discovery_interval"`
	DiscoveryAllowedFailures int      `yaml:"discovery_allowed_failures"`
	DiscoveryWorkers         int      `yaml:"discovery_workers"`
	Workers                  int      `yaml:"workers"`
	Namespace                string   `yaml:"namespace"`
}

// CheckConfig holds config needed for an integration instance to run
type CheckConfig struct {
	Name                  string
	IPAddress             string
	Port                  uint16
	CommunityString       string
	SnmpVersion           string
	Timeout               int
	Retries               int
	User                  string
	AuthProtocol          string
	AuthKey               string
	PrivProtocol          string
	PrivKey               string
	ContextName           string
	OidConfig             OidConfig
	Metrics               []MetricsConfig
	Metadata              MetadataConfig
	MetricTags            []MetricTagConfig
	OidBatchSize          int
	BulkMaxRepetitions    uint32
	Profiles              profileDefinitionMap
	ProfileTags           []string
	Profile               string
	ProfileDef            *profileDefinition
	ExtraTags             []string
	InstanceTags          []string
	CollectDeviceMetadata bool
	UseDeviceIDAsHostname bool
	DeviceID              string
	DeviceIDTags          []string
	ResolvedSubnetName    string
	Namespace             string
	AutodetectProfile     bool
	MinCollectionInterval time.Duration

	Network                  string
	DiscoveryWorkers         int
	Workers                  int
	DiscoveryInterval        int
	IgnoredIPAddresses       map[string]bool
	DiscoveryAllowedFailures int
}

// RefreshWithProfile refreshes config based on profile
func (c *CheckConfig) RefreshWithProfile(profile string) error {
	if _, ok := c.Profiles[profile]; !ok {
		return fmt.Errorf("unknown profile `%s`", profile)
	}
	log.Debugf("Refreshing with profile `%s`", profile)
	tags := []string{"snmp_profile:" + profile}
	definition := c.Profiles[profile]
	c.ProfileDef = &definition
	c.Profile = profile

	c.Metadata = updateMetadataDefinitionWithLegacyFallback(definition.Metadata)
	c.Metrics = append(c.Metrics, definition.Metrics...)
	c.MetricTags = append(c.MetricTags, definition.MetricTags...)

	c.OidConfig.clean()
	c.OidConfig.addScalarOids(c.parseScalarOids(c.Metrics, c.MetricTags, c.Metadata))
	c.OidConfig.addColumnOids(c.parseColumnOids(c.Metrics, c.Metadata))

	if definition.Device.Vendor != "" {
		tags = append(tags, "device_vendor:"+definition.Device.Vendor)
	}
	tags = append(tags, definition.StaticTags...)
	c.ProfileTags = tags
	return nil
}

// UpdateDeviceIDAndTags updates DeviceID and DeviceIDTags
func (c *CheckConfig) UpdateDeviceIDAndTags() {
	c.DeviceIDTags = coreutil.SortUniqInPlace(c.getDeviceIDTags())
	c.DeviceID = c.Namespace + ":" + c.IPAddress
}

func (c *CheckConfig) addUptimeMetric() {
	c.Metrics = append(c.Metrics, uptimeMetricConfig)
}

// GetStaticTags return static tags built from configuration
func (c *CheckConfig) GetStaticTags() []string {
	tags := common.CopyStrings(c.ExtraTags)
	tags = append(tags, deviceNamespaceTagKey+":"+c.Namespace)
	if c.IPAddress != "" {
		tags = append(tags, deviceIPTagKey+":"+c.IPAddress)
	}

	if c.UseDeviceIDAsHostname {
		hname, err := hostname.Get(context.TODO())
		if err != nil {
			log.Warnf("Error getting the hostname: %v", err)
		} else {
			tags = append(tags, "agent_host:"+hname)
		}
	}
	return tags
}

// GetNetworkTags returns network tags
// network tags are not part of the static tags since we don't want the deviceID
// to change if the network/subnet changes e.g. 10.0.0.0/29 to 10.0.0.0/30
func (c *CheckConfig) GetNetworkTags() []string {
	var tags []string
	if c.Network != "" {
		tags = append(tags, subnetTagKey+":"+c.Network)
	}
	return tags
}

// getDeviceIDTags return sorted tags used for generating device id
// warning: changing getDeviceIDTags logic might lead to different deviceID
func (c *CheckConfig) getDeviceIDTags() []string {
	tags := []string{deviceNamespaceTagKey + ":" + c.Namespace, deviceIPTagKey + ":" + c.IPAddress}
	sort.Strings(tags)
	return tags
}

// ToString used for logging CheckConfig without sensitive information
func (c *CheckConfig) ToString() string {
	return fmt.Sprintf("CheckConfig: IPAddress=`%s`, Port=`%d`, SnmpVersion=`%s`, Timeout=`%d`, Retries=`%d`, "+
		"User=`%s`, AuthProtocol=`%s`, PrivProtocol=`%s`, ContextName=`%s`, OidConfig=`%#v`, "+
		"OidBatchSize=`%d`, ProfileTags=`%#v`",
		c.IPAddress,
		c.Port,
		c.SnmpVersion,
		c.Timeout,
		c.Retries,
		c.User,
		c.AuthProtocol,
		c.PrivProtocol,
		c.ContextName,
		c.OidConfig,
		c.OidBatchSize,
		c.ProfileTags,
	)
}

// NewCheckConfig builds a new check config
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}
	initConfig := InitConfig{}

	// Set defaults before unmarshalling
	instance.UseGlobalMetrics = true
	initConfig.CollectDeviceMetadata = true

	err := yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return nil, err
	}

	c := &CheckConfig{}

	c.Name = instance.Name
	c.SnmpVersion = instance.SnmpVersion
	c.IPAddress = instance.IPAddress
	c.Port = uint16(instance.Port)
	c.Network = instance.Network

	if c.IPAddress == "" && c.Network == "" {
		return nil, fmt.Errorf("`ip_address` or `network` config must be provided")
	}

	if c.IPAddress != "" && c.Network != "" {
		return nil, fmt.Errorf("`ip_address` and `network` cannot be used at the same time")
	}
	if c.Network != "" {
		_, _, err = net.ParseCIDR(c.Network)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse SNMP network: %s", err)
		}
	}

	if instance.CollectDeviceMetadata != nil {
		c.CollectDeviceMetadata = bool(*instance.CollectDeviceMetadata)
	} else {
		c.CollectDeviceMetadata = bool(initConfig.CollectDeviceMetadata)
	}

	if instance.UseDeviceIDAsHostname != nil {
		c.UseDeviceIDAsHostname = bool(*instance.UseDeviceIDAsHostname)
	} else {
		c.UseDeviceIDAsHostname = bool(initConfig.UseDeviceIDAsHostname)
	}

	if instance.ExtraTags != "" {
		c.ExtraTags = strings.Split(instance.ExtraTags, ",")
	}

	if instance.DiscoveryWorkers == 0 {
		c.DiscoveryWorkers = defaultDiscoveryWorkers
	} else {
		c.DiscoveryWorkers = instance.DiscoveryWorkers
	}

	if instance.Workers == 0 {
		c.Workers = defaultWorkers
	} else {
		c.Workers = instance.Workers
	}

	if instance.DiscoveryAllowedFailures == 0 {
		c.DiscoveryAllowedFailures = defaultDiscoveryAllowedFailures
	} else {
		c.DiscoveryAllowedFailures = instance.DiscoveryAllowedFailures
	}

	if instance.DiscoveryInterval == 0 {
		c.DiscoveryInterval = defaultDiscoveryInterval
	} else {
		c.DiscoveryInterval = instance.DiscoveryInterval
	}

	c.IgnoredIPAddresses = make(map[string]bool, len(instance.IgnoredIPAddresses))
	for _, ipAddress := range instance.IgnoredIPAddresses {
		c.IgnoredIPAddresses[ipAddress] = true
	}

	if c.Port == 0 {
		c.Port = defaultPort
	}

	if instance.Retries == 0 {
		c.Retries = defaultRetries
	} else {
		c.Retries = int(instance.Retries)
	}

	if instance.Timeout == 0 {
		c.Timeout = defaultTimeout
	} else {
		c.Timeout = int(instance.Timeout)
	}

	if instance.ExtraMinCollectionInterval != 0 {
		c.MinCollectionInterval = time.Duration(instance.ExtraMinCollectionInterval) * time.Second
	} else if instance.MinCollectionInterval != 0 {
		c.MinCollectionInterval = time.Duration(instance.MinCollectionInterval) * time.Second
	} else if initConfig.MinCollectionInterval != 0 {
		c.MinCollectionInterval = time.Duration(initConfig.MinCollectionInterval) * time.Second
	} else {
		c.MinCollectionInterval = defaults.DefaultCheckInterval
	}
	if c.MinCollectionInterval < 0 {
		return nil, fmt.Errorf("min collection interval must be > 0, but got: %v", c.MinCollectionInterval.Seconds())
	}

	// SNMP connection configs
	c.CommunityString = instance.CommunityString
	c.User = instance.User
	c.AuthProtocol = instance.AuthProtocol
	c.AuthKey = instance.AuthKey
	c.PrivProtocol = instance.PrivProtocol
	c.PrivKey = instance.PrivKey
	c.ContextName = instance.ContextName

	c.Metrics = instance.Metrics

	if instance.OidBatchSize != 0 {
		c.OidBatchSize = int(instance.OidBatchSize)
	} else if initConfig.OidBatchSize != 0 {
		c.OidBatchSize = int(initConfig.OidBatchSize)
	} else {
		c.OidBatchSize = defaultOidBatchSize
	}

	var bulkMaxRepetitions int
	if instance.BulkMaxRepetitions != 0 {
		bulkMaxRepetitions = int(instance.BulkMaxRepetitions)
	} else if initConfig.BulkMaxRepetitions != 0 {
		bulkMaxRepetitions = int(initConfig.BulkMaxRepetitions)
	} else {
		bulkMaxRepetitions = int(DefaultBulkMaxRepetitions)
	}
	if bulkMaxRepetitions <= 0 {
		return nil, fmt.Errorf("bulk max repetition must be a positive integer. Invalid value: %d", bulkMaxRepetitions)
	}
	c.BulkMaxRepetitions = uint32(bulkMaxRepetitions)

	if instance.Namespace != "" {
		c.Namespace = instance.Namespace
	} else if initConfig.Namespace != "" {
		c.Namespace = initConfig.Namespace
	} else {
		c.Namespace = coreconfig.Datadog.GetString("network_devices.namespace")
	}

	c.Namespace, err = common.NormalizeNamespace(c.Namespace)
	if err != nil {
		return nil, err
	}

	// profile configs
	profile := instance.Profile
	if profile != "" || len(c.Metrics) > 0 {
		c.AutodetectProfile = false
	} else {
		c.AutodetectProfile = true
	}

	// metrics Configs
	if instance.UseGlobalMetrics {
		c.Metrics = append(c.Metrics, initConfig.GlobalMetrics...)
	}
	normalizeMetrics(c.Metrics)

	c.InstanceTags = instance.Tags
	c.MetricTags = instance.MetricTags

	c.addUptimeMetric()

	c.Metadata = updateMetadataDefinitionWithLegacyFallback(nil)
	c.OidConfig.addScalarOids(c.parseScalarOids(c.Metrics, c.MetricTags, c.Metadata))
	c.OidConfig.addColumnOids(c.parseColumnOids(c.Metrics, c.Metadata))

	// Profile Configs
	var profiles profileDefinitionMap
	if len(initConfig.Profiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, err := loadProfiles(initConfig.Profiles)
		if err != nil {
			return nil, fmt.Errorf("failed to load custom profiles: %s", err)
		}
		profiles = customProfiles
	} else {
		defaultProfiles, err := loadDefaultProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load default profiles: %s", err)
		}
		profiles = defaultProfiles
	}

	for _, profileDef := range profiles {
		normalizeMetrics(profileDef.Metrics)
	}

	c.Profiles = profiles

	errors := ValidateEnrichMetrics(c.Metrics)
	errors = append(errors, ValidateEnrichMetricTags(c.MetricTags)...)
	if len(errors) > 0 {
		return nil, fmt.Errorf("validation errors: %s", strings.Join(errors, "\n"))
	}

	if profile != "" {
		err = c.RefreshWithProfile(profile)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh with profile `%s`: %s", profile, err)
		}
	}

	c.UpdateDeviceIDAndTags()

	c.ResolvedSubnetName = c.getResolvedSubnetName()
	return c, nil
}

func (c *CheckConfig) getResolvedSubnetName() string {
	var resolvedSubnet string
	if c.Network != "" {
		resolvedSubnet = c.Network
	} else {
		subnet, err := getSubnetFromTags(c.InstanceTags)
		if err != nil {
			log.Debugf("subnet not found: %s", err)
		} else {
			resolvedSubnet = subnet
		}
	}
	return resolvedSubnet
}

// DeviceDigest returns a hash value representing the minimal configs used to connect to the device.
// DeviceDigest is used for device discovery.
func (c *CheckConfig) DeviceDigest(address string) DeviceDigest {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(address))                   //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%d", c.Port))) //nolint:errcheck
	h.Write([]byte(c.SnmpVersion))             //nolint:errcheck
	h.Write([]byte(c.CommunityString))         //nolint:errcheck
	h.Write([]byte(c.User))                    //nolint:errcheck
	h.Write([]byte(c.AuthKey))                 //nolint:errcheck
	h.Write([]byte(c.AuthProtocol))            //nolint:errcheck
	h.Write([]byte(c.PrivKey))                 //nolint:errcheck
	h.Write([]byte(c.PrivProtocol))            //nolint:errcheck
	h.Write([]byte(c.ContextName))             //nolint:errcheck

	// Sort the addresses to get a stable digest
	addresses := make([]string, 0, len(c.IgnoredIPAddresses))
	for ip := range c.IgnoredIPAddresses {
		addresses = append(addresses, ip)
	}
	sort.Strings(addresses)
	for _, ip := range addresses {
		h.Write([]byte(ip)) //nolint:errcheck
	}

	return DeviceDigest(strconv.FormatUint(h.Sum64(), 16))
}

// IsIPIgnored checks the given IP against ignoredIPAddresses
func (c *CheckConfig) IsIPIgnored(ip net.IP) bool {
	ipString := ip.String()
	_, present := c.IgnoredIPAddresses[ipString]
	return present
}

// Copy makes a copy of CheckConfig
func (c *CheckConfig) Copy() *CheckConfig {
	newConfig := CheckConfig{}
	newConfig.IPAddress = c.IPAddress
	newConfig.Network = c.Network
	newConfig.Port = c.Port
	newConfig.CommunityString = c.CommunityString
	newConfig.SnmpVersion = c.SnmpVersion
	newConfig.Timeout = c.Timeout
	newConfig.Retries = c.Retries
	newConfig.User = c.User
	newConfig.AuthProtocol = c.AuthProtocol
	newConfig.AuthKey = c.AuthKey
	newConfig.PrivProtocol = c.PrivProtocol
	newConfig.PrivKey = c.PrivKey
	newConfig.ContextName = c.ContextName
	newConfig.ContextName = c.ContextName
	newConfig.OidConfig = c.OidConfig
	newConfig.Metrics = make([]MetricsConfig, 0, len(c.Metrics))
	for _, metric := range c.Metrics {
		newConfig.Metrics = append(newConfig.Metrics, metric)
	}

	// Metadata: shallow copy is enough since metadata is not modified.
	// However, it might be fully replaced, see CheckConfig.RefreshWithProfile
	newConfig.Metadata = c.Metadata

	newConfig.MetricTags = make([]MetricTagConfig, 0, len(c.MetricTags))
	for _, metricTag := range c.MetricTags {
		newConfig.MetricTags = append(newConfig.MetricTags, metricTag)
	}
	newConfig.OidBatchSize = c.OidBatchSize
	newConfig.BulkMaxRepetitions = c.BulkMaxRepetitions
	newConfig.Profiles = c.Profiles
	newConfig.ProfileTags = common.CopyStrings(c.ProfileTags)
	newConfig.Profile = c.Profile
	newConfig.ProfileDef = c.ProfileDef
	newConfig.ExtraTags = common.CopyStrings(c.ExtraTags)
	newConfig.InstanceTags = common.CopyStrings(c.InstanceTags)
	newConfig.CollectDeviceMetadata = c.CollectDeviceMetadata
	newConfig.UseDeviceIDAsHostname = c.UseDeviceIDAsHostname
	newConfig.DeviceID = c.DeviceID

	newConfig.DeviceIDTags = common.CopyStrings(c.DeviceIDTags)
	newConfig.ResolvedSubnetName = c.ResolvedSubnetName
	newConfig.Namespace = c.Namespace
	newConfig.AutodetectProfile = c.AutodetectProfile
	newConfig.MinCollectionInterval = c.MinCollectionInterval

	return &newConfig
}

// CopyWithNewIP makes a copy of CheckConfig with new IP
func (c *CheckConfig) CopyWithNewIP(ipAddress string) *CheckConfig {
	newConfig := c.Copy()
	newConfig.IPAddress = ipAddress
	newConfig.UpdateDeviceIDAndTags()
	return newConfig
}

// IsDiscovery return weather it's a network/autodiscovery config or not
func (c *CheckConfig) IsDiscovery() bool {
	return c.Network != ""
}

func (c *CheckConfig) parseScalarOids(metrics []MetricsConfig, metricTags []MetricTagConfig, metadataConfigs MetadataConfig) []string {
	var oids []string
	for _, metric := range metrics {
		oids = append(oids, metric.Symbol.OID)
	}
	for _, metricTag := range metricTags {
		oids = append(oids, metricTag.OID)
	}
	if c.CollectDeviceMetadata {
		for resource, metadataConfig := range metadataConfigs {
			if !IsMetadataResourceWithScalarOids(resource) {
				continue
			}
			for _, field := range metadataConfig.Fields {
				oids = append(oids, field.Symbol.OID)
				for _, symbol := range field.Symbols {
					oids = append(oids, symbol.OID)
				}
			}
			// we don't support tags for now for resource (e.g. device) based on scalar OIDs
			// profile root level `metric_tags` (tags used for both metadata, metrics, service checks)
			// can be used instead
		}
	}
	return oids
}

func (c *CheckConfig) parseColumnOids(metrics []MetricsConfig, metadataConfigs MetadataConfig) []string {
	var oids []string
	for _, metric := range metrics {
		for _, symbol := range metric.Symbols {
			oids = append(oids, symbol.OID)
		}
		for _, metricTag := range metric.MetricTags {
			oids = append(oids, metricTag.Column.OID)
		}
	}
	if c.CollectDeviceMetadata {
		for resource, metadataConfig := range metadataConfigs {
			if IsMetadataResourceWithScalarOids(resource) {
				continue
			}
			for _, field := range metadataConfig.Fields {
				oids = append(oids, field.Symbol.OID)
				for _, symbol := range field.Symbols {
					oids = append(oids, symbol.OID)
				}
			}
			for _, tagConfig := range metadataConfig.IDTags {
				oids = append(oids, tagConfig.Column.OID)
			}
		}
	}
	return oids
}

// GetProfileForSysObjectID return a profile for a sys object id
func GetProfileForSysObjectID(profiles profileDefinitionMap, sysObjectID string) (string, error) {
	tmpSysOidToProfile := map[string]string{}
	var matchedOids []string

	for profile, definition := range profiles {
		for _, oidPattern := range definition.SysObjectIds {
			found, err := filepath.Match(oidPattern, sysObjectID)
			if err != nil {
				log.Debugf("pattern error: %s", err)
				continue
			}
			if !found {
				continue
			}
			if matchedProfile, ok := tmpSysOidToProfile[oidPattern]; ok {
				return "", fmt.Errorf("profile %s has the same sysObjectID (%s) as %s", profile, oidPattern, matchedProfile)
			}
			tmpSysOidToProfile[oidPattern] = profile
			matchedOids = append(matchedOids, oidPattern)
		}
	}
	oid, err := getMostSpecificOid(matchedOids)
	if err != nil {
		return "", fmt.Errorf("failed to get most specific profile for sysObjectID `%s`, for matched oids %v: %s", sysObjectID, matchedOids, err)
	}
	return tmpSysOidToProfile[oid], nil
}

func getSubnetFromTags(tags []string) (string, error) {
	for _, tag := range tags {
		// `autodiscovery_subnet` is set as tags in AD Template
		// e.g. cmd/agent/dist/conf.d/snmp.d/auto_conf.yaml
		prefix := subnetTagKey + ":"
		if strings.HasPrefix(tag, prefix) {
			return tag[len(prefix):], nil
		}
	}
	return "", fmt.Errorf("subnet not found in tags %v", tags)
}
