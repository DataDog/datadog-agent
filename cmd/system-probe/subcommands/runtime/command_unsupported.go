// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package runtime holds runtime related files
package runtime

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Commands returns the runtime security commands
func Commands(*command.GlobalParams) []*cobra.Command {
	return nil
}

// StartRuntimeSecurity starts runtime security
func StartRuntimeSecurity(log log.Component, config config.Component, _ string, _ startstop.Stopper, _ statsd.ClientInterface, _ workloadmeta.Component) (*secagent.RuntimeSecurityAgent, error) {
	enabled := config.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	return nil, errors.New("Datadog runtime security agent is only supported on Linux and Windows")
}
