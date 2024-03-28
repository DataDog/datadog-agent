// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package standalone

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/jmx"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// ExecJMXCommandConsole runs the provided JMX command name on the selected checks, and
// reports with the ConsoleReporter to the agent's `log.Info`.
// The common utils, including AutoConfig, must have already been initialized.
func ExecJMXCommandConsole(command string, selectedChecks []string, logLevel string, configs []integration.Config, wmeta workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, agentAPI internalAPI.Component, collector optional.Option[collector.Component], jmxLogger jmxlogger.Component) error {
	return execJmxCommand(command, selectedChecks, jmxfetch.ReporterConsole, jmxLogger.JMXInfo, logLevel, configs, wmeta, taggerComp, ac, senderManager, agentAPI, collector, jmxLogger)
}

// ExecJmxListWithMetricsJSON runs the JMX command with "with-metrics", reporting
// the data as a JSON on the console. It is used by the `check jmx` cli command
// of the Agent.
// The common utils, including AutoConfig, must have already been initialized.
func ExecJmxListWithMetricsJSON(selectedChecks []string, logLevel string, configs []integration.Config, wmeta workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, agentAPI internalAPI.Component, collector optional.Option[collector.Component], jmxLogger jmxlogger.Component) error {
	// don't pollute the JSON with the log pattern.
	out := func(a ...interface{}) {
		fmt.Println(a...)
	}
	return execJmxCommand("list_with_metrics", selectedChecks, jmxfetch.ReporterJSON, out, logLevel, configs, wmeta, taggerComp, ac, senderManager, agentAPI, collector, jmxLogger)
}

// ExecJmxListWithRateMetricsJSON runs the JMX command with "with-rate-metrics", reporting
// the data as a JSON on the console. It is used by the `check jmx --rate` cli command
// of the Agent.
// The common utils, including AutoConfig, must have already been initialized.
func ExecJmxListWithRateMetricsJSON(selectedChecks []string, logLevel string, configs []integration.Config, wmeta workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, agentAPI internalAPI.Component, collector optional.Option[collector.Component], jmxLogger jmxlogger.Component) error {
	// don't pollute the JSON with the log pattern.
	out := func(a ...interface{}) {
		fmt.Println(a...)
	}
	return execJmxCommand("list_with_rate_metrics", selectedChecks, jmxfetch.ReporterJSON, out, logLevel, configs, wmeta, taggerComp, ac, senderManager, agentAPI, collector, jmxLogger)
}

// execJmxCommand runs the provided JMX command name on the selected checks.
// The common utils, including AutoConfig, must have already been initialized.
func execJmxCommand(command string,
	selectedChecks []string,
	reporter jmxfetch.JMXReporter,
	output func(...interface{}),
	logLevel string,
	configs []integration.Config,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	senderManager sender.DiagnoseSenderManager,
	agentAPI internalAPI.Component,
	collector optional.Option[collector.Component],
	logger jmxlogger.Component) error {
	// start the cmd HTTP server
	if err := agentAPI.StartServer(wmeta, taggerComp, ac, optional.NewNoneOption[logsAgent.Component](), senderManager, collector); err != nil {
		return fmt.Errorf("Error while starting api server, exiting: %v", err)
	}

	runner := jmxfetch.NewJMXFetch(logger)

	runner.Reporter = reporter
	runner.Command = command
	runner.IPCPort = agentAPI.ServerAddress().Port
	runner.Output = output
	runner.LogLevel = logLevel

	loadJMXConfigs(runner, selectedChecks, configs)

	err := runner.Start(false)
	if err != nil {
		return err
	}

	err = runner.Wait()
	if err != nil {
		return err
	}

	fmt.Printf(
		"JMXFetch exited successfully. If nothing was displayed please check your configuration and flags, "+
			"or re-run the command with a more verbose log level (current log level: '%s').\n",
		logLevel,
	)
	return nil
}

func loadJMXConfigs(runner *jmxfetch.JMXFetch, selectedChecks []string, configs []integration.Config) {
	fmt.Println("Loading configs...")

	includeEverything := len(selectedChecks) == 0

	for _, c := range configs {
		if check.IsJMXConfig(c) && (includeEverything || configIncluded(c, selectedChecks)) {
			fmt.Println("Config ", c.Name, " was loaded.")
			instances := []integration.Data{}

			// Retain only JMX instances
			for _, instance := range c.Instances {
				if !check.IsJMXInstance(c.Name, instance, c.InitConfig) {
					continue
				}
				instances = append(instances, instance)
			}
			c.Instances = instances

			jmx.AddScheduledConfig(c)
			runner.ConfigureFromInitConfig(c.InitConfig) //nolint:errcheck
			for _, instance := range c.Instances {
				runner.ConfigureFromInstance(instance) //nolint:errcheck
			}
		}
	}
}

func configIncluded(config integration.Config, selectedChecks []string) bool {
	for _, c := range selectedChecks {
		if strings.EqualFold(config.Name, c) {
			return true
		}
	}
	return false
}
