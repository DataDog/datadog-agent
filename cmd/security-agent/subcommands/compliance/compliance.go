// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

func StartCompliance(log log.Component, config config.Component, hostname string, stopper startstop.Stopper, statsdClient *ddgostatsd.Client) (*agent.Agent, error) {
	enabled := config.GetBool("compliance_config.enabled")
	if !enabled {
		return nil, nil
	}

	endpoints, context, err := command.NewLogContextCompliance(log)
	if err != nil {
		log.Error(err)
	}
	stopper.Add(context)

	runPath := config.GetString("compliance_config.run_path")
	reporter, err := event.NewLogReporter(stopper, "compliance-agent", "compliance", runPath, endpoints, context)
	if err != nil {
		return nil, err
	}

	runner := runner.NewRunner()
	stopper.Add(runner)

	scheduler := scheduler.NewScheduler(runner.GetChan())
	runner.SetScheduler(scheduler)

	checkInterval := config.GetDuration("compliance_config.check_interval")
	checkMaxEvents := config.GetInt("compliance_config.check_max_events_per_run")
	configDir := config.GetString("compliance_config.dir")
	metricsEnabled := config.GetBool("compliance_config.metrics.enabled")

	options := []checks.BuilderOption{
		checks.WithInterval(checkInterval),
		checks.WithMaxEvents(checkMaxEvents),
		checks.WithHostname(hostname),
		checks.WithHostRootMount(os.Getenv("HOST_ROOT")),
		checks.MayFail(checks.WithDocker()),
		checks.MayFail(checks.WithAudit()),
		checks.WithConfigDir(configDir),
	}
	if metricsEnabled {
		options = append(options, checks.WithStatsd(statsdClient))
	}

	agent, err := agent.New(
		reporter,
		scheduler,
		configDir,
		endpoints,
		options...,
	)
	if err != nil {
		log.Errorf("Compliance agent failed to initialize: %v", err)
		return nil, err
	}
	err = agent.Run()
	if err != nil {
		log.Errorf("Error starting compliance agent, exiting: %v", err)
		return nil, err
	}
	stopper.Add(agent)

	log.Infof("Running compliance checks every %s", checkInterval)

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
