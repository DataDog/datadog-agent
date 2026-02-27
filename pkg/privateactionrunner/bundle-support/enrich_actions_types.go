// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package bundle_support

type EnrichedActionInputs struct {
	Search string `json:"search,omitempty"`
}

type EnrichedActionOutputs struct {
	Options     []LabelValue `json:"options"`
	Placeholder string       `json:"placeholder"`
}

type LabelValue struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}
