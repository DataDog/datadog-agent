// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package localapiclientimpl provides the local API client component.
package localapiclientimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/updater/localapiclient"
	"github.com/DataDog/datadog-agent/pkg/installer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is the fx module for the installer local api client.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newLocalAPIClientComponent),
	)
}

func newLocalAPIClientComponent() localapiclient.Component {
	return installer.NewLocalAPIClient()
}
