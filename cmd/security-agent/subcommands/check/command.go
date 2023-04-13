// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package check

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// CliParams needs to be exported because the compliance subcommand is tightly coupled to this subcommand and tests need to be able to access this type.
type CliParams struct {
	*command.GlobalParams

	args []string

	framework         string
	file              string
	verbose           bool
	report            bool
	overrideRegoInput string
	dumpRegoInput     string
	dumpReports       string
	skipRegoEval      bool
}

func SecurityAgentCommands(globalParams *command.GlobalParams) []*cobra.Command {
	return commandsWrapped(func() core.BundleParams {
		return core.BundleParams{
			ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
			LogParams:    log.LogForOneShot(command.LoggerName, "info", true),
		}
	})
}

func ClusterAgentCommands(bundleParams core.BundleParams) []*cobra.Command {
	return commandsWrapped(func() core.BundleParams {
		return bundleParams
	})
}

func commandsWrapped(bundleParamsFactory func() core.BundleParams) []*cobra.Command {
	checkArgs := &CliParams{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			checkArgs.args = args

			bundleParams := bundleParamsFactory()
			if checkArgs.verbose {
				bundleParams.LogParams = log.LogForOneShot(bundleParams.LogParams.LoggerName(), "trace", true)
			}

			return fxutil.OneShot(RunCheck,
				fx.Supply(checkArgs),
				fx.Supply(bundleParams),
				core.Bundle,
			)
		},
	}

	cmd.Flags().StringVarP(&checkArgs.framework, flags.Framework, "", "", "Framework to run the checks from")
	cmd.Flags().StringVarP(&checkArgs.file, flags.File, "f", "", "Compliance suite file to read rules from")
	cmd.Flags().BoolVarP(&checkArgs.verbose, flags.Verbose, "v", false, "Include verbose details")
	cmd.Flags().BoolVarP(&checkArgs.report, flags.Report, "r", false, "Send report")
	cmd.Flags().StringVarP(&checkArgs.overrideRegoInput, flags.OverrideRegoInput, "", "", "Rego input to use when running rego checks")
	cmd.Flags().StringVarP(&checkArgs.dumpRegoInput, flags.DumpRegoInput, "", "", "Path to file where to dump the Rego input JSON")
	cmd.Flags().StringVarP(&checkArgs.dumpReports, flags.DumpReports, "", "", "Path to file where to dump reports")
	cmd.Flags().BoolVarP(&checkArgs.skipRegoEval, flags.SkipRegoEval, "", false, "Skip rego evaluation")

	return []*cobra.Command{cmd}
}

func RunCheck(log log.Component, config config.Component, checkArgs *CliParams) error {
	if checkArgs.skipRegoEval && checkArgs.dumpReports != "" {
		return errors.New("skipping the rego evaluation does not allow the generation of reports")
	}

	configDir := pkgconfig.Datadog.GetString("compliance_config.dir")
	options := []checks.BuilderOption{}

	if flavor.GetFlavor() == flavor.ClusterAgent {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Info("Waiting for APIClient")
		apiCl, err := apiserver.WaitForAPIClient(ctx)
		if err != nil {
			return err
		}
		options = append(options, checks.MayFail(checks.WithKubernetesClient(apiCl.DynamicCl, "")))
	} else {
		options = append(options, []checks.BuilderOption{
			checks.WithHostRootMount(os.Getenv("HOST_ROOT")),
			checks.MayFail(checks.WithDocker()),
			checks.MayFail(checks.WithAudit()),
			checks.WithConfigDir(configDir),
		}...)
	}

	var ruleID string
	if len(checkArgs.args) != 0 {
		ruleID = checkArgs.args[0]
	}

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return err
	}

	options = append(options, checks.WithHostname(hname))

	stopper := startstop.NewSerialStopper()
	defer stopper.Stop()

	reporter, err := NewCheckReporter(log, config, stopper, checkArgs.report, checkArgs.dumpReports)
	if err != nil {
		return err
	}

	if ruleID != "" {
		log.Infof("Looking for rule with ID=%s", ruleID)
		options = append(options, checks.WithMatchRule(checks.IsRuleID(ruleID)))
	}

	if checkArgs.framework != "" {
		log.Infof("Looking for rules with framework=%s", checkArgs.framework)
		options = append(options, checks.WithMatchSuite(checks.IsFramework(checkArgs.framework)))
	}

	if checkArgs.overrideRegoInput != "" {
		log.Infof("Running on provided rego input: path=%s", checkArgs.overrideRegoInput)
		options = append(options, checks.WithRegoInput(checkArgs.overrideRegoInput))
	}

	if checkArgs.dumpRegoInput != "" {
		options = append(options, checks.WithRegoInputDumpPath(checkArgs.dumpRegoInput))
	}

	metricsEnabled := config.GetBool("compliance_config.metrics.enabled")
	if metricsEnabled {
		// Create a statsd Client
		statsdAddr := os.Getenv("STATSD_URL")
		if statsdAddr == "" {
			// Retrieve statsd host and port from the datadog agent configuration file
			statsdHost := pkgconfig.GetBindHost()
			statsdPort := config.GetInt("dogstatsd_port")
			statsdAddr = fmt.Sprintf("%s:%d", statsdHost, statsdPort)
		}
		statsdClient, err := ddgostatsd.New(statsdAddr)
		if err != nil {
			log.Warnf("Error creating statsd Client: %s", err)
		} else {
			defer statsdClient.Flush()
			options = append(options, checks.WithStatsd(statsdClient))
		}
	}

	options = append(options, checks.WithRegoEvalSkip(checkArgs.skipRegoEval))

	if checkArgs.file != "" {
		err = agent.RunChecksFromFile(reporter, checkArgs.file, options...)
	} else {
		configDir := config.GetString("compliance_config.dir")
		err = agent.RunChecks(reporter, configDir, options...)
	}

	if err != nil {
		log.Errorf("Failed to run checks: %v", err)
		return err
	}

	if err := reporter.dumpReports(); err != nil {
		log.Errorf("Failed to dump reports %v", err)
		return err
	}

	return nil
}
