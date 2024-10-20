// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
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
		host, port, err := parseAddress(logsDDURL)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}
		main.Host = host
		main.Port = port
		main.useSSL = !logsConfig.logsNoSSL()
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
		host, port, useSSL, err := parseAddressWithScheme(vectorURL, defaultNoSSL, parseAddress)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", vectorURL, err)
		}
		main.Host = host
		main.Port = port
		main.useSSL = useSSL
	} else if logsDDURL, logsDDURLDefined := logsConfig.logsDDURL(); logsDDURLDefined {
		host, port, useSSL, err := parseAddressWithScheme(logsDDURL, defaultNoSSL, parseAddress)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}
		main.Host = host
		main.Port = port
		main.useSSL = useSSL
	} else {
		addr := pkgconfigutils.GetMainEndpoint(coreConfig, endpointPrefix, logsConfig.getConfigKey("dd_url"))
		host, port, useSSL, err := parseAddressWithScheme(addr, logsConfig.devModeNoSSL(), parseAddressAsHost)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}

		main.Host = host
		main.Port = port
		main.useSSL = useSSL
	}

	additionals := loadHTTPAdditionalEndpoints(main, logsConfig, intakeTrackType, intakeProtocol, intakeOrigin)

	// Add in the MRF endpoint if MRF is enabled.
	if coreConfig.GetBool("multi_region_failover.enabled") {
		mrfURL, err := pkgconfigutils.GetMRFLogsEndpoint(coreConfig, endpointPrefix)
		if err != nil {
			return nil, fmt.Errorf("cannot construct MRF endpoint: %s", err)
		}

		mrfHost, mrfPort, mrfUseSSL, err := parseAddressWithScheme(mrfURL, defaultNoSSL, parseAddressAsHost)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", mrfURL, err)
		}

		e := NewEndpoint(coreConfig.GetString("multi_region_failover.api_key"), mrfHost, mrfPort, mrfUseSSL)
		e.IsMRF = true
		e.UseCompression = main.UseCompression
		e.CompressionLevel = main.CompressionLevel
		e.BackoffBase = main.BackoffBase
		e.BackoffMax = main.BackoffMax
		e.BackoffFactor = main.BackoffFactor
		e.RecoveryInterval = main.RecoveryInterval
		e.RecoveryReset = main.RecoveryReset
		e.Version = main.Version
		e.TrackType = intakeTrackType
		e.Protocol = intakeProtocol
		e.Origin = intakeOrigin

		additionals = append(additionals, e)
	}

	batchWait := logsConfig.batchWait()
	batchMaxConcurrentSend := logsConfig.batchMaxConcurrentSend()
	batchMaxSize := logsConfig.batchMaxSize()
	batchMaxContentSize := logsConfig.batchMaxContentSize()
	inputChanSize := logsConfig.inputChanSize()

	return NewEndpointsWithBatchSettings(main, additionals, false, true, batchWait, batchMaxConcurrentSend, batchMaxSize, batchMaxContentSize, inputChanSize), nil
}

type defaultParseAddressFunc func(string) (host string, port int, err error)

func parseAddressWithScheme(address string, defaultNoSSL bool, defaultParser defaultParseAddressFunc) (host string, port int, useSSL bool, err error) {
	if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "http://") {
		if strings.HasPrefix(address, "https://") && !defaultNoSSL {
			log.Warn("dd_url set to a URL with an HTTPS prefix and logs_no_ssl set to true. These are conflicting options. In a future release logs_no_ssl will override the dd_url prefix.")
		}
		host, port, useSSL, err = parseURL(address)
	} else {
		host, port, err = defaultParser(address)
		if err != nil {
			err = fmt.Errorf("could not parse %s: %v", address, err)
			return
		}
		useSSL = !defaultNoSSL
	}
	return
}

func parseURL(address string) (host string, port int, useSSL bool, err error) {
	u, errParse := url.Parse(address)
	if errParse != nil {
		err = errParse
		return
	}
	switch u.Scheme {
	case "https":
		useSSL = true
	case "http":
		useSSL = false
	}
	host = u.Hostname()
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil {
			return
		}
	}

	return
}

// parseAddress returns the host and the port of the address.
func parseAddress(address string) (string, int, error) {
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// parseAddressAsHost returns the host and the port of the address.
// this function consider that the address is the host
func parseAddressAsHost(address string) (string, int, error) {
	return address, 0, nil
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
