// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the Fx module for Azure App Service extension metadata.
package fx

import (
	serverlessaasimpl "github.com/DataDog/datadog-agent/comp/metadata/serverlessaas/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module registers the AAS metadata implementation. The binary selects the runner.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(serverlessaasimpl.NewComponent),
	)
}
