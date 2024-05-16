// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains all the types needed by Flare providers without the underlying implementation and dependencies.
// This allows components to offer flare capabilities without linking to the flare dependencies when the flare feature
// is not built in the binary.
package types

import (
	flaredef "github.com/DataDog/datadog-agent/comp/core/flare/def"
	"go.uber.org/fx"
)

// FlareBuilder contains all the helpers to add files to a flare archive.
// see the aliased type for the full description
type FlareBuilder = flaredef.FlareBuilder

// FlareCallback is called by the FlareBuilder to build the flare
// see the aliased type for the full description
type FlareCallback = flaredef.FlareCallback

// Provider is provided by other components to register themselves to provide flare data.
type Provider struct {
	fx.Out
	Callback flaredef.FlareCallback `group:"flare"`
}

// NewProvider returns a new Provider to be called when a flare is created
func NewProvider(callback flaredef.FlareCallback) Provider {
	return Provider{
		Callback: callback,
	}
}
