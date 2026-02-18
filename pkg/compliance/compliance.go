// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements a specific part of the datadog-agent
// responsible for scanning host and containers and report various
// misconfigurations and compliance issues.
package compliance

import (
	"fmt"
	"os"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// StartCompliance runs the compliance sub-agent running compliance benchmarks
// and checks.
func StartCompliance(log log.Component,
	config config.Component,
	hostname string,
	stopper startstop.Stopper,
	statsdClient ddgostatsd.ClientInterface,
	wmeta workloadmeta.Component,
	filterStore workloadfilter.Component,
	compression compression.Component,
	sysProbeClient SysProbeClient,
) (*Agent, error) {

	enabled := config.GetBool("compliance_config.enabled")
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

	resolverOptions := ResolverOptions{
		Hostname:           hostname,
		HostRoot:           os.Getenv("HOST_ROOT"),
		DockerProvider:     DefaultDockerProvider,
		LinuxAuditProvider: DefaultLinuxAuditProvider,
	}

	if metricsEnabled {
		resolverOptions.StatsdClient = statsdClient
	}

	enabledConfigurationsExporters := []ConfigurationExporter{
		KubernetesExporter,
	}
	if config.GetBool("compliance_config.database_benchmarks.enabled") {
		enabledConfigurationsExporters = append(enabledConfigurationsExporters, DBExporter)
	}

	reporter := NewLogReporter(hostname, "compliance-agent", "compliance", endpoints, context, compression)
	telemetrySender := telemetry.NewSimpleTelemetrySenderFromStatsd(statsdClient)

	agent := NewAgent(telemetrySender, wmeta, filterStore, hostname, AgentOptions{
		ResolverOptions:               resolverOptions,
		ConfigDir:                     configDir,
		Reporter:                      reporter,
		CheckInterval:                 checkInterval,
		EnabledConfigurationExporters: enabledConfigurationsExporters,
		SysProbeClient:                sysProbeClient,
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
func sendRunningMetrics(statsdClient ddgostatsd.ClientInterface, moduleName string) *time.Ticker {
	// Retrieve the agent version using a dedicated package
	tags := []string{
		"version:" + version.AgentVersion,
		constants.CardinalityTagPrefix + "none",
	}

	// Send the metric regularly
	heartbeat := time.NewTicker(15 * time.Second)
	go func() {
		for range heartbeat.C {
			statsdClient.Gauge(fmt.Sprintf("datadog.security_agent.%s.running", moduleName), 1, tags, 1) //nolint:errcheck
		}
	}()

	return heartbeat
}
