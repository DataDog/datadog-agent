// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements a specific part of the datadog-agent
// responsible for scanning host and containers and report various
// misconfigurations and compliance issues.
package compliance

import (
	"context"
	"fmt"
	"os"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// hostCCRIDFetchTimeout bounds the cloud provider metadata queries needed to
// resolve the host CCRID, so that we never block the agent startup for long.
const hostCCRIDFetchTimeout = 10 * time.Second

// FetchHostCCRID returns the Canonical Cloud Resource ID of the host, or an
// empty string if the host is not running on a supported cloud provider or if
// the CCRID collection is disabled.
//
// It queries the cloud provider metadata endpoints and is therefore expected to
// be called only once, at startup. The resolved value is then handed over to
// the compliance agent through AgentOptions.HostCCRID.
func FetchHostCCRID(ctx context.Context, conf pkgconfigmodel.Reader) string {
	if !conf.GetBool("collect_ccrid") {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, hostCCRIDFetchTimeout)
	defer cancel()
	cloudProvider, _ := cloudproviders.DetectCloudProvider(ctx, false)
	return cloudproviders.GetHostCCRID(ctx, cloudProvider)
}

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
	secretsComp secrets.Component,
) (*Agent, error) {

	enabled := config.GetBool("compliance_config.enabled")
	configDir := config.GetString("compliance_config.dir")
	metricsEnabled := config.GetBool("compliance_config.metrics.enabled")
	checkInterval := config.GetDuration("compliance_config.check_interval")

	if !enabled {
		return nil, nil
	}

	hostCCRID := FetchHostCCRID(context.Background(), config)

	endpoints, logContext, err := common.NewLogContextCompliance()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(logContext)

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

	reporter := NewLogReporter(hostname, "compliance-agent", "compliance", endpoints, logContext, compression, secretsComp)
	telemetrySender := telemetry.NewSimpleTelemetrySenderFromStatsd(statsdClient)

	agent := NewAgent(telemetrySender, wmeta, filterStore, hostname, AgentOptions{
		ResolverOptions:               resolverOptions,
		HostCCRID:                     hostCCRID,
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
