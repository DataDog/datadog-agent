// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains all the types needed by Flare providers without the underlying implementation and dependencies.
// This allows components to offer flare capabilities without linking to the flare dependencies when the flare feature
// is not built in the binary.
package types

import (
	"time"

	"go.uber.org/fx"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
)

// ProfileData maps (pprof) profile names to the profile data.
type ProfileData map[string][]byte

// FlareBuilder contains all the helpers to add files to a flare archive.
// see the aliased type for the full description
type FlareBuilder = flarebuilder.FlareBuilder

// FlareArgs contains the args passed in by the caller to the flare generation process
// see the aliased type for the full description
type FlareArgs = flarebuilder.FlareArgs

// FlareCallback is a function that can be registered as a data provider for flares by way of the FlareProvider struct.
// This function, if registered, will be called everytime a flare is created.
type FlareCallback func(fb FlareBuilder) error

// FlareTimeout is a function that provides the maximum expected runtime duration of a FlareProvider's callback.
// Return 0 from this function to utilize the default timeout instead.
type FlareTimeout func(fb FlareBuilder) time.Duration

// FlareFiller is a struct that can be registered as a data provider for flares. This struct's callback, if registered, will
// be called everytime a flare is created.
type FlareFiller struct {
	Callback FlareCallback
	Timeout  FlareTimeout
}

// Provider is provided by other components to register themselves to provide flare data.
type Provider struct {
	fx.Out
	FlareFiller *FlareFiller `group:"flare"`
}

// NewFiller wraps the callback with the default FlareProvider configuration. This function is exposed
// via the public api to support legacy flare functionality that has not yet been componetized. New callers
// are strongly encouraged to utilize the NewProvider or NewProviderWithTimeout functions instead.
func NewFiller(callback FlareCallback) *FlareFiller {
	return &FlareFiller{Callback: callback, Timeout: func(FlareBuilder) time.Duration { return 0 }}
}

// NewProvider returns a new Provider to be called when a flare is created
func NewProvider(callback FlareCallback) Provider {
	return Provider{FlareFiller: NewFiller(callback)}
}

// NewProviderWithTimeout returns a new Provider to be called when a flare is created
func NewProviderWithTimeout(callback FlareCallback, timeout FlareTimeout) Provider {
	return Provider{
		FlareFiller: &FlareFiller{
			Callback: callback,
			Timeout:  timeout,
		},
	}
}
