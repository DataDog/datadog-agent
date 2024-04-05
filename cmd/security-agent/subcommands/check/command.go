// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package check holds check related files
package check

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
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
	dumpReports       string
}

// SecurityAgentCommands returns the security agent commands
func SecurityAgentCommands(globalParams *command.GlobalParams) []*cobra.Command {
	return commandsWrapped(func() core.BundleParams {
		return core.BundleParams{
			ConfigParams:         config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
			LogParams:            logimpl.ForOneShot(command.LoggerName, "info", true),
		}
	})
}

// ClusterAgentCommands returns the cluster agent commands
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
				bundleParams.LogParams = logimpl.ForOneShot(bundleParams.LogParams.LoggerName(), "trace", true)
			}

			return fxutil.OneShot(RunCheck,
				fx.Supply(checkArgs),
				fx.Supply(bundleParams),
				core.Bundle(),
				dogstatsd.ClientBundle,
			)
		},
	}

	cmd.Flags().StringVarP(&checkArgs.framework, "framework", "", "", "Framework to run the checks from")
	cmd.Flags().StringVarP(&checkArgs.file, "file", "f", "", "Compliance suite file to read rules from")
	cmd.Flags().BoolVarP(&checkArgs.verbose, "verbose", "v", false, "Include verbose details")
	cmd.Flags().BoolVarP(&checkArgs.report, "report", "r", false, "Send report")
	cmd.Flags().StringVarP(&checkArgs.overrideRegoInput, "override-rego-input", "", "", "Rego input to use when running rego checks")
	cmd.Flags().StringVarP(&checkArgs.dumpReports, "dump-reports", "", "", "Path to file where to dump reports")

	return []*cobra.Command{cmd}
}

// RunCheck runs a check
func RunCheck(log log.Component, config config.Component, _ secrets.Component, statsd statsd.Component, checkArgs *CliParams) error {
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return err
	}

	var statsdClient ddgostatsd.ClientInterface
	metricsEnabled := config.GetBool("compliance_config.metrics.enabled")
	if metricsEnabled {
		cl, err := statsd.CreateForHostPort(pkgconfig.GetBindHost(), config.GetInt("dogstatsd_port"))
		if err != nil {
			log.Warnf("Error creating statsd Client: %s", err)
		} else {
			statsdClient = cl
			defer cl.Flush()
		}
	}

	if len(checkArgs.args) == 1 && checkArgs.args[0] == "k8sconfig" {
		_, resourceData := k8sconfig.LoadConfiguration(context.Background(), os.Getenv("HOST_ROOT"))
		b, _ := json.MarshalIndent(resourceData, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	var resolver compliance.Resolver
	if checkArgs.overrideRegoInput != "" {
		resolver = newFakeResolver(checkArgs.overrideRegoInput)
	} else if flavor.GetFlavor() == flavor.ClusterAgent {
		resolver = compliance.NewResolver(context.Background(), compliance.ResolverOptions{
			Hostname:           hname,
			DockerProvider:     compliance.DefaultDockerProvider,
			LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
			KubernetesProvider: complianceKubernetesProvider,
			StatsdClient:       statsdClient,
		})
	} else {
		resolver = compliance.NewResolver(context.Background(), compliance.ResolverOptions{
			Hostname:           hname,
			HostRoot:           os.Getenv("HOST_ROOT"),
			DockerProvider:     compliance.DefaultDockerProvider,
			LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
			StatsdClient:       statsdClient,
		})
	}
	defer resolver.Close()

	configDir := config.GetString("compliance_config.dir")
	var benchDir, benchGlob string
	var ruleFilter compliance.RuleFilter
	if checkArgs.file != "" {
		benchDir, benchGlob = filepath.Dir(checkArgs.file), filepath.Base(checkArgs.file)
	} else if checkArgs.framework != "" {
		benchDir, benchGlob = configDir, fmt.Sprintf("%s.yaml", checkArgs.framework)
	} else {
		ruleFilter = compliance.DefaultRuleFilter
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
			case rule.IsXCCDF():
				ruleEvents = compliance.EvaluateXCCDFRule(context.Background(), hname, statsdClient, benchmark, rule)
			case rule.IsRego():
				inputs, err := resolver.ResolveInputs(context.Background(), rule)
				if err != nil {
					ruleEvents = append(ruleEvents, compliance.CheckEventFromError(compliance.RegoEvaluator, rule, benchmark, err))
				} else {
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

	if checkArgs.dumpReports != "" {
		if err := dumpComplianceEvents(checkArgs.dumpReports, events); err != nil {
			log.Error(err)
			return err
		}
	}
	if checkArgs.report {
		if err := reportComplianceEvents(log, config, events); err != nil {
			log.Error(err)
			return err
		}
	}
	return nil
}

func dumpComplianceEvents(reportFile string, events []*compliance.CheckEvent) error {
	eventsMap := make(map[string][]*compliance.CheckEvent)
	for _, event := range events {
		eventsMap[event.RuleID] = append(eventsMap[event.RuleID], event)
	}
	b, err := json.MarshalIndent(eventsMap, "", "\t")
	if err != nil {
		return fmt.Errorf("could not marshal events map: %w", err)
	}
	if err := os.WriteFile(reportFile, b, 0o644); err != nil {
		return fmt.Errorf("could not write report file in %q: %w", reportFile, err)
	}
	return nil
}

func reportComplianceEvents(log log.Component, config config.Component, events []*compliance.CheckEvent) error {
	hostnameDetected, err := utils.GetHostnameWithContextAndFallback(context.Background())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	runPath := config.GetString("compliance_config.run_path")
	endpoints, context, err := common.NewLogContextCompliance()
	if err != nil {
		return fmt.Errorf("reporter: could not reate log context for compliance: %w", err)
	}
	reporter := compliance.NewLogReporter(hostnameDetected, "compliance-agent", "compliance", runPath, endpoints, context)
	defer reporter.Stop()
	for _, event := range events {
		reporter.ReportEvent(event)
	}
	return nil
}

func complianceKubernetesProvider(_ctx context.Context) (dynamic.Interface, discovery.DiscoveryInterface, error) {
	ctx, cancel := context.WithTimeout(_ctx, 2*time.Second)
	defer cancel()
	apiCl, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	return apiCl.DynamicCl, apiCl.Cl.Discovery(), nil
}

type fakeResolver struct {
	regoInputPath string
}

func newFakeResolver(regoInputPath string) compliance.Resolver {
	return &fakeResolver{regoInputPath}
}

func (r *fakeResolver) ResolveInputs(_ context.Context, rule *compliance.Rule) (compliance.ResolvedInputs, error) {
	var fixtures map[string]map[string]interface{}
	data, err := os.ReadFile(r.regoInputPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return nil, fmt.Errorf("could not unmarshal faked rego input: %w", err)
	}
	fixture, ok := fixtures[rule.ID]
	if !ok {
		return nil, fmt.Errorf("could not find fixtures for rule %q", rule.ID)
	}
	var resolvingContext compliance.ResolvingContext
	if err := jsonRountrip(fixture["context"], &resolvingContext); err != nil {
		return nil, err
	}
	delete(fixture, "context")
	return compliance.NewResolvedInputs(resolvingContext, fixture)
}

func jsonRountrip(i interface{}, v interface{}) error {
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &v)
}

func (r *fakeResolver) Close() {
}
