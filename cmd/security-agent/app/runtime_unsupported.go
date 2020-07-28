// +build !linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"errors"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func startRuntimeSecurity(hostname string, endpoints *config.Endpoints, context *client.DestinationsContext, stopper restart.Stopper) error {
	enabled := coreconfig.Datadog.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil
	}

	return errors.New("Datadog runtime security agent is only supported on Linux")
}
