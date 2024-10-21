// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnose provides the diagnose functionality for the Agent.
package diagnose

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

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

func diagnoseChecksInAgentProcess(collector collector.Component) []diagnosis.Diagnosis {
	var diagnoses []diagnosis.Diagnosis

	// get list of checks
	checks := collector.GetChecks()

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

func diagnoseChecksInCLIProcess(_ diagnosis.Config, senderManager diagnosesendermanager.Component, _ integrations.Component, secretResolver secrets.Component, wmeta optional.Option[workloadmeta.Component], ac autodiscovery.Component, tagger tagger.Component) []diagnosis.Diagnosis {
	// other choices
	// 	run() github.com\DataDog\datadog-agent\pkg\cli\subcommands\check\command.go
	//  runCheck() github.com\DataDog\datadog-agent\cmd\agent\gui\checks.go

	senderManagerInstance, err := senderManager.LazyGetSenderManager()
	if err != nil {
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisFail,
				Name:        err.Error(),
				Diagnosis:   err.Error(),
				Remediation: err.Error(),
				RawError:    err.Error(),
			},
		}
	}

	wmetaInstance, ok := wmeta.Get()
	if !ok {
		errMsg := "Workload Meta is not available"
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisFail,
				Name:        errMsg,
				Diagnosis:   errMsg,
				Remediation: errMsg,
			},
		}
	}
	// Initializing the aggregator with a flush interval of 0 (to disable the flush goroutines)
	common.LoadComponents(secretResolver, wmetaInstance, ac, pkgconfigsetup.Datadog().GetString("confd_path"))
	ac.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.
	pkgcollector.InitCheckScheduler(optional.NewNoneOption[collector.Component](), senderManagerInstance, optional.NewNoneOption[integrations.Component](), tagger)

	// Load matching configurations (should we use common.AC.GetAllConfigs())
	waitCtx, cancelTimeout := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	diagnoseConfigs, err := common.WaitForAllConfigsFromAD(waitCtx, ac)
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
		instances := pkgcollector.GetChecksByNameForConfigs(checkName, diagnoseConfigs)
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
