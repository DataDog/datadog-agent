// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
// by the Agent.
package metadata

import (
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		runnerimpl.Module(),
		resourcesimpl.Module(),
		host.Module(),
		inventoryagent.Module(),
		inventoryhostimpl.Module(),
		inventorychecksimpl.Module(),
	)
}

// MockBundle defines the mock fx options for this bundle.
func MockBundle() fxutil.BundleOptions { return fxutil.Bundle() }
