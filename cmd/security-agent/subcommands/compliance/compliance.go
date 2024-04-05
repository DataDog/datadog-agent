// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// StartCompliance runs the compliance sub-agent running compliance benchmarks
// and checks.
func StartCompliance(log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, hostname string, stopper startstop.Stopper, statsdClient ddgostatsd.ClientInterface, senderManager sender.SenderManager, wmeta workloadmeta.Component) (*compliance.Agent, error) {
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

	resolverOptions := compliance.ResolverOptions{
		Hostname:           hostname,
		HostRoot:           os.Getenv("HOST_ROOT"),
		DockerProvider:     compliance.DefaultDockerProvider,
		LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
	}

	if metricsEnabled {
		resolverOptions.StatsdClient = statsdClient
	}

	var sysProbeClient *http.Client
	if config := sysprobeconfig.SysProbeObject(); config != nil && config.SocketAddress != "" {
		sysProbeClient = newSysProbeClient(config.SocketAddress)
	}

	enabledConfigurationsExporters := []compliance.ConfigurationExporter{
		compliance.KubernetesExporter,
	}
	if config.GetBool("compliance_config.database_benchmarks.enabled") {
		enabledConfigurationsExporters = append(enabledConfigurationsExporters, compliance.DBExporter)
	}

	reporter := compliance.NewLogReporter(hostname, "compliance-agent", "compliance", runPath, endpoints, context)
	agent := compliance.NewAgent(senderManager, wmeta, compliance.AgentOptions{
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

func newSysProbeClient(address string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", address)
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}
