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
	// RemoteFilter is the filter to apply to the remote tagger when streaming tag events
	RemoteFilter *types.Filter
	// RemoteTarget function return the target in which the remote tagger will connect
	// If it returns an error we stop the application
	RemoteTarget func(config.Component) (string, error)
	// RemoteTokenFetcher is the function to fetch the token for the remote tagger
	// If it returns an error the remote tagger will continue to attempt to fetch the token
	RemoteTokenFetcher func(config.Component) func() (string, error)
}

// Params provides local tagger parameters
type Params struct {
	// UseFakeTagger is a flag to enable the fake tagger. Only use for testing
	UseFakeTagger bool
}

// DualParams provides dual tagger parameters
type DualParams struct {
	// UseRemote is a function to determine if the remote tagger should be used
	UseRemote func(config.Component) bool
}
