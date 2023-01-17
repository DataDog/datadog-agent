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

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
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
func GlobalProcessingRules() ([]*ProcessingRule, error) {
	var rules []*ProcessingRule
	var err error
	raw := coreConfig.Datadog.Get("logs_config.processing_rules")
	if raw == nil {
		return rules, nil
	}
	if s, ok := raw.(string); ok && s != "" {
		err = json.Unmarshal([]byte(s), &rules)
	} else {
		err = coreConfig.Datadog.UnmarshalKey("logs_config.processing_rules", &rules)
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
func BuildEndpoints(httpConnectivity HTTPConnectivity, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	coreConfig.SanitizeAPIKeyConfig(coreConfig.Datadog, "logs_config.api_key")
	return BuildEndpointsWithConfig(defaultLogsConfigKeys(), httpEndpointPrefix, httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildEndpointsWithVectorOverride returns the endpoints to send logs and enforce Vector override config keys
func BuildEndpointsWithVectorOverride(httpConnectivity HTTPConnectivity, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	coreConfig.SanitizeAPIKeyConfig(coreConfig.Datadog, "logs_config.api_key")
	return BuildEndpointsWithConfig(defaultLogsConfigKeysWithVectorOverride(), httpEndpointPrefix, httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildEndpointsWithConfig returns the endpoints to send logs.
func BuildEndpointsWithConfig(logsConfig *LogsConfigKeys, endpointPrefix string, httpConnectivity HTTPConnectivity, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	if logsConfig.devModeNoSSL() {
		log.Warnf("Use of illegal configuration parameter, if you need to send your logs to a proxy, "+
			"please use '%s' and '%s' instead", logsConfig.getConfigKey("logs_dd_url"), logsConfig.getConfigKey("logs_no_ssl"))
	}
	if logsConfig.isForceHTTPUse() || logsConfig.vectorEnabled() || (bool(httpConnectivity) && !(logsConfig.isForceTCPUse() || logsConfig.isSocks5ProxySet() || logsConfig.hasAdditionalEndpoints())) {
		return BuildHTTPEndpointsWithConfig(logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	}
	log.Warnf("You are currently sending Logs to Datadog through TCP (either because %s or %s is set or the HTTP connectivity test has failed) "+
		"To benefit from increased reliability and better network performances, "+
		"we strongly encourage switching over to compressed HTTPS which is now the default protocol.",
		logsConfig.getConfigKey("force_use_tcp"), logsConfig.getConfigKey("socks5_proxy_address"))
	return buildTCPEndpoints(logsConfig)
}

// BuildServerlessEndpoints returns the endpoints to send logs for the Serverless agent.
func BuildServerlessEndpoints(intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol) (*Endpoints, error) {
	coreConfig.SanitizeAPIKeyConfig(coreConfig.Datadog, "logs_config.api_key")
	return BuildHTTPEndpointsWithConfig(defaultLogsConfigKeys(), serverlessHTTPEndpointPrefix, intakeTrackType, intakeProtocol, ServerlessIntakeOrigin)
}

// ExpectedTagsDuration returns a duration of the time expected tags will be submitted for.
func ExpectedTagsDuration() time.Duration {
	return defaultLogsConfigKeys().expectedTagsDuration()
}

// IsExpectedTagsSet returns boolean showing if expected tags feature is enabled.
func IsExpectedTagsSet() bool {
	return ExpectedTagsDuration() > 0
}

func buildTCPEndpoints(logsConfig *LogsConfigKeys) (*Endpoints, error) {
	useProto := logsConfig.devModeUseProto()
	proxyAddress := logsConfig.socks5ProxyAddress()
	main := Endpoint{
		APIKey:                  logsConfig.getLogsAPIKey(),
		ProxyAddress:            proxyAddress,
		ConnectionResetInterval: logsConfig.connectionResetInterval(),
	}

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
		main.UseSSL = !logsConfig.logsNoSSL()
	} else if logsConfig.usePort443() {
		main.Host = logsConfig.ddURL443()
		main.Port = 443
		main.UseSSL = true
	} else {
		// If no proxy is set, we default to 'logs_config.dd_url' if set, or to 'site'.
		// if none of them is set, we default to the US agent endpoint.
		main.Host = coreConfig.GetMainEndpoint(tcpEndpointPrefix, logsConfig.getConfigKey("dd_url"))
		if port, found := logsEndpoints[main.Host]; found {
			main.Port = port
		} else {
			main.Port = logsConfig.ddPort()
		}
		main.UseSSL = !logsConfig.devModeNoSSL()
	}

	additionals := logsConfig.getAdditionalEndpoints()
	for i := 0; i < len(additionals); i++ {
		additionals[i].UseSSL = main.UseSSL
		additionals[i].ProxyAddress = proxyAddress
		additionals[i].APIKey = coreConfig.SanitizeAPIKey(additionals[i].APIKey)
	}
	return NewEndpoints(main, additionals, useProto, false), nil
}

// BuildHTTPEndpoints returns the HTTP endpoints to send logs to.
func BuildHTTPEndpoints(intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	return BuildHTTPEndpointsWithConfig(defaultLogsConfigKeys(), httpEndpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildHTTPEndpointsWithVectorOverride returns the HTTP endpoints to send logs to.
func BuildHTTPEndpointsWithVectorOverride(intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	return BuildHTTPEndpointsWithConfig(defaultLogsConfigKeysWithVectorOverride(), httpEndpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildHTTPEndpointsWithConfig uses two arguments that instructs it how to access configuration parameters, then returns the HTTP endpoints to send logs to. This function is able to default to the 'classic' BuildHTTPEndpoints() w ldHTTPEndpointsWithConfigdefault variables logsConfigDefaultKeys and httpEndpointPrefix
func BuildHTTPEndpointsWithConfig(logsConfig *LogsConfigKeys, endpointPrefix string, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) (*Endpoints, error) {
	// Provide default values for legacy settings when the configuration key does not exist
	defaultNoSSL := logsConfig.logsNoSSL()

	main := Endpoint{
		APIKey:                  logsConfig.getLogsAPIKey(),
		UseCompression:          logsConfig.useCompression(),
		CompressionLevel:        logsConfig.compressionLevel(),
		ConnectionResetInterval: logsConfig.connectionResetInterval(),
		BackoffBase:             logsConfig.senderBackoffBase(),
		BackoffMax:              logsConfig.senderBackoffMax(),
		BackoffFactor:           logsConfig.senderBackoffFactor(),
		RecoveryInterval:        logsConfig.senderRecoveryInterval(),
		RecoveryReset:           logsConfig.senderRecoveryReset(),
	}

	if logsConfig.useV2API() && intakeTrackType != "" {
		main.Version = EPIntakeVersion2
		main.TrackType = intakeTrackType
		main.Protocol = intakeProtocol
		main.Origin = intakeOrigin
	} else {
		main.Version = EPIntakeVersion1
	}

	if vectorURL, vectorURLDefined := logsConfig.getVectorURL(); logsConfig.vectorEnabled() && vectorURLDefined {
		if strings.HasPrefix(vectorURL, "https://") || strings.HasPrefix(vectorURL, "http://") {
			u, err := url.Parse(vectorURL)
			if err != nil {
				return nil, fmt.Errorf("could not parse %s: %v", vectorURL, err)
			}
			switch u.Scheme {
			case "https":
				main.UseSSL = true
			case "http":
				main.UseSSL = false
			}
			main.Host = u.Hostname()
			if u.Port() != "" {
				port, err := strconv.Atoi(u.Port())
				if err != nil {
					return nil, fmt.Errorf("could not parse %s: %v", vectorURL, err)
				}
				main.Port = port
			}
		} else {
			host, port, err := parseAddress(vectorURL)
			if err != nil {
				return nil, fmt.Errorf("could not parse %s: %v", vectorURL, err)
			}
			main.Host = host
			main.Port = port
			main.UseSSL = !defaultNoSSL
		}

	} else if logsDDURL, logsDDURLDefined := logsConfig.logsDDURL(); logsDDURLDefined {
		host, port, err := parseAddress(logsDDURL)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsDDURL, err)
		}
		main.Host = host
		main.Port = port
		main.UseSSL = !defaultNoSSL
	} else {
		main.Host = coreConfig.GetMainEndpoint(endpointPrefix, logsConfig.getConfigKey("dd_url"))
		main.UseSSL = !logsConfig.devModeNoSSL()
	}

	additionals := logsConfig.getAdditionalEndpoints()
	for i := 0; i < len(additionals); i++ {
		additionals[i].UseSSL = main.UseSSL
		additionals[i].APIKey = coreConfig.SanitizeAPIKey(additionals[i].APIKey)
		additionals[i].UseCompression = main.UseCompression
		additionals[i].CompressionLevel = main.CompressionLevel
		additionals[i].BackoffBase = main.BackoffBase
		additionals[i].BackoffMax = main.BackoffMax
		additionals[i].BackoffFactor = main.BackoffFactor
		additionals[i].RecoveryInterval = main.RecoveryInterval
		additionals[i].RecoveryReset = main.RecoveryReset

		if additionals[i].Version == 0 {
			additionals[i].Version = main.Version
		}
		if additionals[i].Version == EPIntakeVersion2 {
			additionals[i].TrackType = intakeTrackType
			additionals[i].Protocol = intakeProtocol
			additionals[i].Origin = intakeOrigin
		}
	}

	batchWait := logsConfig.batchWait()
	batchMaxConcurrentSend := logsConfig.batchMaxConcurrentSend()
	batchMaxSize := logsConfig.batchMaxSize()
	batchMaxContentSize := logsConfig.batchMaxContentSize()
	inputChanSize := logsConfig.inputChanSize()

	return NewEndpointsWithBatchSettings(main, additionals, false, true, batchWait, batchMaxConcurrentSend, batchMaxSize, batchMaxContentSize, inputChanSize), nil
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

// TaggerWarmupDuration is used to configure the tag providers
func TaggerWarmupDuration() time.Duration {
	return defaultLogsConfigKeys().taggerWarmupDuration()
}

// AggregationTimeout is used when performing aggregation operations
func AggregationTimeout() time.Duration {
	return defaultLogsConfigKeys().aggregationTimeout()
}
