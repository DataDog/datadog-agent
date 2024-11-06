// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// RemoteParams provides remote tagger parameters
type RemoteParams struct {
	RemoteFilter       *types.Filter
	RemoteTarget       func(config.Component) (string, error)
	RemoteTokenFetcher func(config.Component) func() (string, error)
}

// Params provides local tagger parameters
type Params struct {
	UseFakeTagger bool
}

// DualParams provides dual tagger parameters
type DualParams struct {
	UseRemote func(config.Component) bool
}
