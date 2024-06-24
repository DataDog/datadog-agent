// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimplnone provides the noop rdnsquerier component
package rdnsquerierimplnone

import (
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

// Provides defines the output of the rdnsquerier component
type Provides struct {
	Comp rdnsquerier.Component
}

type rdnsQuerierImplNone struct{}

// NewNone creates a new noop rdnsquerier component
func NewNone() Provides {
	return Provides{
		Comp: &rdnsQuerierImplNone{},
	}
}

// GetHostname does nothing for the noop rdnsquerier implementation
func (q *rdnsQuerierImplNone) GetHostname(_ []byte, _ func(string)) {
	// noop
}
