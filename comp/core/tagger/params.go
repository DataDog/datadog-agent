// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// AgentTypeForTagger represents agent types that tagger is used for
type AgentTypeForTagger uint8

// Define agent type for tagger
const (
	LocalTaggerAgent AgentTypeForTagger = 1 << iota
	NodeRemoteTaggerAgent
	CLCRunnerRemoteTaggerAgent
	FakeTagger
)

// Params provides the kind of agent we're instantiating workloadmeta for
type Params struct {
	AgentTypeForTagger                 AgentTypeForTagger
	FallBackToLocalIfRemoteTaggerFails bool
}

// NewTaggerParamsForCoreAgent is a constructor function for creating core agent tagger params
func NewTaggerParamsForCoreAgent(_ config.Component) Params {
	if pkgconfigsetup.IsCLCRunner(pkgconfigsetup.Datadog()) {
		return NewCLCRunnerRemoteTaggerParams()
	}
	return NewTaggerParams()
}

// NewTaggerParams creates a Params struct with the default LocalTagger type
func NewTaggerParams() Params {
	return Params{AgentTypeForTagger: LocalTaggerAgent,
		FallBackToLocalIfRemoteTaggerFails: false}
}

// NewFakeTaggerParams creates a Params struct with the FakeTagger type and for testing purposes
func NewFakeTaggerParams() Params {
	return Params{AgentTypeForTagger: FakeTagger,
		FallBackToLocalIfRemoteTaggerFails: false}
}

// NewNodeRemoteTaggerParams creates a Params struct with the NodeRemoteTagger type
func NewNodeRemoteTaggerParams() Params {
	return Params{AgentTypeForTagger: NodeRemoteTaggerAgent,
		FallBackToLocalIfRemoteTaggerFails: false}
}

// NewNodeRemoteTaggerParamsWithFallback creates a Params struct with the NodeRemoteTagger type
// and fallback to local tagger if remote tagger fails
func NewNodeRemoteTaggerParamsWithFallback() Params {
	return Params{AgentTypeForTagger: NodeRemoteTaggerAgent,
		FallBackToLocalIfRemoteTaggerFails: true}
}

// NewCLCRunnerRemoteTaggerParams creates a Params struct with the CLCRunnerRemoteTagger type
func NewCLCRunnerRemoteTaggerParams() Params {
	return Params{AgentTypeForTagger: CLCRunnerRemoteTaggerAgent,
		FallBackToLocalIfRemoteTaggerFails: false}
}
