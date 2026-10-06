// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fxsharedlibrary provides shared-library check support.
package fxsharedlibrary

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/collector/sharedlibrary"
	sharedlibraryimpl "github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/sharedlibraryimpl"
)

type initializer struct{}

func (initializer) InitSharedLibraryChecksLoader() {
	sharedlibraryimpl.InitSharedLibraryChecksLoader()
}

// Module provides shared-library check loader initialization.
func Module() fx.Option {
	return fx.Module(
		"comp/collector/sharedlibrary/fx-sharedlibrary",
		fx.Provide(func() sharedlibrary.LoaderInitializer {
			return initializer{}
		}),
	)
}
