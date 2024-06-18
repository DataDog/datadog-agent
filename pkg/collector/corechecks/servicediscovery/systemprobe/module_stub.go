// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package systemprobe

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// ServiceDiscoveryModule is the service_discovery module factory.
var ServiceDiscoveryModule = module.Factory{
	Name:             config.ServiceDiscoveryModule,
	ConfigNamespaces: moduleNamespaces,
	Fn: func(_ *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component], _ telemetry.Component) (module.Module, error) {
		return nil, module.ErrNotEnabled
	},
}
