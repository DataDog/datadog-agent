// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package demultiplexerimpl

import "github.com/DataDog/datadog-agent/pkg/aggregator"

// Params contains the parameters for the demultiplexer
type Params struct {
	aggregator.AgentDemultiplexerOptions
	ContinueOnMissingHostname bool
}

// NewDefaultParams returns the default parameters for the demultiplexer
func NewDefaultParams() Params {
	return Params{
		AgentDemultiplexerOptions: aggregator.DefaultAgentDemultiplexerOptions(),
		ContinueOnMissingHostname: false,
	}
}
