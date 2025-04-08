// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package agent holds agent related files
package agent

import (
	"fmt"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// StartRuntimeSecurity starts runtime security
func StartRuntimeSecurity(log log.Component, config config.Component, hostname string, stopper startstop.Stopper, statsdClient ddgostatsd.ClientInterface, wmeta workloadmeta.Component, compression compression.Component) (*RuntimeSecurityAgent, error) {
	enabled := config.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	// start/stop order is important, agent need to be stopped first and started after all the others
	// components
	agent, err := NewRuntimeSecurityAgent(statsdClient, hostname, RSAOptions{
		LogProfiledWorkloads: config.GetBool("runtime_security_config.log_profiled_workloads"),
	}, wmeta)
	if err != nil {
		return nil, fmt.Errorf("unable to create a runtime security agent instance: %w", err)
	}
	stopper.Add(agent)

	useSecRuntimeTrack := config.GetBool("runtime_security_config.use_secruntime_track")
	endpoints, ctx, err := common.NewLogContextRuntime(useSecRuntimeTrack)
	if err != nil {
		_ = log.Error(err)
	}
	stopper.Add(ctx)

	reporter, err := reporter.NewCWSReporter(hostname, stopper, endpoints, ctx, compression)
	if err != nil {
		return nil, err
	}

	agent.Start(reporter, endpoints)

	log.Info("Datadog runtime security agent is now running")

	return agent, nil
}
