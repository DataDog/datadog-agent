// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides fx options for the compression component.
package fx

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	compressionimpl "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
)

// Module specifies the compression module.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			compressionimpl.NewComponent,
		),
		fxutil.ProvideOptional[compression.Component](),
	)
}
