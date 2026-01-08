// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentparams

type channel string

const (
	StableChannel  channel = "stable"
	BetaChannel    channel = "beta"
	NightlyChannel channel = "nightly"
)

// Agent flavor constants
//
// PackageVersion.Flavor is in some cases passed directly to underlying install scripts,
// so this list is not exhaustive.
//
// See PackageFlavor https://github.com/DataDog/agent-release-management/blob/main/generator/const.py
const (
	DefaultFlavor = BaseFlavor
	BaseFlavor    = "datadog-agent"
	FIPSFlavor    = "datadog-fips-agent"
)

type PackageVersion struct {
	Major      string
	Minor      string // Empty means latest
	Channel    channel
	PipelineID string
	Flavor     string // Empty means default (base)
	LocalPath  string // Local path to the agent packages
}
