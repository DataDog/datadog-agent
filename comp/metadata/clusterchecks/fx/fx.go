// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

// Package fx provides the fx module for the clusterchecks metadata component
package fx

import (
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	clusterchecksimpl "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(clusterchecksimpl.NewComponent),
		fxutil.ProvideOptional[clusterchecksmetadata.Component](),
	)
}
