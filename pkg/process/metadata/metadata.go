// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import "github.com/DataDog/datadog-agent/pkg/process/procutil"

// Extractor is common interface for Process metadata extraction
type Extractor interface {
	Extract(p *procutil.Process)
	Type() string
}

// ProcessMetadataProvider - (Proof of Concept) This is unused at the moment.
// For the current connection check this is over-engineered, but for a more generalized usage across checks I'm thinking
// something similar to go's context or workload meta where parsed metadata can be shared across checks. The challenge
// here I'm seeing is how the data retrieval happens for any parser. The generic lookup can be by pid, but the docker
// proxy filter is also indexing data by connection details
type ProcessMetadataProvider struct {
	extractorsByType map[string]Extractor
}

func NewProcessMetadataProvider() *ProcessMetadataProvider {
	return &ProcessMetadataProvider{
		extractorsByType: make(map[string]Extractor),
	}
}

func (pm *ProcessMetadataProvider) Register(p Extractor) {
	pm.extractorsByType[p.Type()] = p
}

func (pm *ProcessMetadataProvider) Extract(procs map[int32]*procutil.Process) {
	for _, p := range procs {
		for _, parser := range pm.extractorsByType {
			parser.Extract(p)
		}
	}
}
