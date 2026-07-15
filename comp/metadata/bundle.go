// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
// by the Agent.
package metadata

import (
	haagentmetadatafx "github.com/DataDog/datadog-agent/comp/metadata/haagent/fx"
	hostfx "github.com/DataDog/datadog-agent/comp/metadata/host/fx"
	hostgpu "github.com/DataDog/datadog-agent/comp/metadata/hostgpu/fx"
	hostsysteminfo "github.com/DataDog/datadog-agent/comp/metadata/hostsysteminfo/fx"
	inventoryagentfx "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/fx"
	inventorychecksfx "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/fx"
	inventoryhostfx "github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/fx"
	packagesigningfx "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/fx"
	resourcesfx "github.com/DataDog/datadog-agent/comp/metadata/resources/fx"
	runnerfx "github.com/DataDog/datadog-agent/comp/metadata/runner/fx"
	securityagent "github.com/DataDog/datadog-agent/comp/metadata/securityagent/fx"
	systemprobe "github.com/DataDog/datadog-agent/comp/metadata/systemprobe/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-configuration

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		runnerfx.Module(),
		resourcesfx.Module(),
		hostfx.Module(),
		inventoryagentfx.Module(),
		inventoryhostfx.Module(),
		hostgpu.Module(),
		hostsysteminfo.Module(),
		inventorychecksfx.Module(),
		packagesigningfx.Module(),
		systemprobe.Module(),
		securityagent.Module(),
		haagentmetadatafx.Module(),
	)
}

// MockBundle defines the mock fx options for this bundle.
func MockBundle() fxutil.BundleOptions { return fxutil.Bundle() }
