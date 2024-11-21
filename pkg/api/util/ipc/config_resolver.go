// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file implement a basic Agent DNS to resolve Agent IPC addresses
// It would provide Client and Server building blocks to convert "http://core-cmd/agent/status" into "http://localhost:5001/agent/status" based on the configuration

package ipc

import (
	"fmt"
	"net"
	"strconv"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// ConfigResolver is an address resolver build around the Agent configuration
type ConfigResolver struct{}

// NewConfigResolver return a new instance of ConfigResolver
func NewConfigResolver() AddrResolver {
	return &ConfigResolver{}
}

// Resolve function implements the AddrResolver.Resolve() function and return the endpoint defined in the configuration
func (c *ConfigResolver) Resolve(name string) ([]Endpoint, error) {
	config := pkgconfigsetup.Datadog()

	switch name {
	case CoreCmd:
		host, err := pkgconfigsetup.GetIPCAddress(config)
		if err != nil {
			return nil, err
		}
		port := config.GetString("cmd_port")

		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, port))}, nil
	case CoreIPC:
		port := config.GetInt("agent_ipc.port")
		if port <= 0 {
			return nil, fmt.Errorf("agent_ipc.port cannot be <= 0")
		}

		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(config.GetString("agent_ipc.host"), strconv.Itoa(port)))}, nil

	case CoreExpvar:
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, config.GetString("expvar_port")))}, nil

	case TraceCmd:
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, config.GetString("apm_config.debug.port")))}, nil

	case TraceExpvar:
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, config.GetString("apm_config.debug.port")))}, nil

	case ProcessCmd:
		addr, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(addr)}, nil

	case ProcessExpvar:
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, config.GetString("process_config.expvar_port")))}, nil

	case SecurityCmd:
		addr, err := pkgconfigsetup.GetSecurityAgentAPIAddressPort(pkgconfigsetup.Datadog())
		if err != nil {
			return nil, err
		}

		return []Endpoint{NewTCPEndpoint(addr)}, nil

	case SecurityExpvar:
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, config.GetString("security_agent.expvar_port")))}, nil

	case ClusterAgent:
		host, err := pkgconfigsetup.GetIPCAddress(config)

		if err != nil {
			return nil, err
		}
		return []Endpoint{NewTCPEndpoint(net.JoinHostPort(host, config.GetString("cluster_agent.cmd_port")))}, nil
	default:
		return []Endpoint{}, fmt.Errorf("%v is not register in the configuration resolver", name)
	}
}
