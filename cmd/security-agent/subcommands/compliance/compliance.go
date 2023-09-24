// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// StartCompliance runs the compliance sub-agent running compliance benchmarks
// and checks.
func StartCompliance(log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, hostname string, stopper startstop.Stopper, statsdClient *ddgostatsd.Client) (*compliance.Agent, error) {
	enabled := config.GetBool("compliance_config.enabled")
	runPath := config.GetString("compliance_config.run_path")
	configDir := config.GetString("compliance_config.dir")
	metricsEnabled := config.GetBool("compliance_config.metrics.enabled")
	checkInterval := config.GetDuration("compliance_config.check_interval")

	if !enabled {
		return nil, nil
	}

	endpoints, context, err := common.NewLogContextCompliance()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(context)

	reporter, err := compliance.NewLogReporter(stopper, "compliance-agent", "compliance", runPath, endpoints, context)
	if err != nil {
		return nil, err
	}

	resolverOptions := compliance.ResolverOptions{
		Hostname:           hostname,
		HostRoot:           os.Getenv("HOST_ROOT"),
		DockerProvider:     compliance.DefaultDockerProvider,
		LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
	}

	if metricsEnabled {
		resolverOptions.StatsdClient = statsdClient
	}

	senderManager := aggregator.GetSenderManager()
	runner := runner.NewRunner(senderManager)
	stopper.Add(runner)
	agent := compliance.NewAgent(senderManager, compliance.AgentOptions{
		ResolverOptions: resolverOptions,
		ConfigDir:       configDir,
		Reporter:        reporter,
		CheckInterval:   checkInterval,
	})
	err = agent.Start()
	if err != nil {
		log.Errorf("Error starting compliance agent, exiting: %v", err)
		return nil, err
	}
	stopper.Add(agent)

	// Send the compliance 'running' metrics periodically
	ticker := sendRunningMetrics(statsdClient, "compliance")
	stopper.Add(ticker)

	return agent, nil
}

// sendRunningMetrics exports a metric to distinguish between security-agent modules that are activated
func sendRunningMetrics(statsdClient *ddgostatsd.Client, moduleName string) *time.Ticker {
	// Retrieve the agent version using a dedicated package
	tags := []string{fmt.Sprintf("version:%s", version.AgentVersion)}

	// Send the metric regularly
	heartbeat := time.NewTicker(15 * time.Second)
	go func() {
		for range heartbeat.C {
			statsdClient.Gauge(fmt.Sprintf("datadog.security_agent.%s.running", moduleName), 1, tags, 1) //nolint:errcheck
		}
	}()

	return heartbeat
}
