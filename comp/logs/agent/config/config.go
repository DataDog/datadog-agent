// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerCollectAll is the name of the docker integration that collect logs from all containers
const ContainerCollectAll = "container_collect_all"

// logs-intake endpoint prefix.
const (
	tcpEndpointPrefix            = "agent-intake.logs."
	httpEndpointPrefix           = "agent-http-intake.logs."
	serverlessHTTPEndpointPrefix = "http-intake.logs."
)

// AgentJSONIntakeProtocol agent json protocol
const AgentJSONIntakeProtocol = "agent-json"

// DefaultIntakeProtocol indicates that no special protocol is in use for the endpoint intake track type.
const DefaultIntakeProtocol IntakeProtocol = ""

// DefaultIntakeOrigin indicates that no special DD_SOURCE header is in use for the endpoint intake track type.
const DefaultIntakeOrigin IntakeOrigin = "agent"

// ServerlessIntakeOrigin is the lambda extension origin
const ServerlessIntakeOrigin IntakeOrigin = "lambda-extension"

// logs-intake endpoints depending on the site and environment.
var logsEndpoints = map[string]int{
	"agent-intake.logs.datadoghq.com": 10516,
	"agent-intake.logs.datadoghq.eu":  443,
	"agent-intake.logs.datad0g.com":   10516,
	"agent-intake.logs.datad0g.eu":    443,
}

// regex for URL with scheme
var urlWithScheme = regexp.MustCompile(`^([\w]+:)?//`)

// HTTPConnectivity is the status of the HTTP connectivity
type HTTPConnectivity bool

var (
	// HTTPConnectivitySuccess is the status for successful HTTP connectivity
	HTTPConnectivitySuccess HTTPConnectivity = true
	// HTTPConnectivityFailure is the status for failed HTTP connectivity
	HTTPConnectivityFailure HTTPConnectivity = false
)

// GlobalProcessingRules returns the global processing rules to apply to all logs.
func GlobalProcessingRules(coreConfig pkgconfigmodel.Reader) ([]*ProcessingRule, error) {
	var rules []*ProcessingRule
	var err error
	raw := coreConfig.Get("logs_config.processing_rules")
	if raw == nil {
		return rules, nil
	}
	if s, ok := raw.(string); ok && s != "" {
		err = json.Unmarshal([]byte(s), &rules)
	} else {
		err = structure.UnmarshalKey(coreConfig, "logs_config.processing_rules", &rules, structure.ConvertEmptyStringToNil)
	}
	if err != nil {
		return nil, err
	}
	err = ValidateProcessingRules(rules)
	if err != nil {
		return nil, err
	}
	err = CompileProcessingRules(rules)
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// HasMultiLineRule returns true if the rule set contains a multi_line rule
func HasMultiLineRule(rules []*ProcessingRule) bool {
	for _, rule := range rules {
		if rule.Type == MultiLine {
			return true
		}
	}
	return false
}

// BuildEndpoints returns the endpoints to send logs.
func BuildEndpoints(coreConfig pkgconfigmodel.Reader, httpConnectivity HTTPConnectivity, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	return BuildEndpointsWithConfig(coreConfig, defaultLogsConfigKeys(coreConfig), httpEndpointPrefix, httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildEndpointsWithVectorOverride returns the endpoints to send logs and enforce Vector override config keys
func BuildEndpointsWithVectorOverride(coreConfig pkgconfigmodel.Reader, httpConnectivity HTTPConnectivity, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	return BuildEndpointsWithConfig(coreConfig, defaultLogsConfigKeysWithVectorOverride(coreConfig), httpEndpointPrefix, httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildEndpointsWithConfig returns the endpoints to send logs.
func BuildEndpointsWithConfig(coreConfig pkgconfigmodel.Reader, logsConfig *LogsConfigKeys, endpointPrefix string, httpConnectivity HTTPConnectivity, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	if logsConfig.devModeNoSSL() {
		log.Warnf("Use of illegal configuration parameter, if you need to send your logs to a proxy, "+
			"please use '%s' and '%s' instead", logsConfig.getConfigKey("logs_dd_url"), logsConfig.getConfigKey("logs_no_ssl"))
	}

	mrfEnabled := coreConfig.GetBool("multi_region_failover.enabled")
	if logsConfig.isForceHTTPUse() || logsConfig.obsPipelineWorkerEnabled() || mrfEnabled || (bool(httpConnectivity) && !(logsConfig.isForceTCPUse() || logsConfig.isSocks5ProxySet() || logsConfig.hasAdditionalEndpoints())) {
		return BuildHTTPEndpointsWithConfig(coreConfig, logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	}
	log.Warnf("You are currently sending Logs to Datadog through TCP (either because %s or %s is set or the HTTP connectivity test has failed) "+
		"To benefit from increased reliability and better network performances, "+
		"we strongly encourage switching over to compressed HTTPS which is now the default protocol.",
		logsConfig.getConfigKey("force_use_tcp"), logsConfig.getConfigKey("socks5_proxy_address"))
	return buildTCPEndpoints(coreConfig, logsConfig)
}

// BuildServerlessEndpoints returns the endpoints to send logs for the Serverless agent.
func BuildServerlessEndpoints(coreConfig pkgconfigmodel.Reader, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol) (*Endpoints, error) {
	return BuildHTTPEndpointsWithConfig(coreConfig, defaultLogsConfigKeysWithVectorOverride(coreConfig), serverlessHTTPEndpointPrefix, intakeTrackType, intakeProtocol, ServerlessIntakeOrigin)
}

// ExpectedTagsDuration returns a duration of the time expected tags will be submitted for.
func ExpectedTagsDuration(coreConfig pkgconfigmodel.Reader) time.Duration {
	return defaultLogsConfigKeys(coreConfig).expectedTagsDuration()
}

// IsExpectedTagsSet returns boolean showing if expected tags feature is enabled.
func IsExpectedTagsSet(coreConfig pkgconfigmodel.Reader) bool {
	return ExpectedTagsDuration(coreConfig) > 0
}

func buildTCPEndpoints(coreConfig pkgconfigmodel.Reader, logsConfig *LogsConfigKeys) (*Endpoints, error) {
	useProto := logsConfig.devModeUseProto()
	main := NewTCPEndpoint(logsConfig)

	if logsDDURL, defined := logsConfig.logsDDURL(); defined {
		// Proxy settings, expect 'logs_config.logs_dd_url' to respect the format '<HOST>:<PORT>'
		// and '<PORT>' to be an integer.
		// By default ssl is enabled ; to disable ssl set 'logs_config.logs_no_ssl' to true.
		err := parseAddress(logsDDURL, &main, logsConfig.logsNoSSL(), true)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}
	} else if logsConfig.usePort443() {
		main.Host = logsConfig.ddURL443()
		main.Port = 443
		main.useSSL = true
	} else {
		// If no proxy is set, we default to 'logs_config.dd_url' if set, or to 'site'.
		// if none of them is set, we default to the US agent endpoint.
		main.Host = pkgconfigutils.GetMainEndpoint(coreConfig, tcpEndpointPrefix, logsConfig.getConfigKey("dd_url"))
		if port, found := logsEndpoints[main.Host]; found {
			main.Port = port
		} else {
			main.Port = logsConfig.ddPort()
		}
		main.useSSL = !logsConfig.devModeNoSSL()
	}

	additionals := loadTCPAdditionalEndpoints(main, logsConfig)
	return NewEndpoints(main, additionals, useProto, false), nil
}

// BuildHTTPEndpoints returns the HTTP endpoints to send logs to.
func BuildHTTPEndpoints(coreConfig pkgconfigmodel.Reader, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	return BuildHTTPEndpointsWithConfig(coreConfig, defaultLogsConfigKeys(coreConfig), httpEndpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildHTTPEndpointsWithVectorOverride returns the HTTP endpoints to send logs to.
func BuildHTTPEndpointsWithVectorOverride(coreConfig pkgconfigmodel.Reader, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	return BuildHTTPEndpointsWithConfig(coreConfig, defaultLogsConfigKeysWithVectorOverride(coreConfig), httpEndpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildHTTPEndpointsWithConfig uses two arguments that instructs it how to access configuration parameters, then returns the HTTP endpoints to send logs to. This function is able to default to the 'classic' BuildHTTPEndpoints() w ldHTTPEndpointsWithConfigdefault variables logsConfigDefaultKeys and httpEndpointPrefix
func BuildHTTPEndpointsWithConfig(coreConfig pkgconfigmodel.Reader, logsConfig *LogsConfigKeys, endpointPrefix string, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	// Provide default values for legacy settings when the configuration key does not exist
	defaultNoSSL := logsConfig.logsNoSSL()

	main := NewHTTPEndpoint(logsConfig)

	if logsConfig.useV2API() && intakeTrackType != "" {
		main.Version = EPIntakeVersion2
		main.TrackType = intakeTrackType
		main.Protocol = intakeProtocol
		main.Origin = intakeOrigin
	} else {
		main.Version = EPIntakeVersion1
	}

	if vectorURL, vectorURLDefined := logsConfig.getObsPipelineURL(); logsConfig.obsPipelineWorkerEnabled() && vectorURLDefined {
		err := parseAddress(vectorURL, &main, defaultNoSSL, false)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", vectorURL, err)
		}
	} else if logsDDURL, logsDDURLDefined := logsConfig.logsDDURL(); logsDDURLDefined {
		err := parseAddress(logsDDURL, &main, defaultNoSSL, true)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}
	} else {
		addr := pkgconfigutils.GetMainEndpoint(coreConfig, endpointPrefix, logsConfig.getConfigKey("dd_url"))
		err := parseAddress(addr, &main, logsConfig.devModeNoSSL(), false)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}
	}

	additionals := loadHTTPAdditionalEndpoints(main, logsConfig, intakeTrackType, intakeProtocol, intakeOrigin)

	// Add in the MRF endpoint if MRF is enabled.
	if coreConfig.GetBool("multi_region_failover.enabled") {
		mrfURL, err := pkgconfigutils.GetMRFEndpoint(coreConfig, endpointPrefix, "multi_region_failover.dd_url")
		if err != nil {
			return nil, fmt.Errorf("cannot construct MRF endpoint: %s", err)
		}

		var endpoint Endpoint
		err := parseAddress(mrfURL, &endpoint, defaultNoSSL, false)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", mrfURL, err)
		}

		additionals = append(additionals, Endpoint{
			IsMRF:            true,
			APIKey:           coreConfig.GetString("multi_region_failover.api_key"),
			Host:             endpoint.Host,
			Port:             endpoint.Port,
			useSSL:           endpoint.useSSL,
			UseCompression:   main.UseCompression,
			CompressionLevel: main.CompressionLevel,
			BackoffBase:      main.BackoffBase,
			BackoffMax:       main.BackoffMax,
			BackoffFactor:    main.BackoffFactor,
			RecoveryInterval: main.RecoveryInterval,
			RecoveryReset:    main.RecoveryReset,
			Version:          main.Version,
			TrackType:        intakeTrackType,
			Protocol:         intakeProtocol,
			Origin:           intakeOrigin,
		})
	}

	batchWait := logsConfig.batchWait()
	batchMaxConcurrentSend := logsConfig.batchMaxConcurrentSend()
	batchMaxSize := logsConfig.batchMaxSize()
	batchMaxContentSize := logsConfig.batchMaxContentSize()
	inputChanSize := logsConfig.inputChanSize()

	return NewEndpointsWithBatchSettings(main, additionals, false, true, batchWait, batchMaxConcurrentSend, batchMaxSize, batchMaxContentSize, inputChanSize), nil
}

func parseAddress(address string, endpoint *Endpoint, defaultNoSSL, requirePort bool) (err error) {
	endpoint.useSSL = !defaultNoSSL
	testAddr := address
	if len(urlWithScheme.Find([]byte(address))) == 0 {
		testAddr = "//" + testAddr
	}
	u, err := url.Parse(testAddr)
	if err != nil {
		return
	}
	switch u.Scheme {
	case "https":
		if !defaultNoSSL {
			log.Warn("dd_url set to a URL with an HTTPS prefix and logs_no_ssl set to true. These are conflicting options. In a future release logs_no_ssl will override the dd_url prefix.")
		}
		if u.Port() == "" && requirePort {
			endpoint.Port = 443
		}
		endpoint.useSSL = true
	case "http":
		endpoint.useSSL = false
		if u.Port() == "" && requirePort {
			endpoint.Port = 80
		}
	}
	endpoint.Prefix = u.Path
	endpoint.Host = u.Hostname()
	if u.Port() != "" {
		endpoint.Port, err = strconv.Atoi(u.Port())
		if err != nil {
			return
		}
	} else if requirePort {
		err = fmt.Errorf("either scheme or port should be set in %q", address)
	}
	return
}

// TaggerWarmupDuration is used to configure the tag providers
func TaggerWarmupDuration(coreConfig pkgconfigmodel.Reader) time.Duration {
	return defaultLogsConfigKeys(coreConfig).taggerWarmupDuration()
}

// AggregationTimeout is used when performing aggregation operations
func AggregationTimeout(coreConfig pkgconfigmodel.Reader) time.Duration {
	return defaultLogsConfigKeys(coreConfig).aggregationTimeout()
}

// MaxMessageSizeBytes is used to cap the maximum log message size in bytes
func MaxMessageSizeBytes(coreConfig pkgconfigmodel.Reader) int {
	return defaultLogsConfigKeys(coreConfig).maxMessageSizeBytes()
}
