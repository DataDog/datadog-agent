// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"errors"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	ddgostatsd "github.com/DataDog/datadog-go/statsd"
	"github.com/spf13/cobra"
)

var runtimeCmd *cobra.Command

func startRuntimeSecurity(hostname string, stopper startstop.Stopper, statsdClient *ddgostatsd.Client) (*secagent.RuntimeSecurityAgent, error) {
	enabled := coreconfig.Datadog.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	return nil, errors.New("Datadog runtime security agent is only supported on Linux")
}
