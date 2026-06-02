// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the subcommands of the trace-agent.
package subcommands

import coreconfig "github.com/DataDog/datadog-agent/comp/core/config"

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfPath             string
	ConfigName           string
	LoggerName           string
	FleetPoliciesDirPath string
}

// AgentParamsOptions builds the option list that should be passed to
// coreconfig.NewAgentParams for any trace-agent subcommand that wires up
// the config component. It centralizes the policy that when no explicit
// config file is in play (ConfPath empty), the core-config search-directory
// fallback (defaults to /etc/datadog-agent on Linux) must also be disabled
// so that a stray core-agent datadog.yaml is not loaded silently for the
// trace-agent. Subcommands may append additional options as needed.
func AgentParamsOptions(p *GlobalParams) []func(*coreconfig.Params) {
	opts := []func(*coreconfig.Params){
		coreconfig.WithFleetPoliciesDirPath(p.FleetPoliciesDirPath),
	}
	if p.ConfPath == "" {
		opts = append(opts, coreconfig.WithoutDefaultConfPath())
	}
	return opts
}
