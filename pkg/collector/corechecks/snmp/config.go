package snmp

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"hash/fnv"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Using high oid batch size might lead to snmp calls timing out.
// For some devices, the default oid_batch_size of 5 might be high (leads to timeouts),
// and require manual setting oid_batch_size to a lower value.
var defaultOidBatchSize = 5

var defaultPort = uint16(161)
var defaultRetries = 3
var defaultTimeout = 2
var defaultWorkers = 10
var defaultDiscoveryWorkers = 10
var defaultDiscoveryAllowedFailures = 3
var defaultDiscoveryInterval = 3600
var subnetTagPrefix = "autodiscovery_subnet"

// Using too high max repetitions might lead to tooBig SNMP error messages.
// - Java SNMP and gosnmp (gosnmp.defaultMaxRepetitions) uses 50
// - snmp-net uses 10
const defaultBulkMaxRepetitions = uint32(10)

type snmpInitConfig struct {
	Profiles              profileConfigMap `yaml:"profiles"`
	GlobalMetrics         []metricsConfig  `yaml:"global_metrics"`
	OidBatchSize          Number           `yaml:"oid_batch_size"`
	BulkMaxRepetitions    Number           `yaml:"bulk_max_repetitions"`
	CollectDeviceMetadata Boolean          `yaml:"collect_device_metadata"`
	MinCollectionInterval int              `yaml:"min_collection_interval"`
}

type snmpInstanceConfig struct {
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
	Metrics               []metricsConfig   `yaml:"metrics"`
	MetricTags            []metricTagConfig `yaml:"metric_tags"`
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
	TestInstances            int      `yaml:"test_instances"`
	Network                  string   `yaml:"network_address"`
	IgnoredIPAddresses       []string `yaml:"ignored_ip_addresses"`
	DiscoveryInterval        int      `yaml:"discovery_interval"`
	DiscoveryAllowedFailures int      `yaml:"discovery_allowed_failures"`
	DiscoveryWorkers         int      `yaml:"discovery_workers"`
	Workers                  int      `yaml:"workers"`
}

type snmpConfig struct {

	// static profile configs
	profiles profileDefinitionMap

	// [device] settings
	session sessionAPI

	// [device] dynamic configs (for autodiscovery)
	ipAddress    string
	deviceID     string   // TODO: move to deviceCheck
	deviceIDTags []string // TODO: move to deviceCheck

	// [device] static configs
	port            uint16
	communityString string
	snmpVersion     string
	timeout         int
	retries         int
	user            string
	authProtocol    string
	authKey         string
	privProtocol    string
	privKey         string
	contextName     string

	// [device] dynamic profile configs (can be changed with profile refresh)
	autodetectProfile bool               // TODO: move to deviceCheck
	oidConfig         oidConfig          // TODO: move to deviceCheck
	metrics           []metricsConfig    // TODO: move to deviceCheck
	metricTags        []metricTagConfig  // TODO: move to deviceCheck
	profileTags       []string           // tags created from current profile // TODO: move to deviceCheck
	profile           string             // TODO: move to deviceCheck
	profileDef        *profileDefinition // TODO: move to deviceCheck

	// [integration instance] performance settings
	oidBatchSize       int
	bulkMaxRepetitions uint32

	// [integration instance] static settings
	extraTags             []string
	instanceTags          []string
	collectDeviceMetadata bool
	minCollectionInterval time.Duration

	// [integration instance] discovery settings
	subnet                   string
	network                  string
	discoveryWorkers         int
	workers                  int
	discoveryInterval        int
	ignoredIPAddresses       map[string]bool
	discoveryAllowedFailures int
	testInstances            int // TODO: remove me

	// [integration instance] dynamic settings
	sender metricSender // is recreated at each checkrun
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

// Digest returns an hash value representing the data stored in this configuration, minus the network address
func (c *snmpConfig) Digest(address string) string {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(address))                   //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%d", c.port))) //nolint:errcheck
	h.Write([]byte(c.snmpVersion))             //nolint:errcheck
	h.Write([]byte(c.communityString))         //nolint:errcheck
	h.Write([]byte(c.user))                    //nolint:errcheck
	h.Write([]byte(c.authKey))                 //nolint:errcheck
	h.Write([]byte(c.authProtocol))            //nolint:errcheck
	h.Write([]byte(c.privKey))                 //nolint:errcheck
	h.Write([]byte(c.privProtocol))            //nolint:errcheck
	//h.Write([]byte(c.ContextEngineID))         //nolint:errcheck
	h.Write([]byte(c.contextName)) //nolint:errcheck
	//h.Write([]byte(c.loader))                  //nolint:errcheck

	// Sort the addresses to get a stable digest
	addresses := make([]string, 0, len(c.ignoredIPAddresses))
	for ip := range c.ignoredIPAddresses {
		addresses = append(addresses, ip)
	}
	sort.Strings(addresses)
	for _, ip := range addresses {
		h.Write([]byte(ip)) //nolint:errcheck
	}

	return strconv.FormatUint(h.Sum64(), 16)
}

// IsIPIgnored checks the given IP against ignoredIPAddresses
func (c *snmpConfig) IsIPIgnored(ip net.IP) bool {
	ipString := ip.String()
	_, present := c.ignoredIPAddresses[ipString]
	return present
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

func (c *snmpConfig) Copy() *snmpConfig {
	// TODO: use a separate config for things that do not change e.g. profiles
	// TODO: convert to deviceConfig instead
	newConfig := snmpConfig{}
	newConfig.ipAddress = c.ipAddress
	newConfig.port = c.port
	newConfig.communityString = c.communityString
	newConfig.snmpVersion = c.snmpVersion
	newConfig.timeout = c.timeout
	newConfig.retries = c.retries
	newConfig.user = c.user
	newConfig.authProtocol = c.authProtocol
	newConfig.authKey = c.authKey
	newConfig.privProtocol = c.privProtocol
	newConfig.privKey = c.privKey
	newConfig.contextName = c.contextName
	newConfig.contextName = c.contextName
	newConfig.oidConfig = c.oidConfig
	newConfig.metrics = make([]metricsConfig, len(c.metrics))
	for _, metric := range c.metrics {
		newConfig.metrics = append(newConfig.metrics, metric)
	}
	newConfig.metricTags = make([]metricTagConfig, len(c.metricTags))
	for _, metricTag := range c.metricTags {
		newConfig.metricTags = append(newConfig.metricTags, metricTag)
	}
	newConfig.oidBatchSize = c.oidBatchSize
	newConfig.bulkMaxRepetitions = c.bulkMaxRepetitions
	newConfig.profiles = c.profiles
	newConfig.profileTags = copyStrings(c.profileTags)
	newConfig.profile = c.profile       // TODO: does not change
	newConfig.profileDef = c.profileDef // copy by ref, content is never changed
	newConfig.extraTags = copyStrings(c.extraTags)
	newConfig.instanceTags = copyStrings(c.instanceTags)
	newConfig.collectDeviceMetadata = c.collectDeviceMetadata
	newConfig.deviceID = c.deviceID

	newConfig.deviceIDTags = copyStrings(c.deviceIDTags)
	newConfig.subnet = c.subnet
	newConfig.autodetectProfile = c.autodetectProfile
	newConfig.minCollectionInterval = c.minCollectionInterval

	// TODO: no need to copy network config for device config
	newConfig.network = c.network
	newConfig.discoveryWorkers = c.discoveryWorkers
	newConfig.workers = c.workers
	newConfig.discoveryInterval = c.discoveryInterval
	newConfig.ignoredIPAddresses = c.ignoredIPAddresses
	newConfig.discoveryAllowedFailures = c.discoveryAllowedFailures
	newConfig.testInstances = c.testInstances
	newConfig.sender = c.sender
	return &newConfig
}

func buildConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*snmpConfig, error) {
	instance := snmpInstanceConfig{}
	initConfig := snmpInitConfig{}

	// Set defaults before unmarshalling
	instance.UseGlobalMetrics = true

	err := yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return nil, err
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

	//if instance.network != "" {
	//	log.Warnf("`network_address` config is not available for corecheck SNMP integration to use autodiscovery. Agent `snmp_listener` config can be used instead: https://docs.datadoghq.com/network_monitoring/devices/setup?tab=snmpv2#autodiscovery")
	//
	//}

	c.network = instance.Network
	if instance.DiscoveryWorkers == 0 {
		c.discoveryWorkers = defaultDiscoveryWorkers
	} else {
		c.discoveryWorkers = instance.DiscoveryWorkers
	}

	if instance.Workers == 0 {
		c.workers = defaultWorkers
	} else {
		c.workers = instance.Workers
	}

	if instance.DiscoveryAllowedFailures == 0 {
		c.discoveryAllowedFailures = defaultDiscoveryAllowedFailures
	} else {
		c.discoveryAllowedFailures = instance.DiscoveryAllowedFailures
	}

	if instance.DiscoveryInterval == 0 {
		c.discoveryInterval = defaultDiscoveryInterval
	} else {
		c.discoveryInterval = instance.DiscoveryInterval
	}

	// TODO: test me
	c.ignoredIPAddresses = make(map[string]bool, len(instance.IgnoredIPAddresses))
	for _, ipAddress := range instance.IgnoredIPAddresses {
		c.ignoredIPAddresses[ipAddress] = true
	}

	c.testInstances = instance.TestInstances

	if c.ipAddress == "" && c.network == "" {
		// TODO: TEST ME
		return nil, fmt.Errorf("ip_address or network config must be provided")
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

	if instance.ExtraMinCollectionInterval != 0 {
		c.minCollectionInterval = time.Duration(instance.ExtraMinCollectionInterval) * time.Second
	} else if instance.MinCollectionInterval != 0 {
		c.minCollectionInterval = time.Duration(instance.MinCollectionInterval) * time.Second
	} else if initConfig.MinCollectionInterval != 0 {
		c.minCollectionInterval = time.Duration(initConfig.MinCollectionInterval) * time.Second
	} else {
		c.minCollectionInterval = defaults.DefaultCheckInterval
	}
	if c.minCollectionInterval < 0 {
		return nil, fmt.Errorf("min collection interval must be > 0, but got: %v", c.minCollectionInterval.Seconds())
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

	var bulkMaxRepetitions int
	if instance.BulkMaxRepetitions != 0 {
		bulkMaxRepetitions = int(instance.BulkMaxRepetitions)
	} else if initConfig.BulkMaxRepetitions != 0 {
		bulkMaxRepetitions = int(initConfig.BulkMaxRepetitions)
	} else {
		bulkMaxRepetitions = int(defaultBulkMaxRepetitions)
	}
	if bulkMaxRepetitions <= 0 {
		return nil, fmt.Errorf("bulk max repetition must be a positive integer. Invalid value: %d", bulkMaxRepetitions)
	}
	c.bulkMaxRepetitions = uint32(bulkMaxRepetitions)

	// metrics Configs
	if instance.UseGlobalMetrics {
		c.metrics = append(c.metrics, initConfig.GlobalMetrics...)
	}
	normalizeMetrics(c.metrics)

	c.instanceTags = instance.Tags
	c.metricTags = instance.MetricTags

	c.oidConfig.addScalarOids(parseScalarOids(c.metrics, c.metricTags))
	c.oidConfig.addColumnOids(parseColumnOids(c.metrics))

	if c.collectDeviceMetadata {
		c.oidConfig.addScalarOids(metadata.ScalarOIDs)
		c.oidConfig.addColumnOids(metadata.ColumnOIDs)
	}

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

	c.profiles = profiles
	profile := instance.Profile

	errors := validateEnrichMetrics(c.metrics)
	errors = append(errors, validateEnrichMetricTags(c.metricTags)...)
	if len(errors) > 0 {
		return nil, fmt.Errorf("validation errors: %s", strings.Join(errors, "\n"))
	}

	if profile != "" || len(c.metrics) > 0 {
		c.autodetectProfile = false
	} else {
		c.autodetectProfile = true
	}

	if profile != "" {
		err = c.refreshWithProfile(profile)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh with profile `%s`: %s", profile, err)
		}
	}

	c.deviceID, c.deviceIDTags = buildDeviceID(c.getDeviceIDTags())

	subnet, err := getSubnetFromTags(c.instanceTags)
	if err != nil {
		log.Debugf("subnet not found: %s", err)
	}
	c.subnet = subnet

	c.addUptimeMetric()

	c.session = &snmpSession{}
	// TODO: also configure session ?
	return &c, nil
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
