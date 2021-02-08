// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

// +build kubeapiserver,orchestrator

package cluster

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	corecfg "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	coreutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/labels"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	orchestratorCheckName = "orchestrator"
)

func init() {
	core.RegisterCheck(orchestratorCheckName, OrchestratorFactory)
}

// OrchestratorInstance is the config of the orchestrator check instance.
type OrchestratorInstance struct {
	LeaderSkip bool `yaml:"skip_leader_election"`
}

func (c *OrchestratorInstance) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// OrchestratorCheck wraps the config and the informers needed to run the check
type OrchestratorCheck struct {
	core.CheckBase
	orchestratorConfig      *orchcfg.OrchestratorConfig
	instance                *OrchestratorInstance
	cancel                  context.CancelFunc
	stopCh                  chan struct{}
	clusterID               string
	hostname                string
	groupID                 int32
	unassignedPodLister     corelisters.PodLister
	unassignedPodListerSync cache.InformerSynced
	deployLister            appslisters.DeploymentLister
	deployListerSync        cache.InformerSynced
	rsLister                appslisters.ReplicaSetLister
	rsListerSync            cache.InformerSynced
	serviceLister           corelisters.ServiceLister
	serviceListerSync       cache.InformerSynced
	nodesLister             corelisters.NodeLister
	nodesListerSync         cache.InformerSynced
}

func newOrchestratorCheck(base core.CheckBase, instance *OrchestratorInstance) *OrchestratorCheck {
	return &OrchestratorCheck{
		CheckBase:          base,
		orchestratorConfig: orchcfg.NewDefaultOrchestratorConfig(),
		instance:           instance,
		stopCh:             make(chan struct{}),
		groupID:            rand.Int31(),
	}
}

func OrchestratorFactory() check.Check {
	return newOrchestratorCheck(
		core.NewCheckBase(orchestratorCheckName),
		&OrchestratorInstance{},
	)
}

func (o *OrchestratorCheck) Configure(config, initConfig integration.Data, source string) error {
	// loading agent level config
	if !corecfg.Datadog.GetBool("orchestrator_explorer.enabled") {
		return errors.New("orchestrator check is configured but the feature is disabled")
	}
	// check if cluster name is set
	hostname, _ := coreutil.GetHostname()
	if clusterName := clustername.GetClusterName(hostname); clusterName != "" {
		o.orchestratorConfig.KubeClusterName = clusterName
	}

	// load instance level config (TODO:migrate settings)
	err := o.instance.parse(config)
	if err != nil {
		log.Error("could not parse the config for the API server")
		return err
	}

	// start informers
	// reuse the common API Server client to share the cache
	// TODO is it safe? or wait for client (same issue in KSMv2)
	apiCl, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}

	podInformer := apiCl.UnassignedPodInformerFactory.Core().V1().Pods()
	o.clusterID, err = clustername.GetClusterID()
	if err != nil {
		return err
	}

	o.hostname, err = util.GetHostname()
	if err != nil {
		return err
	}

	_, cancel := context.WithCancel(context.Background())
	o.cancel = cancel

	deployInformer := apiCl.InformerFactory.Apps().V1().Deployments()
	rsInformer := apiCl.InformerFactory.Apps().V1().ReplicaSets()
	serviceInformer := apiCl.InformerFactory.Core().V1().Services()
	nodesInformer := apiCl.InformerFactory.Core().V1().Nodes()

	o.unassignedPodLister = podInformer.Lister()
	o.unassignedPodListerSync = podInformer.Informer().HasSynced
	o.deployLister = deployInformer.Lister()
	o.deployListerSync = deployInformer.Informer().HasSynced
	o.rsLister = rsInformer.Lister()
	o.rsListerSync = rsInformer.Informer().HasSynced
	o.serviceLister = serviceInformer.Lister()
	o.serviceListerSync = serviceInformer.Informer().HasSynced
	o.nodesLister = nodesInformer.Lister()
	o.nodesListerSync = nodesInformer.Informer().HasSynced

	apiCl.UnassignedPodInformerFactory.Start(o.stopCh)
	apiCl.InformerFactory.Start(o.stopCh)

	return apiserver.SyncInformers(map[apiserver.InformerName]cache.SharedInformer{
		apiserver.PodsInformer:        apiCl.UnassignedPodInformerFactory.Core().V1().Pods().Informer(),
		apiserver.DeploysInformer:     apiCl.InformerFactory.Apps().V1().Deployments().Informer(),
		apiserver.ReplicaSetsInformer: apiCl.InformerFactory.Apps().V1().ReplicaSets().Informer(),
		apiserver.ServicesInformer:    apiCl.InformerFactory.Core().V1().Services().Informer(),
		apiserver.NodesInformer:       apiCl.InformerFactory.Core().V1().Nodes().Informer(),
	})
}

func (o *OrchestratorCheck) Run() error {
	// access serializer
	sender, err := aggregator.GetSender(o.ID())
	if err != nil {
		return err
	}

	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// The Cluster Agent will passed in the `skip_leader_election` bool.
	if !o.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !config.Datadog.GetBool("leader_election") {
			return log.Error("Leader Election not enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
		}
		errLeader := o.runLeaderElection()
		if errLeader != nil {
			if errLeader == apiserver.ErrNotLeader {
				// Only the leader can instantiate the apiserver client.
				return nil
			}
			return err
		}
	}

	// TODO: check config to only send what we need to
	o.processDeploys(sender)
	o.processReplicaSets(sender)
	o.processPods(sender)
	o.processServices(sender)
	o.processNodes(sender)

	return nil
}

func (o *OrchestratorCheck) processDeploys(sender aggregator.Sender) {
	deployList, err := o.deployLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Unable to list deployments: %s", err)
		return
	}

	msg, err := processDeploymentList(deployList, atomic.AddInt32(&o.groupID, 1), o.orchestratorConfig, o.clusterID)
	if err != nil {
		log.Errorf("Unable to process deployment list: %v", err)
		return
	}

	stats := CheckStats{
		CacheHits: len(deployList) - len(msg),
		CacheMiss: len(msg),
		NodeType:  orchestrator.K8sDeployment,
	}

	orchestrator.KubernetesResourceCache.Set(BuildStatsKey(orchestrator.K8sDeployment), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(msg, o.clusterID, forwarder.PayloadTypeDeployment)
}

func (o *OrchestratorCheck) processReplicaSets(sender aggregator.Sender) {
	rsList, err := o.rsLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Unable to list replica sets: %s", err)
		return
	}

	msg, err := processReplicaSetList(rsList, atomic.AddInt32(&o.groupID, 1), o.orchestratorConfig, o.clusterID)
	if err != nil {
		log.Errorf("Unable to process replica set list: %v", err)
		return
	}

	stats := CheckStats{
		CacheHits: len(rsList) - len(msg),
		CacheMiss: len(msg),
		NodeType:  orchestrator.K8sReplicaSet,
	}

	orchestrator.KubernetesResourceCache.Set(BuildStatsKey(orchestrator.K8sReplicaSet), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(msg, o.clusterID, forwarder.PayloadTypeReplicaSet)
}

func (o *OrchestratorCheck) processServices(sender aggregator.Sender) {
	serviceList, err := o.serviceLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Unable to list services: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processServiceList(serviceList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		log.Errorf("Unable to process service list: %s", err)
		return
	}

	stats := CheckStats{
		CacheHits: len(serviceList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sService,
	}

	orchestrator.KubernetesResourceCache.Set(BuildStatsKey(orchestrator.K8sService), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, forwarder.PayloadTypeService)
}

func (o *OrchestratorCheck) processNodes(sender aggregator.Sender) {
	nodesList, err := o.nodesLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Unable to list nodes: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processNodesList(nodesList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		log.Errorf("Unable to process node list: %s", err)
		return
	}

	stats := CheckStats{
		CacheHits: len(nodesList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sNode,
	}

	orchestrator.KubernetesResourceCache.Set(BuildStatsKey(orchestrator.K8sNode), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, forwarder.PayloadTypeNode)
}

func (o *OrchestratorCheck) processPods(sender aggregator.Sender) {
	podList, err := o.unassignedPodLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Unable to list pods: %s", err)
		return
	}

	// we send an empty hostname for unassigned pods
	msg, err := orchestrator.ProcessPodList(podList, atomic.AddInt32(&o.groupID, 1), "", o.clusterID, o.orchestratorConfig)
	if err != nil {
		log.Errorf("Unable to process pod list: %v", err)
		return
	}

	stats := CheckStats{
		CacheHits: len(podList) - len(msg),
		CacheMiss: len(msg),
		NodeType:  orchestrator.K8sPod,
	}

	orchestrator.KubernetesResourceCache.Set(BuildStatsKey(orchestrator.K8sPod), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(msg, o.clusterID, forwarder.PayloadTypePod)
}

func (o *OrchestratorCheck) Cancel() {
	//TODO
	close(o.stopCh)
}

func (o *OrchestratorCheck) Stop() {
	//TODO
}

func (o *OrchestratorCheck) runLeaderElection() error {
	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		o.Warn("Failed to instantiate the Leader Elector. Not running the Kubernetes API Server check or collecting Kubernetes Events.") //nolint:errcheck
		return err
	}

	err = leaderEngine.EnsureLeaderElectionRuns()
	if err != nil {
		o.Warn("Leader Election process failed to start") //nolint:errcheck
		return err
	}

	if !leaderEngine.IsLeader() {
		log.Debugf("Leader is %q. %s will not run Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader(), leaderEngine.HolderIdentity)
		return apiserver.ErrNotLeader
	}
	log.Tracef("Current leader: %q, running Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader())
	return nil
}
