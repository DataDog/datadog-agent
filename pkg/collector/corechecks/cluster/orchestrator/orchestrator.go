// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//go:build kubeapiserver && orchestrator

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"go.uber.org/atomic"
	"gopkg.in/yaml.v2"
	"k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	vpai "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// CheckName is the name of the check
	CheckName = orchestrator.CheckName

	maximumWaitForAPIServer = 10 * time.Second
	collectionInterval      = 10 * time.Second
	defaultResyncInterval   = 300 * time.Second
)

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
	orchestratorConfig          *orchcfg.OrchestratorConfig
	instance                    *OrchestratorInstance
	collectorBundle             *CollectorBundle
	wlmStore                    workloadmeta.Component
	cfg                         configcomp.Component
	tagger                      tagger.Component
	stopCh                      chan struct{}
	clusterID                   string
	groupID                     *atomic.Int32
	isCLCRunner                 bool
	apiClient                   *apiserver.APIClient
	orchestratorInformerFactory *collectors.OrchestratorInformerFactory
	agentVersion                *model.AgentVersion
}

func newOrchestratorCheck(base core.CheckBase, instance *OrchestratorInstance, cfg configcomp.Component, wlmStore workloadmeta.Component, tagger tagger.Component) *OrchestratorCheck {
	agentVersion, err := version.Agent()
	if err != nil {
		log.Warnf("Failed to get agent version: %s", err)
	}

	return &OrchestratorCheck{
		CheckBase:   base,
		instance:    instance,
		wlmStore:    wlmStore,
		tagger:      tagger,
		cfg:         cfg,
		stopCh:      make(chan struct{}),
		groupID:     atomic.NewInt32(rand.Int31()),
		isCLCRunner: pkgconfigsetup.IsCLCRunner(pkgconfigsetup.Datadog()),
		agentVersion: &model.AgentVersion{
			Major:  agentVersion.Major,
			Minor:  agentVersion.Minor,
			Patch:  agentVersion.Patch,
			Pre:    agentVersion.Pre,
			Commit: agentVersion.Commit,
		},
	}
}

// Factory creates a new check factory
func Factory(wlm workloadmeta.Component, cfg configcomp.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check { return newCheck(cfg, wlm, tagger) })
}

func newCheck(cfg configcomp.Component, wlm workloadmeta.Component, tagger tagger.Component) check.Check {
	return newOrchestratorCheck(
		core.NewCheckBase(CheckName),
		&OrchestratorInstance{},
		cfg,
		wlm,
		tagger,
	)
}

// Interval returns the scheduling time for the check.
func (o *OrchestratorCheck) Interval() time.Duration {
	return collectionInterval
}

// Configure configures the orchestrator check
func (o *OrchestratorCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	o.BuildID(integrationConfigDigest, config, initConfig)

	err := o.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	// Retrieves tags on the check config (applicable when scheduled on CLC)
	checkConfigExtraTags, err := getTags(initConfig, config)
	if err != nil {
		return fmt.Errorf("could not parse tags from check config: %w", err)
	}

	// Retrieves tags from the tagger (applicable when scheduled on DCA)
	taggerExtraTags, err := o.tagger.GlobalTags(types.LowCardinality)
	if err != nil {
		return fmt.Errorf("could not get global tags from tagger: %w", err)
	}

	extraTags := make([]string, 0, len(checkConfigExtraTags)+len(taggerExtraTags))
	extraTags = append(extraTags, checkConfigExtraTags...)
	extraTags = append(extraTags, taggerExtraTags...)

	o.orchestratorConfig = orchcfg.NewDefaultOrchestratorConfig(extraTags)

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
		_ = log.Errorc("could not parse check instance config", orchestrator.ExtraLogContext...)
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

	o.orchestratorInformerFactory = getOrchestratorInformerFactory(o.apiClient)

	// Create a new bundle for the check.
	o.collectorBundle = NewCollectorBundle(o)

	return nil
}

// Run runs the orchestrator check
func (o *OrchestratorCheck) Run() error {
	// Initialize collectors
	o.collectorBundle.Initialize()

	// access serializer
	sender, err := o.GetSender()
	if err != nil {
		return err
	}

	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// we also do a safety check for dedicated runners to avoid trying the leader election
	if !o.isCLCRunner || !o.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !pkgconfigsetup.Datadog().GetBool("leader_election") {
			return log.Errorc("Leader Election not enabled. The cluster-agent will not run the check.", orchestrator.ExtraLogContext...)
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
	log.Infoc(fmt.Sprintf("Shutting down informers used by the check '%s'", o.ID()), orchestrator.ExtraLogContext...)
	close(o.stopCh)
	// send all terminated resources
	if o.collectorBundle != nil {
		o.collectorBundle.GetTerminatedResourceBundle().Stop()
	}
}

func getOrchestratorInformerFactory(apiClient *apiserver.APIClient) *collectors.OrchestratorInformerFactory {
	unassignedPodsTweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", "").String()
	}

	// Only collect pods that are in a terminated state: Succeeded, Failed, or Pending.
	terminatedPodsTweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.AndSelectors(
			fields.OneTermNotEqualSelector("status.phase", "Pending"),
			fields.OneTermNotEqualSelector("status.phase", "Running"),
			fields.OneTermNotEqualSelector("status.phase", "Unknown"),
		).String()
	}

	of := &collectors.OrchestratorInformerFactory{
		InformerFactory:              informers.NewSharedInformerFactoryWithOptions(apiClient.InformerCl, defaultResyncInterval),
		CRDInformerFactory:           externalversions.NewSharedInformerFactory(apiClient.CRDInformerClient, defaultResyncInterval),
		DynamicInformerFactory:       dynamicinformer.NewDynamicSharedInformerFactory(apiClient.DynamicInformerCl, defaultResyncInterval),
		VPAInformerFactory:           vpai.NewSharedInformerFactory(apiClient.VPAInformerClient, defaultResyncInterval),
		UnassignedPodInformerFactory: informers.NewSharedInformerFactoryWithOptions(apiClient.InformerCl, defaultResyncInterval, informers.WithTweakListOptions(unassignedPodsTweakListOptions)),
		TerminatedPodInformerFactory: informers.NewSharedInformerFactoryWithOptions(apiClient.InformerCl, defaultResyncInterval, informers.WithTweakListOptions(terminatedPodsTweakListOptions)),
	}

	return of
}

// getTags extracts tags from the configurations
func getTags(initConfig, config integration.Data) ([]string, error) {
	initCommonOptions := integration.CommonInstanceConfig{}
	err := yaml.Unmarshal(initConfig, &initCommonOptions)
	if err != nil {
		return nil, err
	}

	commonOptions := integration.CommonInstanceConfig{}
	err = yaml.Unmarshal(config, &commonOptions)
	if err != nil {
		return nil, err
	}

	var tags []string
	if commonOptions.Tags != nil {
		tags = append(tags, commonOptions.Tags...)
	}
	if initCommonOptions.Tags != nil {
		tags = append(tags, initCommonOptions.Tags...)
	}

	return tags, nil
}
