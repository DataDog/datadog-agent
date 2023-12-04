// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

type TaggerType uint8
type TaggerAgentType uint8

const (
	LocalTagger  TaggerType = 1 << iota
	RemoteTagger TaggerType = 2
	FakeTagger   TaggerType = 3
)

const (
	// agent types that tagger is used for
	LocalTaggerAgent TaggerAgentType = 1 << iota
	NodeRemoteTaggerAgent
	CLCRunnerRemoteTaggerAgent
)

// TaggerParams provides the kind of agent we're instantiating workloadmeta for
type Params struct {
	TaggerType TaggerType
	AgentType  TaggerAgentType
}

// NewTaggerType creates a Params struct with the default LocalTagger type
func NewParams() Params {
	return Params{TaggerType: LocalTagger,
		AgentType: LocalTaggerAgent}
}
