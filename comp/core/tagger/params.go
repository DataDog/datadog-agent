// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

// TaggerAgentType represents agent types that tagger is used for
type TaggerAgentType uint8

// Define agent type for tagger
const (
	LocalTaggerAgent TaggerAgentType = 1 << iota
	NodeRemoteTaggerAgent
	CLCRunnerRemoteTaggerAgent
	FakeTagger
)

// Params provides the kind of agent we're instantiating workloadmeta for
type Params struct {
	TaggerAgentType TaggerAgentType
}

// NewTaggerParams creates a Params struct with the default LocalTagger type
func NewTaggerParams() Params {
	return Params{TaggerAgentType: LocalTaggerAgent}
}

// NewFakeTaggerParams creates a Params struct with the FakeTagger type and for testing purposes
func NewFakeTaggerParams() Params {
	return Params{TaggerAgentType: FakeTagger}
}
