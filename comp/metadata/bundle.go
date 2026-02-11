// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
// by the Agent.
package metadata

import (
	haagentmetadatafx "github.com/DataDog/datadog-agent/comp/metadata/haagent/fx"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	hostgpu "github.com/DataDog/datadog-agent/comp/metadata/hostgpu/fx"
	hostsysteminfo "github.com/DataDog/datadog-agent/comp/metadata/hostsysteminfo/fx"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning/packagesigningimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	securityagent "github.com/DataDog/datadog-agent/comp/metadata/securityagent/fx"
	systemprobe "github.com/DataDog/datadog-agent/comp/metadata/systemprobe/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-configuration

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		runnerimpl.Module(),
		resourcesimpl.Module(),
		hostimpl.Module(),
		inventoryagentimpl.Module(),
		inventoryhostimpl.Module(),
		hostgpu.Module(),
		hostsysteminfo.Module(),
		inventorychecksimpl.Module(),
		packagesigningimpl.Module(),
		systemprobe.Module(),
		securityagent.Module(),
		haagentmetadatafx.Module(),
	)
}

// MockBundle defines the mock fx options for this bundle.
func MockBundle() fxutil.BundleOptions { return fxutil.Bundle() }
