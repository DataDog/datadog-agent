// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the rcservicemrf component.
package fx

import (
	rcservicemrfimpl "github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module conditionally provides the HA DC remote config service.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(rcservicemrfimpl.NewMrfRemoteConfigServiceOptional),
	)
}
