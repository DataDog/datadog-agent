// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package settings bundles all core settings components
package settings

import (
	"github.com/DataDog/datadog-agent/comp/core/settings/registry/registryimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings/server/serverimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		registryimpl.Module(),
		serverimpl.Module())
}
