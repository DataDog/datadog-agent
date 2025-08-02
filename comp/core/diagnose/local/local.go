// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package local contains the code to run local diagnose.
// It is use when building a local flare or runningthe diagnose command locally.
package local

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/diagnose/checkhealth"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/diagnose/ports"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// simpleCollector is a minimal implementation that provides check information from expvars
type simpleCollector struct {
	checks []check.Check
}

func newSimpleCollector() collector.Component {
	return &simpleCollector{
		checks: make([]check.Check, 0),
	}
}

func (sc *simpleCollector) RunCheck(inner check.Check) (checkid.ID, error) {
	sc.checks = append(sc.checks, inner)
	return inner.ID(), nil
}

func (sc *simpleCollector) StopCheck(id checkid.ID) error {
	// Remove check from slice
	for i, c := range sc.checks {
		if c.ID() == id {
			sc.checks = append(sc.checks[:i], sc.checks[i+1:]...)
			break
		}
	}
	return nil
}

func (sc *simpleCollector) MapOverChecks(cb func([]check.Info)) {
	checks := make([]check.Info, len(sc.checks))
	for i, c := range sc.checks {
		checks[i] = c
	}
	cb(checks)
}

func (sc *simpleCollector) GetChecks() []check.Check {
	return sc.checks
}

func (sc *simpleCollector) ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error) {
	// Remove old instances
	oldIDs := make([]checkid.ID, 0)
	for i, c := range sc.checks {
		if c.String() == name {
			oldIDs = append(oldIDs, c.ID())
			sc.checks = append(sc.checks[:i], sc.checks[i+1:]...)
		}
	}

	// Add new instances
	for _, instance := range newInstances {
		sc.checks = append(sc.checks, instance)
	}

	return oldIDs, nil
}

func (sc *simpleCollector) AddEventReceiver(cb collector.EventReceiver) {
	// No-op for simple collector
}

// Run runs the local diagnose suite.
func Run(
	diagnoseComponent diagnose.Component,
	diagnoseConfig diagnose.Config,
	log log.Component,
	senderManager diagnosesendermanager.Component,
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
		diagnose.CheckHealth: func(_ diagnose.Config) []diagnose.Diagnosis {
			// Create a simple collector that can load checks from configurations
			simpleColl := newSimpleCollector()

			// Get integration configs and load checks
			integrationConfigs, err := getLocalIntegrationConfigs(senderManager, wmeta, ac, secretResolver, tagger, config)
			if err != nil {
				return []diagnose.Diagnosis{
					{
						Status:      diagnose.DiagnosisFail,
						Name:        "Error getting integration configs",
						Category:    "check-health",
						Diagnosis:   err.Error(),
						Remediation: "Check your agent configuration and ensure autodiscovery is properly configured.",
						RawError:    err.Error(),
					},
				}
			}

			// Load checks into the simple collector
			if len(integrationConfigs) > 0 {
				senderManagerInstance, err := senderManager.LazyGetSenderManager()
				if err != nil {
					return []diagnose.Diagnosis{
						{
							Status:      diagnose.DiagnosisFail,
							Name:        "Error getting sender manager",
							Category:    "check-health",
							Diagnosis:   err.Error(),
							Remediation: "Check your agent configuration and ensure the sender manager is properly initialized.",
							RawError:    err.Error(),
						},
					}
				}

				// Initialize check scheduler with our simple collector
				checkScheduler := pkgcollector.InitCheckScheduler(option.New(simpleColl), senderManagerInstance, option.None[integrations.Component](), tagger)

				// Load checks from configs
				checks := checkScheduler.GetChecksFromConfigs(integrationConfigs, false)
				for _, c := range checks {
					simpleColl.RunCheck(c)
				}
			}

			// Run the actual check health diagnostics
			return checkhealth.DiagnoseCheckHealth(simpleColl, log)
		},
	}

	integrationConfigs, err := getLocalIntegrationConfigs(senderManager, wmeta, ac, secretResolver, tagger, config)

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

func getLocalIntegrationConfigs(senderManager diagnosesendermanager.Component,
	wmeta option.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	tagger tagger.Component,
	config config.Component) ([]integration.Config, error) {
	senderManagerInstance, err := senderManager.LazyGetSenderManager()
	if err != nil {
		return nil, err
	}

	wmetaInstance, ok := wmeta.Get()
	if !ok {
		return nil, fmt.Errorf("Workload Meta is not available")
	}
	common.LoadComponents(secretResolver, wmetaInstance, ac, config.GetString("confd_path"))
	ac.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to AutoDiscovery.
	pkgcollector.InitCheckScheduler(option.None[collector.Component](), senderManagerInstance, option.None[integrations.Component](), tagger)

	// Load matching configurations (should we use common.AC.GetAllConfigs())
	waitCtx, cancelTimeout := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	diagnoseConfigs, err := common.WaitForAllConfigsFromAD(waitCtx, ac)
	cancelTimeout()
	if err != nil {
		return nil, err
	}

	return diagnoseConfigs, nil
}
