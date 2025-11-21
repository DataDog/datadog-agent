// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package check holds check related files
package check

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type CliParams struct {
	*command.GlobalParams

	args []string

	framework         string
	file              string
	verbose           bool
	report            bool
	overrideRegoInput string
	dumpReports       string
}

// SecurityAgentCommands returns security agent commands
func SecurityAgentCommands(globalParams *command.GlobalParams) []*cobra.Command {
	return commandsWrapped(func() core.BundleParams {
		return core.BundleParams{
			ConfigParams:         config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
			LogParams:            log.ForOneShot(command.LoggerName, "info", true),
		}
	})
}

// ClusterAgentCommands returns cluster agent commands
func ClusterAgentCommands(_ core.BundleParams) []*cobra.Command {
	return nil
}

func commandsWrapped(bundleParamsFactory func() core.BundleParams) []*cobra.Command {
	checkArgs := &CliParams{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(_ *cobra.Command, args []string) error {
			checkArgs.args = args

			bundleParams := bundleParamsFactory()
			if checkArgs.verbose {
				bundleParams.LogParams = log.ForOneShot(bundleParams.LogParams.LoggerName(), "trace", true)
			}

			return fxutil.OneShot(RunCheck,
				fx.Supply(checkArgs),
				fx.Supply(bundleParams),
				core.Bundle(),
				secretsfx.Module(),
				logscompressionfx.Module(),
				statsd.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	return []*cobra.Command{cmd}
}

// RunCheck runs a check
func RunCheck(log log.Component, config config.Component, _ secrets.Component, statsdComp statsd.Component, checkArgs *CliParams, compression logscompression.Component, ipc ipc.Component) error {
	var hname string
	var err error

	// compliance_config.metrics.enabled not supported on Windows

	var resolver compliance.Resolver
	resolver = compliance.NewResolver(context.Background(), compliance.ResolverOptions{
		Hostname: hname,
		HostRoot: os.Getenv("HOST_ROOT"),
	})
	defer resolver.Close()

	configDir := config.GetString("compliance_config.dir")
	var benchDir, benchGlob string
	var ruleFilter compliance.RuleFilter
	if checkArgs.file != "" {
		benchDir, benchGlob = filepath.Dir(checkArgs.file), filepath.Base(checkArgs.file)
	} else if checkArgs.framework != "" {
		benchDir, benchGlob = configDir, fmt.Sprintf("%s.yaml", checkArgs.framework)
	} else {
		ruleFilter = compliance.MakeDefaultRuleFilter(ipc)
		benchDir, benchGlob = configDir, "*.yaml"
	}

	log.Infof("Loading compliance rules from %s", benchDir)
	benchmarks, err := compliance.LoadBenchmarks(benchDir, benchGlob, func(r *compliance.Rule) bool {
		if ruleFilter != nil && !ruleFilter(r) {
			return false
		}
		if len(checkArgs.args) > 0 {
			return r.ID == checkArgs.args[0]
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("could not load benchmark files %q: %w", filepath.Join(benchDir, benchGlob), err)
	}
	if len(benchmarks) == 0 {
		return fmt.Errorf("could not find any benchmark in %q", filepath.Join(benchDir, benchGlob))
	}

	events := make([]*compliance.CheckEvent, 0)
	for _, benchmark := range benchmarks {
		for _, rule := range benchmark.Rules {
			log.Infof("Running check: %s: %s [version=%s]", rule.ID, rule.Description, benchmark.Version)
			var ruleEvents []*compliance.CheckEvent
			switch {
			case rule.IsRego():
				// Extracts data for the inputs of the rule (e.g. Registry values).
				inputs, err := resolver.ResolveInputs(context.Background(), rule)
				if err != nil {
					ruleEvents = append(ruleEvents, compliance.CheckEventFromError(compliance.RegoEvaluator, rule, benchmark, err))
				} else {
					// Evalute the benchmark rule with Rego.
					ruleEvents = compliance.EvaluateRegoRule(context.Background(), inputs, benchmark, rule)
				}
			}

			for _, event := range ruleEvents {
				b, _ := json.MarshalIndent(event, "", "\t")
				fmt.Println(string(b))
				if event.Result != compliance.CheckSkipped {
					events = append(events, event)
				}
			}
		}
	}

	return nil
}
