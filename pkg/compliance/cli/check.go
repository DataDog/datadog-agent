// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package cli implements the compliance check command line interface
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// CheckParams needs to be exported because the compliance subcommand is tightly coupled to this subcommand and tests need to be able to access this type.
type CheckParams struct {
	Args []string

	Framework         string
	File              string
	Verbose           bool
	Report            bool
	OverrideRegoInput string
	DumpReports       string
}

// FillCheckFlags fills the check command flags
func FillCheckFlags(flagSet *pflag.FlagSet, checkArgs *CheckParams) {
	flagSet.StringVarP(&checkArgs.Framework, "framework", "", "", "Framework to run the checks from")
	flagSet.StringVarP(&checkArgs.File, "file", "f", "", "Compliance suite file to read rules from")
	flagSet.BoolVarP(&checkArgs.Verbose, "verbose", "v", false, "Include verbose details")
	flagSet.BoolVarP(&checkArgs.Report, "report", "r", false, "Send report")
	flagSet.StringVarP(&checkArgs.OverrideRegoInput, "override-rego-input", "", "", "Rego input to use when running rego checks")
	flagSet.StringVarP(&checkArgs.DumpReports, "dump-reports", "", "", "Path to file where to dump reports")
}

// RunCheck runs a check
func RunCheck(log log.Component, config config.Component, secretsComp secrets.Component, statsdComp statsd.Component, checkArgs *CheckParams, compression logscompression.Component, _ ipc.Component, hostname hostnameinterface.Component) error {
	hname, err := hostname.Get(context.Background())
	if err != nil {
		return err
	}

	var statsdClient ddgostatsd.ClientInterface
	metricsEnabled := config.GetBool("compliance_config.metrics.enabled")
	if metricsEnabled {
		cl, err := statsdComp.Get()
		if err != nil {
			log.Warnf("Error creating statsd Client: %s", err)
		} else {
			statsdClient = cl
			defer cl.Flush()
		}
	}

	if len(checkArgs.Args) == 1 && checkArgs.Args[0] == "k8sconfig" {
		_, resourceData := k8sconfig.LoadConfiguration(context.Background(), os.Getenv("HOST_ROOT"))
		b, _ := json.MarshalIndent(resourceData, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	var resolver compliance.Resolver
	if checkArgs.OverrideRegoInput != "" {
		resolver = newFakeResolver(checkArgs.OverrideRegoInput)
	} else {
		var reflectorStore *compliance.ReflectorStore
		if flavor.GetFlavor() == flavor.ClusterAgent {
			reflectorStore = startComplianceReflectorStore(context.Background())
		}
		resolver = compliance.NewResolver(context.Background(), compliance.ResolverOptions{
			Hostname:           hname,
			HostRoot:           os.Getenv("HOST_ROOT"),
			DockerProvider:     compliance.DefaultDockerProvider,
			LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
			KubernetesProvider: complianceKubernetesProvider,
			ReflectorStore:     reflectorStore,
			StatsdClient:       statsdClient,
		})
	}
	defer resolver.Close()

	configDir := config.GetString("compliance_config.dir")
	var benchDir, benchGlob string
	var ruleFilter compliance.RuleFilter
	if checkArgs.File != "" {
		benchDir, benchGlob = filepath.Dir(checkArgs.File), filepath.Base(checkArgs.File)
	} else if checkArgs.Framework != "" {
		benchDir, benchGlob = configDir, checkArgs.Framework+".yaml"
	} else {
		ruleFilter = compliance.MakeDefaultRuleFilter(hname)
		benchDir, benchGlob = configDir, "*.yaml"
	}

	log.Infof("Loading compliance rules from %s", benchDir)
	benchmarks, err := compliance.LoadBenchmarks(benchDir, benchGlob, func(r *compliance.Rule) bool {
		if ruleFilter != nil && !ruleFilter(r) {
			return false
		}
		if len(checkArgs.Args) > 0 {
			return r.ID == checkArgs.Args[0]
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

	if checkArgs.DumpReports != "" {
		if err := dumpComplianceEvents(checkArgs.DumpReports, events); err != nil {
			log.Error(err)
			return err
		}
	}
	if checkArgs.Report {
		if err := reportComplianceEvents(hname, events, compression, secretsComp); err != nil {
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

func reportComplianceEvents(hostname string, events []*compliance.CheckEvent, compression logscompression.Component, secretsComp secrets.Component) error {
	endpoints, context, err := common.NewLogContextCompliance()
	if err != nil {
		return fmt.Errorf("reporter: could not create log context for compliance: %w", err)
	}
	reporter := compliance.NewLogReporter(hostname, "compliance-agent", "compliance", endpoints, context, compression, secretsComp)
	defer reporter.Stop()
	for _, event := range events {
		reporter.ReportEvent(event)
	}
	return nil
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
