// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogsAgent is the global configuration object
var LogsAgent = config.Datadog

// DefaultSources returns the default log sources that can be directly set from the datadog.yaml or through environment variables.
func DefaultSources() []*LogSource {
	var sources []*LogSource

	tcpForwardPort := LogsAgent.GetInt("logs_config.tcp_forward_port")
	if tcpForwardPort > 0 {
		// append source to collect all logs forwarded by TCP on a given port.
		source := NewLogSource("tcp_forward", &LogsConfig{
			Type: TCPType,
			Port: tcpForwardPort,
		})
		sources = append(sources, source)
	}

	if LogsAgent.GetBool("logs_config.container_collect_all") {
		// append a new source to collect all logs from all containers
		source := NewLogSource("container_collect_all", &LogsConfig{
			Type:    DockerType,
			Service: "docker",
			Source:  "docker",
		})
		sources = append(sources, source)
	}

	return sources
}

// BuildEndpoints returns the endpoints to send logs to.
func BuildEndpoints() (*Endpoints, error) {
	if LogsAgent.GetBool("logs_config.dev_mode_no_ssl") {
		log.Warnf("Use of illegal configuration parameter, if you need to send your logs to a proxy, please use 'logs_config.logs_dd_url' and 'logs_config.logs_no_ssl' instead")
	}

	var useSSL bool
	useProto := LogsAgent.GetBool("logs_config.dev_mode_use_proto")
	proxyAddress := LogsAgent.GetString("logs_config.socks5_proxy_address")

	main := Endpoint{
		APIKey:       LogsAgent.GetString("api_key"),
		Logset:       LogsAgent.GetString("logset"),
		UseProto:     useProto,
		ProxyAddress: proxyAddress,
	}
	switch {
	case LogsAgent.GetString("logs_config.logs_dd_url") != "":
		// Proxy settings, expect 'logs_config.logs_dd_url' to respect the format '<HOST>:<PORT>'
		// and '<PORT>' to be an integer.
		// By default ssl is enabled ; to disable ssl set 'logs_config.logs_no_ssl' to true.
		host, portString, err := net.SplitHostPort(LogsAgent.GetString("logs_config.logs_dd_url"))
		if err != nil {
			return nil, fmt.Errorf("could not parse logs_dd_url: %v", err)
		}
		port, err := strconv.Atoi(portString)
		if err != nil {
			return nil, fmt.Errorf("could not parse logs_dd_url port: %v", err)
		}
		main.Host = host
		main.Port = port
		useSSL = !LogsAgent.GetBool("logs_config.logs_no_ssl")
	case LogsAgent.GetBool("logs_config.use_port_443"):
		main.Host = LogsAgent.GetString("logs_config.dd_url_443")
		main.Port = 443
		useSSL = true
	default:
		// datadog settings
		main.Host = LogsAgent.GetString("logs_config.dd_url")
		main.Port = LogsAgent.GetInt("logs_config.dd_port")
		useSSL = !LogsAgent.GetBool("logs_config.dev_mode_no_ssl")
	}
	main.UseSSL = useSSL

	var additionals []Endpoint
	err := LogsAgent.UnmarshalKey("logs_config.additional_endpoints", &additionals)
	if err != nil {
		log.Warnf("Could not parse additional_endpoints for logs: %v", err)
	}
	for i := 0; i < len(additionals); i++ {
		additionals[i].UseSSL = useSSL
		additionals[i].UseProto = useProto
		additionals[i].ProxyAddress = proxyAddress
	}

	return NewEndpoints(main, additionals), nil
}
