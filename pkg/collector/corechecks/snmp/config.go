package snmp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var defaultOidBatchSize = 60
var defaultPort = uint16(161)
var defaultRetries = 3
var defaultTimeout = 2
var subnetTagPrefix = "autodiscovery_subnet"

type snmpInitConfig struct {
	Profiles              profileConfigMap `yaml:"profiles"`
	GlobalMetrics         []metricsConfig  `yaml:"global_metrics"`
	OidBatchSize          Number           `yaml:"oid_batch_size"`
	CollectDeviceMetadata Boolean          `yaml:"collect_device_metadata"`
}

type snmpInstanceConfig struct {
	IPAddress             string            `yaml:"ip_address"`
	Port                  Number            `yaml:"port"`
	CommunityString       string            `yaml:"community_string"`
	SnmpVersion           string            `yaml:"snmp_version"`
	Timeout               Number            `yaml:"timeout"`
	Retries               Number            `yaml:"retries"`
	OidBatchSize          Number            `yaml:"oid_batch_size"`
	User                  string            `yaml:"user"`
	AuthProtocol          string            `yaml:"authProtocol"`
	AuthKey               string            `yaml:"authKey"`
	PrivProtocol          string            `yaml:"privProtocol"`
	PrivKey               string            `yaml:"privKey"`
	ContextName           string            `yaml:"context_name"`
	Metrics               []metricsConfig   `yaml:"metrics"`
	MetricTags            []metricTagConfig `yaml:"metric_tags"`
	Profile               string            `yaml:"profile"`
	UseGlobalMetrics      bool              `yaml:"use_global_metrics"`
	ExtraTags             string            `yaml:"extra_tags"` // comma separated tags
	Tags                  []string          `yaml:"tags"`       // used for device metadata
	CollectDeviceMetadata *Boolean          `yaml:"collect_device_metadata"`

	// `network` config is only available in Python SNMP integration
	// it's added here to raise warning if used with corecheck SNMP integration
	Network string `yaml:"network_address"`
}

type snmpConfig struct {
	ipAddress             string
	port                  uint16
	communityString       string
	snmpVersion           string
	timeout               int
	retries               int
	user                  string
	authProtocol          string
	authKey               string
	privProtocol          string
	privKey               string
	contextName           string
	oidConfig             oidConfig
	metrics               []metricsConfig
	metricTags            []metricTagConfig
	oidBatchSize          int
	profiles              profileDefinitionMap
	profileTags           []string
	profile               string
	profileDef            *profileDefinition
	extraTags             []string
	instanceTags          []string
	collectDeviceMetadata bool
	deviceID              string
	deviceIDTags          []string
	subnet                string
	autodetectProfile     bool
}

func (c *snmpConfig) refreshWithProfile(profile string) error {
	if _, ok := c.profiles[profile]; !ok {
		return fmt.Errorf("unknown profile `%s`", profile)
	}
	log.Debugf("Refreshing with profile `%s`", profile)
	tags := []string{"snmp_profile:" + profile}
	definition := c.profiles[profile]
	c.profileDef = &definition
	c.profile = profile

	c.metrics = append(c.metrics, definition.Metrics...)
	c.metricTags = append(c.metricTags, definition.MetricTags...)
	c.oidConfig.addScalarOids(parseScalarOids(definition.Metrics, definition.MetricTags))
	c.oidConfig.addColumnOids(parseColumnOids(definition.Metrics))

	if c.collectDeviceMetadata {
		c.oidConfig.addScalarOids(metadata.ScalarOIDs)
		c.oidConfig.addColumnOids(metadata.ColumnOIDs)
	}

	if definition.Device.Vendor != "" {
		tags = append(tags, "device_vendor:"+definition.Device.Vendor)
	}
	c.profileTags = tags
	return nil
}

func (c *snmpConfig) addUptimeMetric() {
	metricConfig := getUptimeMetricConfig()
	c.metrics = append(c.metrics, metricConfig)
	c.oidConfig.addScalarOids([]string{metricConfig.Symbol.OID})
}

// getStaticTags return static tags built from configuration
// warning: changing getStaticTags logic might lead to different deviceID
// getStaticTags does not contain tags from instance[].tags config
func (c *snmpConfig) getStaticTags() []string {
	tags := []string{"snmp_device:" + c.ipAddress}
	tags = append(tags, c.extraTags...)
	return tags
}

// getDeviceIDTags return sorted tags used for generating device id
// warning: changing getDeviceIDTags logic might lead to different deviceID
func (c *snmpConfig) getDeviceIDTags() []string {
	tags := c.getStaticTags()
	tags = append(tags, c.instanceTags...)
	sort.Strings(tags)
	return tags
}

// toString used for logging snmpConfig without sensitive information
func (c *snmpConfig) toString() string {
	return fmt.Sprintf("snmpConfig: ipAddress=`%s`, port=`%d`, snmpVersion=`%s`, timeout=`%d`, retries=`%d`, "+
		"user=`%s`, authProtocol=`%s`, privProtocol=`%s`, contextName=`%s`, oidConfig=`%#v`, "+
		"oidBatchSize=`%d`, profileTags=`%#v`",
		c.ipAddress,
		c.port,
		c.snmpVersion,
		c.timeout,
		c.retries,
		c.user,
		c.authProtocol,
		c.privProtocol,
		c.contextName,
		c.oidConfig,
		c.oidBatchSize,
		c.profileTags,
	)
}

func buildConfig(rawInstance integration.Data, rawInitConfig integration.Data) (snmpConfig, error) {
	instance := snmpInstanceConfig{}
	initConfig := snmpInitConfig{}

	// Set defaults before unmarshalling
	instance.UseGlobalMetrics = true

	err := yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return snmpConfig{}, err
	}

	err = yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return snmpConfig{}, err
	}

	c := snmpConfig{}

	c.snmpVersion = instance.SnmpVersion
	c.ipAddress = instance.IPAddress
	c.port = uint16(instance.Port)

	if instance.CollectDeviceMetadata != nil {
		c.collectDeviceMetadata = bool(*instance.CollectDeviceMetadata)
	} else {
		c.collectDeviceMetadata = bool(initConfig.CollectDeviceMetadata)
	}

	if instance.ExtraTags != "" {
		c.extraTags = strings.Split(instance.ExtraTags, ",")
	}

	if instance.Network != "" {
		log.Warnf("`network_address` config is not available for corecheck SNMP integration to use autodiscovery. Agent `snmp_listener` config can be used instead: https://docs.datadoghq.com/network_monitoring/devices/setup?tab=snmpv2#autodiscovery")
	}

	if c.ipAddress == "" {
		return snmpConfig{}, fmt.Errorf("ip_address config must be provided")
	}

	if c.port == 0 {
		c.port = defaultPort
	}

	if instance.Retries == 0 {
		c.retries = defaultRetries
	} else {
		c.retries = int(instance.Retries)
	}

	if instance.Timeout == 0 {
		c.timeout = defaultTimeout
	} else {
		c.timeout = int(instance.Timeout)
	}

	// SNMP connection configs
	c.communityString = instance.CommunityString
	c.user = instance.User
	c.authProtocol = instance.AuthProtocol
	c.authKey = instance.AuthKey
	c.privProtocol = instance.PrivProtocol
	c.privKey = instance.PrivKey
	c.contextName = instance.ContextName

	c.metrics = instance.Metrics

	if instance.OidBatchSize != 0 {
		c.oidBatchSize = int(instance.OidBatchSize)
	} else if initConfig.OidBatchSize != 0 {
		c.oidBatchSize = int(initConfig.OidBatchSize)
	} else {
		c.oidBatchSize = defaultOidBatchSize
	}

	// metrics Configs
	if instance.UseGlobalMetrics {
		c.metrics = append(c.metrics, initConfig.GlobalMetrics...)
	}
	normalizeMetrics(c.metrics)

	c.instanceTags = instance.Tags
	c.metricTags = instance.MetricTags

	c.oidConfig.addScalarOids(parseScalarOids(c.metrics, c.metricTags))
	c.oidConfig.addColumnOids(parseColumnOids(c.metrics))

	// Profile Configs
	var profiles profileDefinitionMap
	if len(initConfig.Profiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, err := loadProfiles(initConfig.Profiles)
		if err != nil {
			return snmpConfig{}, fmt.Errorf("failed to load custom profiles: %s", err)
		}
		profiles = customProfiles
	} else {
		defaultProfiles, err := loadDefaultProfiles()
		if err != nil {
			return snmpConfig{}, fmt.Errorf("failed to load default profiles: %s", err)
		}
		profiles = defaultProfiles
	}

	for _, profileDef := range profiles {
		normalizeMetrics(profileDef.Metrics)
	}

	c.profiles = profiles
	profile := instance.Profile

	errors := validateEnrichMetrics(c.metrics)
	errors = append(errors, validateEnrichMetricTags(c.metricTags)...)
	if len(errors) > 0 {
		return snmpConfig{}, fmt.Errorf("validation errors: %s", strings.Join(errors, "\n"))
	}

	if profile != "" || len(c.metrics) > 0 {
		c.autodetectProfile = false
	} else {
		c.autodetectProfile = true
	}

	if profile != "" {
		err = c.refreshWithProfile(profile)
		if err != nil {
			return snmpConfig{}, fmt.Errorf("failed to refresh with profile `%s`: %s", profile, err)
		}
	}

	c.deviceID, c.deviceIDTags = buildDeviceID(c.getDeviceIDTags())

	subnet, err := getSubnetFromTags(c.instanceTags)
	if err != nil {
		log.Debugf("subnet not found: %s", err)
	}
	c.subnet = subnet

	c.addUptimeMetric()
	return c, nil
}

func getUptimeMetricConfig() metricsConfig {
	// Reference sysUpTimeInstance directly, see http://oidref.com/1.3.6.1.2.1.1.3.0
	return metricsConfig{Symbol: symbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}
}

func parseScalarOids(metrics []metricsConfig, metricTags []metricTagConfig) []string {
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

func parseColumnOids(metrics []metricsConfig) []string {
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

func getProfileForSysObjectID(profiles profileDefinitionMap, sysObjectID string) (string, error) {
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
