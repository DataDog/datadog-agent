// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerCollectAll is the name of the docker integration that collect logs from all containers
const ContainerCollectAll = "container_collect_all"

// logs-intake endpoint prefix.
const (
	tcpEndpointPrefix  = "agent-intake.logs."
	httpEndpointPrefix = "agent-http-intake.logs."
)

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

// DefaultSources returns the default log sources that can be directly set from the datadog.yaml or through environment variables.
func DefaultSources() []*LogSource {
	var sources []*LogSource

	if coreConfig.Datadog.GetBool("logs_config.container_collect_all") {
		// append a new source to collect all logs from all containers
		source := NewLogSource(ContainerCollectAll, &LogsConfig{
			Type:    DockerType,
			Service: "docker",
			Source:  "docker",
		})
		sources = append(sources, source)
	}

	return sources
}

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

// BuildEndpoints returns the endpoints to send logs.
func BuildEndpoints(httpConnectivity HTTPConnectivity) (*Endpoints, error) {
	if coreConfig.Datadog.GetBool("logs_config.dev_mode_no_ssl") {
		log.Warnf("Use of illegal configuration parameter, if you need to send your logs to a proxy, please use 'logs_config.logs_dd_url' and 'logs_config.logs_no_ssl' instead")
	}
	if isForceHTTPUse() || (bool(httpConnectivity) && !(isForceTCPUse() || isSocks5ProxySet() || hasAdditionalEndpoints())) {
		return BuildHTTPEndpoints()
	}
	log.Warn("You are currently sending Logs to Datadog through TCP (either because logs_config.use_tcp or logs_config.socks5_proxy_address is set or the HTTP connectivity test has failed) " +
		"To benefit from increased reliability and better network performances, " +
		"we strongly encourage switching over to compressed HTTPS which is now the default protocol.")
	return buildTCPEndpoints()
}

func isSocks5ProxySet() bool {
	return len(coreConfig.Datadog.GetString("logs_config.socks5_proxy_address")) > 0
}

func isForceTCPUse() bool {
	return coreConfig.Datadog.GetBool("logs_config.use_tcp")
}

func isForceHTTPUse() bool {
	return coreConfig.Datadog.GetBool("logs_config.use_http")
}

func hasAdditionalEndpoints() bool {
	return len(getAdditionalEndpoints()) > 0
}

func buildTCPEndpoints() (*Endpoints, error) {
	useProto := coreConfig.Datadog.GetBool("logs_config.dev_mode_use_proto")
	proxyAddress := coreConfig.Datadog.GetString("logs_config.socks5_proxy_address")
	main := Endpoint{
		APIKey:       getLogsAPIKey(coreConfig.Datadog),
		ProxyAddress: proxyAddress,
	}
	switch {
	case isSetAndNotEmpty(coreConfig.Datadog, "logs_config.logs_dd_url"):
		// Proxy settings, expect 'logs_config.logs_dd_url' to respect the format '<HOST>:<PORT>'
		// and '<PORT>' to be an integer.
		// By default ssl is enabled ; to disable ssl set 'logs_config.logs_no_ssl' to true.
		host, port, err := parseAddress(coreConfig.Datadog.GetString("logs_config.logs_dd_url"))
		if err != nil {
			return nil, fmt.Errorf("could not parse logs_dd_url: %v", err)
		}
		main.Host = host
		main.Port = port
		main.UseSSL = !coreConfig.Datadog.GetBool("logs_config.logs_no_ssl")
	case coreConfig.Datadog.GetBool("logs_config.use_port_443"):
		main.Host = coreConfig.Datadog.GetString("logs_config.dd_url_443")
		main.Port = 443
		main.UseSSL = true
	default:
		// If no proxy is set, we default to 'logs_config.dd_url' if set, or to 'site'.
		// if none of them is set, we default to the US agent endpoint.
		main.Host = coreConfig.GetMainEndpoint(tcpEndpointPrefix, "logs_config.dd_url")
		if port, found := logsEndpoints[main.Host]; found {
			main.Port = port
		} else {
			main.Port = coreConfig.Datadog.GetInt("logs_config.dd_port")
		}
		main.UseSSL = !coreConfig.Datadog.GetBool("logs_config.dev_mode_no_ssl")
	}

	additionals := getAdditionalEndpoints()
	for i := 0; i < len(additionals); i++ {
		additionals[i].UseSSL = main.UseSSL
		additionals[i].ProxyAddress = proxyAddress
	}
	return NewEndpoints(main, additionals, useProto, false, 0), nil
}

// BuildHTTPEndpoints returns the HTTP endpoints to send logs to.
func BuildHTTPEndpoints() (*Endpoints, error) {
	main := Endpoint{
		APIKey:                  getLogsAPIKey(coreConfig.Datadog),
		UseCompression:          coreConfig.Datadog.GetBool("logs_config.use_compression"),
		CompressionLevel:        coreConfig.Datadog.GetInt("logs_config.compression_level"),
		ConnectionResetInterval: time.Duration(coreConfig.Datadog.GetInt("logs_config.connection_reset_interval")) * time.Second,
	}

	switch {
	case isSetAndNotEmpty(coreConfig.Datadog, "logs_config.logs_dd_url"):
		host, port, err := parseAddress(coreConfig.Datadog.GetString("logs_config.logs_dd_url"))
		if err != nil {
			return nil, fmt.Errorf("could not parse logs_dd_url: %v", err)
		}
		main.Host = host
		main.Port = port
		main.UseSSL = !coreConfig.Datadog.GetBool("logs_config.logs_no_ssl")
	default:
		main.Host = coreConfig.GetMainEndpoint(httpEndpointPrefix, "logs_config.dd_url")
		main.UseSSL = !coreConfig.Datadog.GetBool("logs_config.dev_mode_no_ssl")
	}

	additionals := getAdditionalEndpoints()
	for i := 0; i < len(additionals); i++ {
		additionals[i].UseSSL = main.UseSSL
	}

	batchWait := batchWait(coreConfig.Datadog)

	return NewEndpoints(main, additionals, false, true, batchWait), nil
}

func getAdditionalEndpoints() []Endpoint {
	var endpoints []Endpoint
	var err error
	raw := coreConfig.Datadog.Get("logs_config.additional_endpoints")
	if raw == nil {
		return endpoints
	}
	if s, ok := raw.(string); ok && s != "" {
		err = json.Unmarshal([]byte(s), &endpoints)
	} else {
		err = coreConfig.Datadog.UnmarshalKey("logs_config.additional_endpoints", &endpoints)
	}
	if err != nil {
		log.Warnf("Could not parse additional_endpoints for logs: %v", err)
	}
	return endpoints
}

func isSetAndNotEmpty(config coreConfig.Config, key string) bool {
	return config.IsSet(key) && len(config.GetString(key)) > 0
}

// getLogsAPIKey provides the dd api key used by the main logs agent sender.
func getLogsAPIKey(config coreConfig.Config) string {
	if isSetAndNotEmpty(config, "logs_config.api_key") {
		return config.GetString("logs_config.api_key")
	}
	return config.GetString("api_key")
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

func batchWait(config coreConfig.Config) time.Duration {
	batchWait := coreConfig.Datadog.GetInt("logs_config.batch_wait")
	if batchWait < 1 || 10 < batchWait {
		log.Warnf("Invalid batch_wait: %v should be in [1, 10], fallback on %v", batchWait, coreConfig.DefaultBatchWait)
		return coreConfig.DefaultBatchWait * time.Second
	}
	return (time.Duration(batchWait) * time.Second)
}

// TaggerWarmupDuration is used to configure the tag providers
func TaggerWarmupDuration() time.Duration {
	return coreConfig.Datadog.GetDuration("logs_config.tagger_warmup_duration") * time.Second
}
