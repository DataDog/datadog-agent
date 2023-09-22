// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"context"
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
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("check-datadog", diagnose)
}

func diagnose(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
	if diagCfg.RunningInAgentProcess && common.Coll != nil {
		return diagnoseChecksInAgentProcess()
	}

	return diagnoseChecksInCLIProcess(diagCfg)
}

func getInstanceDiagnoses(instance check.Check) []diagnosis.Diagnosis {

	// Get diagnoses
	diagnoses, err := instance.GetDiagnoses()
	if err != nil {
		// return as diagnosis.DiagnosisUnexpectedError diagnosis
		return []diagnosis.Diagnosis{
			{
				Result:    diagnosis.DiagnosisUnexpectedError,
				Name:      string(instance.ID()),
				Category:  instance.String(),
				Diagnosis: "Check Dianose failes with unexpected errors",
				RawError:  err.Error(),
			},
		}
	}

	// Set category as check name if it was not set
	if len(diagnoses) > 0 {
		for i, d := range diagnoses {
			if len(d.Category) == 0 {
				diagnoses[i].Category = instance.String()
			}
		}
	}

	return diagnoses
}

func diagnoseChecksInAgentProcess() []diagnosis.Diagnosis {
	var diagnoses []diagnosis.Diagnosis

	// get list of checks
	checks := common.Coll.GetChecks()

	// get diagnoses from each
	for _, ch := range checks {
		if ch.Interval() == 0 {
			pkglog.Infof("Ignoring long running check %s", ch.String())
			continue
		}
		instanceDiagnoses := getInstanceDiagnoses(ch)
		diagnoses = append(diagnoses, instanceDiagnoses...)
	}

	return diagnoses
}

func diagnoseChecksInCLIProcess(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
	// other choices
	// 	run() github.com\DataDog\datadog-agent\pkg\cli\subcommands\check\command.go
	//  runCheck() github.com\DataDog\datadog-agent\cmd\agent\gui\checks.go

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisFail,
				Name:        "Host name detection",
				Diagnosis:   "Failed to get host name and cannot continue to run checks diagnostics",
				Remediation: "Please validate host environment",
				RawError:    err.Error(),
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

	common.LoadComponents(context.Background(), aggregator.GetSenderManager(), pkgconfig.Datadog.GetString("confd_path"))
	common.AC.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.  NOTE: we do not start common.Coll, either.
	collector.InitCheckScheduler(common.Coll, aggregator.GetSenderManager())

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
				RawError:    err.Error(),
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
			if instance.Interval() == 0 {
				pkglog.Infof("Ignoring long running check %s", instance.String())
				continue
			}
			instanceDiagnoses := getInstanceDiagnoses(instance)
			diagnoses = append(diagnoses, instanceDiagnoses...)
		}
	}

	return diagnoses
}
