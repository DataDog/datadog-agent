// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"context"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/log"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func init() {
	diagnosis.Register("check-datadog", diagnose)
}

func diagnose(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
	if diagCfg.RunningInAgentProcess {
		return diagnoseInAgentProcess(diagCfg)
	}

	return diagnoseInCLIProcess(diagCfg)
}

func partitionChecks(checks []check.Check) [][]check.Check {
	instancesMap := map[string][]check.Check{}

	// partition per check (instances of a check groupped by check/integration)
	for _, c := range checks {
		checkName := c.String()
		instances, exists := instancesMap[checkName]
		if exists {
			instances = append(instances, c)
		} else {
			instances = make([]check.Check, 1)
			instances[0] = c
		}
		instancesMap[checkName] = instances
	}

	// copy map to array
	instancesArr := make([][]check.Check, 0, len(instancesMap))
	for _, instances := range instancesMap {
		instancesArr = append(instancesArr, instances)
	}

	// sort this array
	sort.Slice(instancesArr, func(i, j int) bool {
		return instancesArr[i][0].String() < instancesArr[j][0].String()
	})

	return instancesArr
}

func getInstanceDiagnoses(instance check.Check) []diagnosis.Diagnosis {

	// Get diagnoses
	diagnoses, err := instance.GetDiagnoses()
	if err == nil && len(diagnoses) > 0 {
		// Set category as check name if it was not set
		for i, d := range diagnoses {
			if len(d.Category) == 0 {
				diagnoses[i].Category = instance.String()
			}
		}
	} else if err != nil {
		// Check Diagnose method return error
		diagnoses = append(diagnoses, diagnosis.Diagnosis{
			Result:    diagnosis.DiagnosisUnexpectedError,
			Name:      string(instance.ID()),
			Category:  instance.String(),
			Diagnosis: "Check Dianose failes with unexpected errors",
			RawError:  err,
		})
	}

	return diagnoses
}

func diagnoseInAgentProcess(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
	// get checks
	checks := common.Coll.GetChecks()

	partInstances := partitionChecks(checks)

	var diagnoses []diagnosis.Diagnosis
	for _, instances := range partInstances {
		for _, instance := range instances {
			instanceDiagnoses := getInstanceDiagnoses(instance)
			if len(instanceDiagnoses) > 0 {
				diagnoses = append(diagnoses, instanceDiagnoses...)
			}
		}
	}

	return diagnoses
}

// Currently diagnose is implemented to run in the CLI process,
// in the next version will connect to the running agent service to get diagnoses
// for scheduled checks without running them
func diagnoseInCLIProcess(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
	// other choices
	// 	run() github.com\DataDog\datadog-agent\pkg\cli\subcommands\check\command.go
	//  runCheck() github.com\DataDog\datadog-agent\cmd\agent\gui\checks.go

	// Always disable SBOM collection in `check` command to avoid BoltDB flock issue
	// and consuming CPU & Memory for asynchronous scans that would not be shown in `agent check` output.
	pkgconfig.Datadog.Set("container_image_collection.sbom.enabled", "false")

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisFail,
				Name:        "Host name detection",
				Diagnosis:   "Failed to get host name and cannot continue to run checks diagnostics",
				Remediation: "Please validate host environment",
				RawError:    err,
			},
		}
	}

	// Initializing the aggregator with a flush interval of 0 (to disable the flush goroutines)
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 0
	opts.DontStartForwarders = true
	opts.UseNoopEventPlatformForwarder = true
	opts.UseNoopOrchestratorForwarder = true
	log := log.NewTemporaryLoggerWithoutInit()

	forwarder := forwarder.NewDefaultForwarder(config.Datadog, log, forwarder.NewOptions(config.Datadog, log, nil))
	aggregator.InitAndStartAgentDemultiplexer(log, forwarder, opts, hostnameDetected)

	common.LoadComponents(context.Background(), pkgconfig.Datadog.GetString("confd_path"))
	common.AC.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.  NOTE: we do not start common.Coll, either.
	collector.InitCheckScheduler(common.Coll)

	// Load matching configurations (should we use common.AC.GetAllConfigs())
	waitCtx, cancelTimeout := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	diagnoseConfigs, err := common.WaitForAllConfigsFromAD(waitCtx)
	cancelTimeout()
	if err != nil {
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisFail,
				Name:        "Check configuration location",
				Diagnosis:   "Failed to find or load checks configurations",
				Remediation: "Please validate Agent configuration",
				RawError:    err,
			},
		}
	}

	// Is there checks to diagnose
	if len(diagnoseConfigs) == 0 {
		return nil
	}

	var diagnoses []diagnosis.Diagnosis
	for _, diagnoseConfig := range diagnoseConfigs {
		checkName := diagnoseConfig.Name
		instances := collector.GetChecksByNameForConfigs(checkName, diagnoseConfigs)
		for _, instance := range instances {
			instanceDiagnoses := getInstanceDiagnoses(instance)
			if len(instanceDiagnoses) > 0 {
				diagnoses = append(diagnoses, instanceDiagnoses...)
			}
		}
	}

	return diagnoses
}
