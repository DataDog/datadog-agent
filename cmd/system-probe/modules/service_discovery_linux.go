// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	servicediscovery "github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/systemprobe"
)

// ServiceDiscoveryModule is the service_discovery module factory.
var ServiceDiscoveryModule = module.Factory{
	Name:             config.ServiceDiscoveryModule,
	ConfigNamespaces: []string{"service_discovery"},
	Fn:               servicediscovery.NewServiceDiscoveryModule,
	NeedsEBPF: func() bool {
		return false
	},
}
