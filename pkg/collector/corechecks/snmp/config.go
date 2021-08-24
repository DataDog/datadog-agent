package snmp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: Refactor all config (and config parsing) related elements to a separate package

// Using high oid batch size might lead to snmp calls timing out.
// For some devices, the default oid_batch_size of 5 might be high (leads to timeouts),
// and require manual setting oid_batch_size to a lower value.
var defaultOidBatchSize = 5

var defaultPort = uint16(161)
var defaultRetries = 3
var defaultTimeout = 2
var subnetTagPrefix = "autodiscovery_subnet"

// DefaultBulkMaxRepetitions is the default max rep
// Using too high max repetitions might lead to tooBig SNMP error messages.
// - Java SNMP and gosnmp (gosnmp.defaultMaxRepetitions) uses 50
// - snmp-net uses 10
const DefaultBulkMaxRepetitions = uint32(10)

// InitConfig maps to a check init config
type InitConfig struct {
	Profiles              profileConfigMap `yaml:"profiles"`
	GlobalMetrics         []MetricsConfig  `yaml:"global_metrics"`
	OidBatchSize          Number           `yaml:"oid_batch_size"`
	BulkMaxRepetitions    Number           `yaml:"bulk_max_repetitions"`
	CollectDeviceMetadata Boolean          `yaml:"collect_device_metadata"`
	MinCollectionInterval int              `yaml:"min_collection_interval"`
}

// InstanceConfig maps to a check instance config
type InstanceConfig struct {
	IPAddress             string            `yaml:"ip_address"`
	Port                  Number            `yaml:"port"`
	CommunityString       string            `yaml:"community_string"`
	SnmpVersion           string            `yaml:"snmp_version"`
	Timeout               Number            `yaml:"timeout"`
	Retries               Number            `yaml:"retries"`
	OidBatchSize          Number            `yaml:"oid_batch_size"`
	BulkMaxRepetitions    Number            `yaml:"bulk_max_repetitions"`
	User                  string            `yaml:"user"`
	AuthProtocol          string            `yaml:"authProtocol"`
	AuthKey               string            `yaml:"authKey"`
	PrivProtocol          string            `yaml:"privProtocol"`
	PrivKey               string            `yaml:"privKey"`
	ContextName           string            `yaml:"context_name"`
	Metrics               []MetricsConfig   `yaml:"metrics"`
	MetricTags            []MetricTagConfig `yaml:"metric_tags"`
	Profile               string            `yaml:"profile"`
	UseGlobalMetrics      bool              `yaml:"use_global_metrics"`
	ExtraTags             string            `yaml:"extra_tags"` // comma separated tags
	Tags                  []string          `yaml:"tags"`       // used for device metadata
	CollectDeviceMetadata *Boolean          `yaml:"collect_device_metadata"`

	// To accept min collection interval from snmp_listener, we need to accept it as string
	// extra_min_collection_interval can accept both string and integer value
	MinCollectionInterval      int    `yaml:"min_collection_interval"`
	ExtraMinCollectionInterval Number `yaml:"extra_min_collection_interval"`

	// `network` config is only available in Python SNMP integration
	// it's added here to raise warning if used with corecheck SNMP integration
	Network string `yaml:"network_address"`
}

// CheckConfig holds config for a check instance
type CheckConfig struct {
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
	DeviceID              string
	DeviceIDTags          []string
	Subnet                string
	AutodetectProfile     bool
	MinCollectionInterval time.Duration
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

	c.Metrics = append(c.Metrics, definition.Metrics...)
	c.MetricTags = append(c.MetricTags, definition.MetricTags...)
	c.OidConfig.addScalarOids(parseScalarOids(definition.Metrics, definition.MetricTags))
	c.OidConfig.addColumnOids(parseColumnOids(definition.Metrics))

	if definition.Device.Vendor != "" {
		tags = append(tags, "device_vendor:"+definition.Device.Vendor)
	}
	c.ProfileTags = tags
	return nil
}

func (c *CheckConfig) addUptimeMetric() {
	metricConfig := getUptimeMetricConfig()
	c.Metrics = append(c.Metrics, metricConfig)
	c.OidConfig.addScalarOids([]string{metricConfig.Symbol.OID})
}

// GetStaticTags return static tags built from configuration
// warning: changing GetStaticTags logic might lead to different deviceID
// GetStaticTags does not contain tags from instance[].tags config
func (c *CheckConfig) GetStaticTags() []string {
	tags := []string{"snmp_device:" + c.IPAddress}
	tags = append(tags, c.ExtraTags...)
	return tags
}

// getDeviceIDTags return sorted tags used for generating device id
// warning: changing getDeviceIDTags logic might lead to different deviceID
func (c *CheckConfig) getDeviceIDTags() []string {
	tags := c.GetStaticTags()
	tags = append(tags, c.InstanceTags...)
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

// BuildConfig builds a new check config
func BuildConfig(rawInstance integration.Data, rawInitConfig integration.Data) (CheckConfig, error) {
	instance := InstanceConfig{}
	initConfig := InitConfig{}

	// Set defaults before unmarshalling
	instance.UseGlobalMetrics = true

	err := yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return CheckConfig{}, err
	}

	err = yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return CheckConfig{}, err
	}

	c := CheckConfig{}

	c.SnmpVersion = instance.SnmpVersion
	c.IPAddress = instance.IPAddress
	c.Port = uint16(instance.Port)

	if instance.CollectDeviceMetadata != nil {
		c.CollectDeviceMetadata = bool(*instance.CollectDeviceMetadata)
	} else {
		c.CollectDeviceMetadata = bool(initConfig.CollectDeviceMetadata)
	}

	if instance.ExtraTags != "" {
		c.ExtraTags = strings.Split(instance.ExtraTags, ",")
	}

	if instance.Network != "" {
		log.Warnf("`network_address` config is not available for corecheck SNMP integration to use autodiscovery. Agent `snmp_listener` config can be used instead: https://docs.datadoghq.com/network_monitoring/devices/setup?tab=snmpv2#autodiscovery")
	}

	if c.IPAddress == "" {
		return CheckConfig{}, fmt.Errorf("ip_address config must be provided")
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
		return CheckConfig{}, fmt.Errorf("min collection interval must be > 0, but got: %v", c.MinCollectionInterval.Seconds())
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
		return CheckConfig{}, fmt.Errorf("bulk max repetition must be a positive integer. Invalid value: %d", bulkMaxRepetitions)
	}
	c.BulkMaxRepetitions = uint32(bulkMaxRepetitions)

	// metrics Configs
	if instance.UseGlobalMetrics {
		c.Metrics = append(c.Metrics, initConfig.GlobalMetrics...)
	}
	normalizeMetrics(c.Metrics)

	c.InstanceTags = instance.Tags
	c.MetricTags = instance.MetricTags

	c.OidConfig.addScalarOids(parseScalarOids(c.Metrics, c.MetricTags))
	c.OidConfig.addColumnOids(parseColumnOids(c.Metrics))

	if c.CollectDeviceMetadata {
		c.OidConfig.addScalarOids(metadata.ScalarOIDs)
		c.OidConfig.addColumnOids(metadata.ColumnOIDs)
	}

	// Profile Configs
	var profiles profileDefinitionMap
	if len(initConfig.Profiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, err := loadProfiles(initConfig.Profiles)
		if err != nil {
			return CheckConfig{}, fmt.Errorf("failed to load custom profiles: %s", err)
		}
		profiles = customProfiles
	} else {
		defaultProfiles, err := loadDefaultProfiles()
		if err != nil {
			return CheckConfig{}, fmt.Errorf("failed to load default profiles: %s", err)
		}
		profiles = defaultProfiles
	}

	for _, profileDef := range profiles {
		normalizeMetrics(profileDef.Metrics)
	}

	c.Profiles = profiles
	profile := instance.Profile

	errors := validateEnrichMetrics(c.Metrics)
	errors = append(errors, ValidateEnrichMetricTags(c.MetricTags)...)
	if len(errors) > 0 {
		return CheckConfig{}, fmt.Errorf("validation errors: %s", strings.Join(errors, "\n"))
	}

	if profile != "" || len(c.Metrics) > 0 {
		c.AutodetectProfile = false
	} else {
		c.AutodetectProfile = true
	}

	if profile != "" {
		err = c.RefreshWithProfile(profile)
		if err != nil {
			return CheckConfig{}, fmt.Errorf("failed to refresh with profile `%s`: %s", profile, err)
		}
	}

	c.DeviceID, c.DeviceIDTags = buildDeviceID(c.getDeviceIDTags())

	subnet, err := getSubnetFromTags(c.InstanceTags)
	if err != nil {
		log.Debugf("subnet not found: %s", err)
	}
	c.Subnet = subnet

	c.addUptimeMetric()
	return c, nil
}

func getUptimeMetricConfig() MetricsConfig {
	// Reference sysUpTimeInstance directly, see http://oidref.com/1.3.6.1.2.1.1.3.0
	return MetricsConfig{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}
}

func parseScalarOids(metrics []MetricsConfig, metricTags []MetricTagConfig) []string {
	var oids []string
	for _, metric := range metrics {
		if metric.Symbol.OID != "" {
			oids = append(oids, metric.Symbol.OID)
		}
	}
	for _, metricTag := range metricTags {
		if metricTag.OID != "" {
			oids = append(oids, metricTag.OID)
		}
	}
	return oids
}

func parseColumnOids(metrics []MetricsConfig) []string {
	var oids []string
	for _, metric := range metrics {
		for _, symbol := range metric.Symbols {
			oids = append(oids, symbol.OID)
		}
		for _, metricTag := range metric.MetricTags {
			if metricTag.Column.OID != "" {
				oids = append(oids, metricTag.Column.OID)
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
		prefix := subnetTagPrefix + ":"
		if strings.HasPrefix(tag, prefix) {
			return tag[len(prefix):], nil
		}
	}
	return "", fmt.Errorf("subnet not found in tags %v", tags)
}
