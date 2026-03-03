// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package local contains the code to run local diagnose.
// It is use when building a local flare or runningthe diagnose command locally.
package local

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/diagnose/ports"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Run runs the local diagnose suite.
func Run(
	diagnoseComponent diagnose.Component,
	diagnoseConfig diagnose.Config,
	log log.Component,
	filterStore workloadfilter.Component,
	wmeta option.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	tagger tagger.Component,
	config config.Component,
) (*diagnose.Result, error) {

	localSuite := diagnose.Suites{
		diagnose.PortConflict: func(_ diagnose.Config) []diagnose.Diagnosis {
			return ports.DiagnosePortSuite()
		},
		diagnose.EventPlatformConnectivity: func(_ diagnose.Config) []diagnose.Diagnosis {
			return eventplatformimpl.Diagnose()
		},
		diagnose.AutodiscoveryConnectivity: func(_ diagnose.Config) []diagnose.Diagnosis {
			return connectivity.DiagnoseMetadataAutodiscoveryConnectivity()
		},
		diagnose.CoreEndpointsConnectivity: func(diagCfg diagnose.Config) []diagnose.Diagnosis {
			return connectivity.Diagnose(diagCfg, log)
		},
	}

	integrationConfigs, err := getLocalIntegrationConfigs(filterStore, wmeta, ac, secretResolver, tagger, config)

	if err != nil {
		localSuite[diagnose.CheckDatadog] = func(_ diagnose.Config) []diagnose.Diagnosis {
			return []diagnose.Diagnosis{
				{
					Status:      diagnose.DiagnosisFail,
					Name:        "Error getting integration configs",
					Diagnosis:   err.Error(),
					Remediation: err.Error(),
					RawError:    err.Error(),
				},
			}
		}
	} else {
		localSuite[diagnose.CheckDatadog] = func(_ diagnose.Config) []diagnose.Diagnosis {
			var diagnoses []diagnose.Diagnosis
			for _, integrationConfig := range integrationConfigs {
				checkName := integrationConfig.Name
				instances := pkgcollector.GetChecksByNameForConfigs(checkName, integrationConfigs)
				for _, instance := range instances {
					if instance.Interval() == 0 {
						log.Infof("Ignoring long running check %s", instance.String())
						continue
					}
					instanceDiagnoses := collector.GetInstanceDiagnoses(instance)
					diagnoses = append(diagnoses, instanceDiagnoses...)
				}
			}

			return diagnoses
		}
	}

	return diagnoseComponent.RunLocalSuite(localSuite, diagnoseConfig)
}

func getLocalIntegrationConfigs(
	filterStore workloadfilter.Component,
	wmeta option.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	tagger tagger.Component,
	config config.Component) ([]integration.Config, error) {
	wmetaInstance, ok := wmeta.Get()
	if !ok {
		return nil, errors.New("Workload Meta is not available")
	}
	common.LoadComponents(secretResolver, wmetaInstance, tagger, filterStore, ac, config.GetString("confd_path"))
	ac.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to AutoDiscovery.
	pkgcollector.InitCheckScheduler(option.None[collector.Component](), aggregator.NewNoOpSenderManager(), option.None[integrations.Component](), tagger, filterStore)

	// Load matching configurations (should we use common.AC.GetAllConfigs())
	waitCtx, cancelTimeout := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	diagnoseConfigs, err := common.WaitForAllConfigsFromAD(waitCtx, ac)
	cancelTimeout()
	if err != nil {
		return nil, err
	}

	return diagnoseConfigs, nil
}
