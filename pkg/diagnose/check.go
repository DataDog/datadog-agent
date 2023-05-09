// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func init() {
	diagnosis.Register("check-datadog", diagnose)
}

// Currently diagnose is implemented to run in the CLI process,
// in the next version will connect to the running agent service to get diagnoses
// for scheduled checks without running them
func diagnose(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
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
	forwarder := forwarder.NewDefaultForwarder(config.Datadog, forwarder.NewOptions(config.Datadog, nil))
	aggregator.InitAndStartAgentDemultiplexer(forwarder, opts, hostnameDetected)

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

			instanceName := string(instance.ID())

			// Run check
			runErr := instance.Run()

			// Get diagnoses
			checkDiagnoses, diagnoseErr := instance.GetDiagnoses()
			if diagnoseErr == nil && len(checkDiagnoses) > 0 {
				for _, d := range checkDiagnoses {
					if len(d.Category) == 0 {
						d.Category = instanceName
					}
					diagnoses = append(diagnoses, d)
				}
			} else if diagnoseErr != nil {
				// Check Diagnose method return error
				diagnoses = append(diagnoses, diagnosis.Diagnosis{
					Result:    diagnosis.DiagnosisUnexpectedError,
					Name:      instanceName,
					Category:  instanceName,
					Diagnosis: "Check Dianose failes with unexpected errors",
					RawError:  runErr,
				})
			}
		}
	}

	return diagnoses
}
