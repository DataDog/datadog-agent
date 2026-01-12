// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package modules

import (
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Traceroute is a factory for NDMs Traceroute module
var Traceroute = &module.Factory{
	Name:             config.TracerouteModule,
	ConfigNamespaces: tracerouteConfigNamespaces,
	Fn:               createTracerouteModule,
}

// startPlatformDriver starts the Windows network driver for traceroute
func startPlatformDriver() error {
	if err := driver.Start(); err != nil {
		log.Errorf("failed to start Windows network driver: %s", err)
		return err
	}
	log.Debug("Windows network driver started for traceroute")
	return nil
}

func stopPlatformDriver() error {
	if err := driver.Stop(); err != nil {
		log.Errorf("failed to stop Windows Driver: %s", err)
		return err
	}
	log.Debug("Windows Driver stopped for traceroute")
	return nil
}
