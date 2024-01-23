// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package workloadmeta

// Params provides the kind of agent we're instantiating workloadmeta for
type Params struct {
	AgentType  AgentType
	InitHelper InitHelper
	NoInstance bool
}

// NewParams creates a Params struct with the default NodeAgent configuration
func NewParams() Params {
	panic("not called")
}
