// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"fmt"
	"net"
	"strconv"

	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/logs/client"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

const endpointPrefix = "agent-intake.logs."

var logsEndpoints = map[string]int{
	"agent-intake.logs.datadoghq.com": 10516,
	"agent-intake.logs.datadoghq.eu":  443,
	"agent-intake.logs.datad0g.com":   10516,
	"agent-intake.logs.datad0g.eu":    443,
}

// BuildEndpoints returns the endpoints to send logs to.
func BuildEndpoints() (*client.Endpoints, error) {
	if config.Datadog.GetBool("logs_config.dev_mode_no_ssl") {
		log.Warnf("Use of illegal configuration parameter, if you need to send your logs to a proxy, please use 'logs_config.logs_dd_url' and 'logs_config.logs_no_ssl' instead")
	}

	var useSSL bool
	useProto := config.Datadog.GetBool("logs_config.dev_mode_use_proto")
	proxyAddress := config.Datadog.GetString("logs_config.socks5_proxy_address")

	main := client.Endpoint{
		APIKey:       config.Datadog.GetString("api_key"),
		Logset:       config.Datadog.GetString("logset"),
		UseProto:     useProto,
		ProxyAddress: proxyAddress,
	}
	switch {
	case config.Datadog.IsSet("logs_config.logs_dd_url"):
		// Proxy settings, expect 'logs_config.logs_dd_url' to respect the format '<HOST>:<PORT>'
		// and '<PORT>' to be an integer.
		// By default ssl is enabled ; to disable ssl set 'logs_config.logs_no_ssl' to true.
		host, portString, err := net.SplitHostPort(config.Datadog.GetString("logs_config.logs_dd_url"))
		if err != nil {
			return nil, fmt.Errorf("could not parse logs_dd_url: %v", err)
		}
		port, err := strconv.Atoi(portString)
		if err != nil {
			return nil, fmt.Errorf("could not parse logs_dd_url port: %v", err)
		}
		main.Host = host
		main.Port = port
		useSSL = !config.Datadog.GetBool("logs_config.logs_no_ssl")
	case config.Datadog.GetBool("logs_config.use_port_443"):
		main.Host = config.Datadog.GetString("logs_config.dd_url_443")
		main.Port = 443
		useSSL = true
	default:
		// If no proxy is set, we default to 'logs_config.dd_url' if set, or to 'site'.
		// if none of them is set, we default to the US agent endpoint.
		main.Host = config.GetMainEndpoint(endpointPrefix, "logs_config.dd_url")
		if port, found := logsEndpoints[main.Host]; found {
			main.Port = port
		} else {
			main.Port = config.Datadog.GetInt("logs_config.dd_port")
		}
		useSSL = !config.Datadog.GetBool("logs_config.dev_mode_no_ssl")
	}
	main.UseSSL = useSSL

	var additionals []client.Endpoint
	err := config.Datadog.UnmarshalKey("logs_config.additional_endpoints", &additionals)
	if err != nil {
		log.Warnf("Could not parse additional_endpoints for logs: %v", err)
	}
	for i := 0; i < len(additionals); i++ {
		additionals[i].UseSSL = useSSL
		additionals[i].UseProto = useProto
		additionals[i].ProxyAddress = proxyAddress
	}

	return client.NewEndpoints(main, additionals), nil
}
