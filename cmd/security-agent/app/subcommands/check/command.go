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
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
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

type checkCliParams struct {
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

func SecAgentCommands(globalParams *common.GlobalParams) []*cobra.Command {
	bp := core.CreateBundleParams(
		"",
		core.WithSecurityAgentConfigFilePaths(globalParams.ConfPathArray),
		core.WithConfigLoadSecurityAgent(true),
	).LogForOneShot(common.LoggerName, "info", true)

	return Commands(bp)
}

// Commands returns a cobra command to run security agent checks
func Commands(bundleParams core.BundleParams) []*cobra.Command {
	checkArgs := &checkCliParams{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			checkArgs.args = args
			return fxutil.OneShot(runCheck,
				fx.Supply(checkArgs),
				fx.Supply(bundleParams),
				core.Bundle,
			)
		},
	}

	cmd.Flags().StringVarP(&checkArgs.framework, "framework", "", "", "Framework to run the checks from")
	cmd.Flags().StringVarP(&checkArgs.file, "file", "f", "", "Compliance suite file to read rules from")
	cmd.Flags().BoolVarP(&checkArgs.verbose, "verbose", "v", false, "Include verbose details")
	cmd.Flags().BoolVarP(&checkArgs.report, "report", "r", false, "Send report")
	cmd.Flags().StringVarP(&checkArgs.overrideRegoInput, "override-rego-input", "", "", "Rego input to use when running rego checks")
	cmd.Flags().StringVarP(&checkArgs.dumpRegoInput, "dump-rego-input", "", "", "Path to file where to dump the Rego input JSON")
	cmd.Flags().StringVarP(&checkArgs.dumpReports, "dump-reports", "", "", "Path to file where to dump reports")
	cmd.Flags().BoolVarP(&checkArgs.skipRegoEval, "skip-rego-eval", "", false, "Skip rego evaluation")

	return []*cobra.Command{cmd}
}

func runCheck(log log.Component, config config.Component, checkArgs *checkCliParams) error {
	if checkArgs.skipRegoEval && checkArgs.dumpReports != "" {
		return errors.New("skipping the rego evaluation does not allow the generation of reports")
	}

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
		}...)

		if pkgconfig.IsKubernetes() {
			nodeLabels, err := agent.WaitGetNodeLabels()
			if err != nil {
				log.Error(err)
			} else {
				options = append(options, checks.WithNodeLabels(nodeLabels))
			}
		}
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

	reporter, err := NewCheckReporter(stopper, checkArgs.report, checkArgs.dumpReports)
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
