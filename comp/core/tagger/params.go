// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

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
	AgentTypeForTagger AgentTypeForTagger
}

// NewTaggerParams creates a Params struct with the default LocalTagger type
func NewTaggerParams() Params {
	return Params{AgentTypeForTagger: LocalTaggerAgent}
}

// NewFakeTaggerParams creates a Params struct with the FakeTagger type and for testing purposes
func NewFakeTaggerParams() Params {
	return Params{AgentTypeForTagger: FakeTagger}
}
