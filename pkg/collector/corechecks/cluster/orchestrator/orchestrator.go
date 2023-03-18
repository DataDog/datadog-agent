// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//go:build kubeapiserver && orchestrator

package orchestrator

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
)

const (
	maximumWaitForAPIServer = 10 * time.Second
	collectionInterval      = 10 * time.Second
)

func init() {
	core.RegisterCheck(orchestrator.CheckName, OrchestratorFactory)
}

// OrchestratorInstance is the config of the orchestrator check instance.
type OrchestratorInstance struct {
	LeaderSkip bool `yaml:"skip_leader_election"`
	// Collectors defines the resource type collectors.
	// Example: Enable services and nodes collectors.
	// collectors:
	//   - nodes
	//   - services
	Collectors []string `yaml:"collectors"`
	// CRDCollectors defines collectors for custom Kubernetes resource types.
	// crd_collectors:
	//   - datadoghq.com/v1alpha1/datadogmetrics
	//   - stable.example.com/v1/crontabs
	CRDCollectors           []string `yaml:"crd_collectors"`
	ExtraSyncTimeoutSeconds int      `yaml:"extra_sync_timeout_seconds"`
}

func (c *OrchestratorInstance) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// OrchestratorCheck wraps the config and the informers needed to run the check
type OrchestratorCheck struct {
	core.CheckBase
	orchestratorConfig *orchcfg.OrchestratorConfig
	instance           *OrchestratorInstance
	collectorBundle    *CollectorBundle
	stopCh             chan struct{}
	clusterID          string
	groupID            *atomic.Int32
	isCLCRunner        bool
	apiClient          *apiserver.APIClient
}

func newOrchestratorCheck(base core.CheckBase, instance *OrchestratorInstance) *OrchestratorCheck {
	return &OrchestratorCheck{
		CheckBase:          base,
		orchestratorConfig: orchcfg.NewDefaultOrchestratorConfig(),
		instance:           instance,
		stopCh:             make(chan struct{}),
		groupID:            atomic.NewInt32(rand.Int31()),
		isCLCRunner:        config.IsCLCRunner(),
	}
}

// OrchestratorFactory returns the orchestrator check
func OrchestratorFactory() check.Check {
	return newOrchestratorCheck(
		core.NewCheckBase(orchestrator.CheckName),
		&OrchestratorInstance{},
	)
}

// Interval returns the scheduling time for the check.
func (o *OrchestratorCheck) Interval() time.Duration {
	return collectionInterval
}

// Configure configures the orchestrator check
func (o *OrchestratorCheck) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	o.BuildID(integrationConfigDigest, config, initConfig)

	err := o.CommonConfigure(integrationConfigDigest, initConfig, config, source)
	if err != nil {
		return err
	}

	err = o.orchestratorConfig.Load()
	if err != nil {
		return err
	}

	if !o.orchestratorConfig.OrchestrationCollectionEnabled {
		return errors.New("orchestrator check is configured but the feature is disabled")
	}
	if o.orchestratorConfig.KubeClusterName == "" {
		return errors.New("orchestrator check is configured but the cluster name is empty")
	}

	// load instance level config
	err = o.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse check instance config")
		return err
	}

	o.clusterID, err = clustername.GetClusterID()
	if err != nil {
		return err
	}

	// Reuse the common API Server client to share the cache
	// Due to how init is done, we cannot use GetAPIClient in `Run()` method
	// So we are waiting for a reasonable amount of time here in case.
	// We cannot wait forever as there's no way to be notified of shutdown
	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()

	o.apiClient, err = apiserver.WaitForAPIClient(apiCtx)
	if err != nil {
		return err
	}

	// Create a new bundle for the check.
	o.collectorBundle = NewCollectorBundle(o)

	// Initialize collectors.
	return o.collectorBundle.Initialize()
}

// Run runs the orchestrator check
func (o *OrchestratorCheck) Run() error {
	// access serializer
	sender, err := o.GetSender()
	if err != nil {
		return err
	}

	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// we also do a safety check for dedicated runners to avoid trying the leader election
	if !o.isCLCRunner || !o.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !config.Datadog.GetBool("leader_election") {
			return log.Error("Leader Election not enabled. The cluster-agent will not run the check.")
		}

		leader, errLeader := cluster.RunLeaderElection()
		if errLeader != nil {
			if errLeader == apiserver.ErrNotLeader {
				log.Debugf("Not leader (leader is %q). Skipping the Orchestrator check", leader)
				return nil
			}

			_ = o.Warn("Leader Election error. Not running the Orchestrator check.")
			return err
		}

		log.Tracef("Current leader: %q, running the Orchestrator check", leader)
	}

	// Run all collectors.
	o.collectorBundle.Run(sender)

	return nil
}

// Cancel cancels the orchestrator check
func (o *OrchestratorCheck) Cancel() {
	log.Infof("Shutting down informers used by the check '%s'", o.ID())
	close(o.stopCh)
}
