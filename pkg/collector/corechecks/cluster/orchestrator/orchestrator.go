// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config"
	corecfg "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	coreutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	orchutil "github.com/DataDog/datadog-agent/pkg/util/orchestrator"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	appslisters "k8s.io/client-go/listers/apps/v1"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	batchlistersBeta1 "k8s.io/client-go/listers/batch/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	orchestratorCheckName   = "orchestrator"
	maximumWaitForAPIServer = 10 * time.Second
	collectionInterval      = 10 * time.Second
)

var (
	// this needs to match what is found in https://github.com/kubernetes/kube-state-metrics/blob/09539977815728349522b58154d800e4b517ec9c/internal/store/builder.go#L176-L206
	// in order to share/split easily the collector configuration with the KSM core check
	defaultResources = []string{
		"pods", // unassigned pods
		"deployments",
		"replicasets",
		"services",
		"nodes",
		"jobs",
		"cronjobs",
		"daemonsets",
		"statefulsets",
	}
)

func init() {
	core.RegisterCheck(orchestratorCheckName, OrchestratorFactory)
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
}

func (c *OrchestratorInstance) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// OrchestratorCheck wraps the config and the informers needed to run the check
type OrchestratorCheck struct {
	core.CheckBase
	orchestratorConfig               *orchcfg.OrchestratorConfig
	instance                         *OrchestratorInstance
	stopCh                           chan struct{}
	clusterID                        string
	groupID                          int32
	isCLCRunner                      bool
	apiClient                        *apiserver.APIClient
	unassignedPodLister              corelisters.PodLister
	unassignedPodListerSync          cache.InformerSynced
	deployLister                     appslisters.DeploymentLister
	deployListerSync                 cache.InformerSynced
	rsLister                         appslisters.ReplicaSetLister
	rsListerSync                     cache.InformerSynced
	serviceLister                    corelisters.ServiceLister
	serviceListerSync                cache.InformerSynced
	nodesLister                      corelisters.NodeLister
	nodesListerSync                  cache.InformerSynced
	jobsLister                       batchlisters.JobLister
	jobsListerSync                   cache.InformerSynced
	cronJobsLister                   batchlistersBeta1.CronJobLister
	cronJobsListerSync               cache.InformerSynced
	daemonSetsLister                 appslisters.DaemonSetLister
	daemonSetsListerSync             cache.InformerSynced
	statefulSetsLister               appslisters.StatefulSetLister
	statefulSetsListerSync           cache.InformerSynced
	persistentVolumesLister          corelisters.PersistentVolumeLister
	persistentVolumesListerSync      cache.InformerSynced
	persistentVolumeClaimsLister     corelisters.PersistentVolumeClaimLister
	persistentVolumeClaimsListerSync cache.InformerSynced
	rolesLister                      rbaclisters.RoleLister
	rolesListerSync                  cache.InformerSynced
	roleBindingsLister               rbaclisters.RoleBindingLister
	roleBindingsListerSync           cache.InformerSynced
	clusterRolesLister               rbaclisters.ClusterRoleLister
	clusterRolesListerSync           cache.InformerSynced
	clusterRoleBindingsLister        rbaclisters.ClusterRoleBindingLister
	clusterRoleBindingsListerSync    cache.InformerSynced
	serviceAccountsLister            corelisters.ServiceAccountLister
	serviceAccountsListerSync        cache.InformerSynced
}

func newOrchestratorCheck(base core.CheckBase, instance *OrchestratorInstance) *OrchestratorCheck {
	return &OrchestratorCheck{
		CheckBase:          base,
		orchestratorConfig: orchcfg.NewDefaultOrchestratorConfig(),
		instance:           instance,
		stopCh:             make(chan struct{}),
		groupID:            rand.Int31(),
		isCLCRunner:        config.IsCLCRunner(),
	}
}

// OrchestratorFactory returns the orchestrator check
func OrchestratorFactory() check.Check {
	return newOrchestratorCheck(
		core.NewCheckBase(orchestratorCheckName),
		&OrchestratorInstance{},
	)
}

// Interval returns the scheduling time for the check.
func (o *OrchestratorCheck) Interval() time.Duration {
	return collectionInterval
}

// Configure configures the orchestrator check
func (o *OrchestratorCheck) Configure(config, initConfig integration.Data, source string) error {
	o.BuildID(config, initConfig)

	err := o.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	// loading agent level config
	o.orchestratorConfig.OrchestrationCollectionEnabled = corecfg.Datadog.GetBool("orchestrator_explorer.enabled")
	if !o.orchestratorConfig.OrchestrationCollectionEnabled {
		return errors.New("orchestrator check is configured but the feature is disabled")
	}
	o.orchestratorConfig.IsScrubbingEnabled = corecfg.Datadog.GetBool("orchestrator_explorer.container_scrubbing.enabled")
	o.orchestratorConfig.ExtraTags = corecfg.Datadog.GetStringSlice("orchestrator_explorer.extra_tags")

	// check if cluster name is set
	hostname, _ := coreutil.GetHostname(context.TODO())
	o.orchestratorConfig.KubeClusterName = clustername.GetClusterName(context.TODO(), hostname)
	if o.orchestratorConfig.KubeClusterName == "" {
		return errors.New("orchestrator check is configured but the cluster name is empty")
	}

	// load instance level config
	err = o.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the API server")
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
	apiCl, err := apiserver.WaitForAPIClient(apiCtx)
	o.apiClient = apiCl
	if err != nil {
		return err
	}

	// Prepare the collectors for the resources specified in the configuration file.
	collectors := o.instance.Collectors

	// Enable the orchestrator default collectors if the config collectors list is empty.
	if len(collectors) == 0 {
		collectors = defaultResources
	}

	informersToSync := map[apiserver.InformerName]cache.SharedInformer{}

	for _, v := range collectors {
		switch v {
		case "pods":
			podInformer := apiCl.UnassignedPodInformerFactory.Core().V1().Pods()
			o.unassignedPodLister = podInformer.Lister()
			o.unassignedPodListerSync = podInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sPod.String())] = podInformer.Informer()
		case "deployments":
			deployInformer := apiCl.InformerFactory.Apps().V1().Deployments()
			o.deployLister = deployInformer.Lister()
			o.deployListerSync = deployInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sDeployment.String())] = deployInformer.Informer()
		case "replicasets":
			rsInformer := apiCl.InformerFactory.Apps().V1().ReplicaSets()
			o.rsLister = rsInformer.Lister()
			o.rsListerSync = rsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sReplicaSet.String())] = rsInformer.Informer()
		case "services":
			serviceInformer := apiCl.InformerFactory.Core().V1().Services()
			o.serviceLister = serviceInformer.Lister()
			o.serviceListerSync = serviceInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sService.String())] = serviceInformer.Informer()
		case "nodes":
			nodesInformer := apiCl.InformerFactory.Core().V1().Nodes()
			o.nodesLister = nodesInformer.Lister()
			o.nodesListerSync = nodesInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sNode.String())] = nodesInformer.Informer()
		case "jobs":
			jobsInformer := apiCl.InformerFactory.Batch().V1().Jobs()
			o.jobsLister = jobsInformer.Lister()
			o.jobsListerSync = jobsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sJob.String())] = jobsInformer.Informer()
		case "cronjobs":
			cronJobsInformer := apiCl.InformerFactory.Batch().V1beta1().CronJobs()
			o.cronJobsLister = cronJobsInformer.Lister()
			o.cronJobsListerSync = cronJobsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sCronJob.String())] = cronJobsInformer.Informer()
		case "daemonsets":
			daemonSetsInformer := apiCl.InformerFactory.Apps().V1().DaemonSets()
			o.daemonSetsLister = daemonSetsInformer.Lister()
			o.daemonSetsListerSync = daemonSetsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sDaemonSet.String())] = daemonSetsInformer.Informer()
		case "statefulsets":
			statefulSetsInformer := apiCl.InformerFactory.Apps().V1().StatefulSets()
			o.statefulSetsLister = statefulSetsInformer.Lister()
			o.statefulSetsListerSync = statefulSetsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sStatefulSet.String())] = statefulSetsInformer.Informer()
		case "persistentvolumes":
			persistentVolumesInformer := apiCl.InformerFactory.Core().V1().PersistentVolumes()
			o.persistentVolumesLister = persistentVolumesInformer.Lister()
			o.persistentVolumesListerSync = persistentVolumesInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sPersistentVolume.String())] = persistentVolumesInformer.Informer()
		case "persistentvolumeclaims":
			persistentVolumeClaimsInformer := apiCl.InformerFactory.Core().V1().PersistentVolumeClaims()
			o.persistentVolumeClaimsLister = persistentVolumeClaimsInformer.Lister()
			o.persistentVolumeClaimsListerSync = persistentVolumeClaimsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sPersistentVolumeClaim.String())] = persistentVolumeClaimsInformer.Informer()
		case "roles":
			rolesInformer := apiCl.InformerFactory.Rbac().V1().Roles()
			o.rolesLister = rolesInformer.Lister()
			o.rolesListerSync = rolesInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sRole.String())] = rolesInformer.Informer()
		case "rolebindings":
			roleBindingsInformer := apiCl.InformerFactory.Rbac().V1().RoleBindings()
			o.roleBindingsLister = roleBindingsInformer.Lister()
			o.roleBindingsListerSync = roleBindingsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sRole.String())] = roleBindingsInformer.Informer()
		case "clusterroles":
			clusterRolesInformer := apiCl.InformerFactory.Rbac().V1().ClusterRoles()
			o.clusterRolesLister = clusterRolesInformer.Lister()
			o.clusterRolesListerSync = clusterRolesInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sRole.String())] = clusterRolesInformer.Informer()
		case "clusterrolebindings":
			clusterRoleBindingsInformer := apiCl.InformerFactory.Rbac().V1().ClusterRoleBindings()
			o.clusterRoleBindingsLister = clusterRoleBindingsInformer.Lister()
			o.clusterRoleBindingsListerSync = clusterRoleBindingsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sRole.String())] = clusterRoleBindingsInformer.Informer()
		case "serviceaccounts":
			serviceAccountsInformer := apiCl.InformerFactory.Core().V1().ServiceAccounts()
			o.serviceAccountsLister = serviceAccountsInformer.Lister()
			o.serviceAccountsListerSync = serviceAccountsInformer.Informer().HasSynced
			informersToSync[apiserver.InformerName(orchestrator.K8sRole.String())] = serviceAccountsInformer.Informer()
		default:
			_ = o.Warnf("Unsupported collector: %s", v)
		}
	}

	apiCl.UnassignedPodInformerFactory.Start(o.stopCh)
	apiCl.InformerFactory.Start(o.stopCh)

	return apiserver.SyncInformers(informersToSync)
}

// Run runs the orchestrator check
func (o *OrchestratorCheck) Run() error {
	// access serializer
	sender, err := aggregator.GetSender(o.ID())
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

	// We launch processing on everything but the ones with no
	// started informers are noop
	o.processDeploys(sender)
	o.processReplicaSets(sender)
	o.processPods(sender)
	o.processServices(sender)
	o.processNodes(sender)
	o.processJobs(sender)
	o.processCronJobs(sender)
	o.processDaemonSets(sender)
	o.processStatefulSets(sender)
	o.processPersistentVolumes(sender)
	o.processPersistentVolumeClaims(sender)
	o.processRoles(sender)
	o.processRoleBindings(sender)
	o.processClusterRoles(sender)
	o.processClusterRoleBindings(sender)
	o.processServiceAccounts(sender)

	return nil
}

func (o *OrchestratorCheck) processDeploys(sender aggregator.Sender) {
	if o.deployLister == nil {
		return
	}
	deployList, err := o.deployLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list deployments: %s", err)
		return
	}

	messages, err := processDeploymentList(deployList, atomic.AddInt32(&o.groupID, 1), o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process deployment list: %v", err)
		return
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(deployList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sDeployment,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sDeployment), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sDeployment))
}

func (o *OrchestratorCheck) processReplicaSets(sender aggregator.Sender) {
	if o.rsLister == nil {
		return
	}
	rsList, err := o.rsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list replica sets: %s", err)
		return
	}

	messages, err := processReplicaSetList(rsList, atomic.AddInt32(&o.groupID, 1), o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = log.Errorf("Unable to process replica set list: %v", err)
		return
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(rsList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sReplicaSet,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sReplicaSet), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sReplicaSet))
}

func (o *OrchestratorCheck) processServices(sender aggregator.Sender) {
	if o.serviceLister == nil {
		return
	}
	serviceList, err := o.serviceLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list services: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processServiceList(serviceList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process service list: %s", err)
		return
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(serviceList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sService,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sService), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sService))
}

func (o *OrchestratorCheck) processNodes(sender aggregator.Sender) {
	if o.nodesLister == nil {
		return
	}
	nodesList, err := o.nodesLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list nodes: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	nodesMessages, clusterModel, err := processNodesList(nodesList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process node list: %s", err)
		return
	}
	sendNodesMetadata(sender, nodesList, nodesMessages, o.clusterID)

	clusterMessage, clusterErr := extractClusterMessage(o.orchestratorConfig, o.clusterID, o.apiClient, groupID, clusterModel)
	if clusterErr != nil {
		_ = o.Warnf("Could not collect orchestrator cluster information: %s, will still send nodes information", err)
		return
	}
	if clusterMessage != nil {
		sendClusterMetadata(sender, clusterMessage, o.clusterID)
	}
}

func (o *OrchestratorCheck) processJobs(sender aggregator.Sender) {
	if o.jobsLister == nil {
		return
	}
	jobList, err := o.jobsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list jobs: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processJobList(jobList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process job list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(jobList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sJob,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sJob), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sJob))
}

func (o *OrchestratorCheck) processCronJobs(sender aggregator.Sender) {
	if o.cronJobsLister == nil {
		return
	}
	cronJobList, err := o.cronJobsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list cron jobs: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processCronJobList(cronJobList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process cron job list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(cronJobList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sCronJob,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sCronJob), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sCronJob))
}

func (o *OrchestratorCheck) processDaemonSets(sender aggregator.Sender) {
	if o.daemonSetsLister == nil {
		return
	}
	daemonSetLists, err := o.daemonSetsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list daemonSets: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processDaemonSetList(daemonSetLists, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process daemonSets list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(daemonSetLists) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sDaemonSet,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sDaemonSet), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sDaemonSet))
}

func (o *OrchestratorCheck) processStatefulSets(sender aggregator.Sender) {
	if o.statefulSetsLister == nil {
		return
	}
	statefulSetLists, err := o.statefulSetsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list StatefulSets: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processStatefulSetList(statefulSetLists, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process StatefulSets list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(statefulSetLists) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sStatefulSet,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sStatefulSet), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sStatefulSet))
}

func sendNodesMetadata(sender aggregator.Sender, nodesList []*v1.Node, nodesMessages []model.MessageBody, clusterID string) {
	stats := orchestrator.CheckStats{
		CacheHits: len(nodesList) - len(nodesMessages),
		CacheMiss: len(nodesMessages),
		NodeType:  orchestrator.K8sNode,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sNode), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(nodesMessages, clusterID, int(orchestrator.K8sNode))
}

func sendClusterMetadata(sender aggregator.Sender, clusterMessage model.MessageBody, clusterID string) {
	stats := orchestrator.CheckStats{
		CacheHits: 0,
		CacheMiss: 1,
		NodeType:  orchestrator.K8sCluster,
	}
	sender.OrchestratorMetadata([]serializer.ProcessMessageBody{clusterMessage}, clusterID, int(orchestrator.K8sCluster))
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sCluster), stats, orchestrator.NoExpiration)
}

func (o *OrchestratorCheck) processPods(sender aggregator.Sender) {
	if o.unassignedPodLister == nil {
		return
	}
	podList, err := o.unassignedPodLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list pods: %s", err)
		return
	}

	// we send an empty hostname for unassigned pods
	messages, err := orchutil.ProcessPodList(podList, atomic.AddInt32(&o.groupID, 1), "", o.clusterID, o.orchestratorConfig)
	if err != nil {
		_ = o.Warnf("Unable to process pod list: %v", err)
		return
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(podList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sPod,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sPod), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sPod))
}

func (o *OrchestratorCheck) processPersistentVolumes(sender aggregator.Sender) {
	if o.persistentVolumesLister == nil {
		return
	}
	pvList, err := o.persistentVolumesLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list pv: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processPersistentVolumeList(pvList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process pv list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(pvList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sPersistentVolume,
	}

	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sPersistentVolume), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sPersistentVolume))
}

func (o *OrchestratorCheck) processPersistentVolumeClaims(sender aggregator.Sender) {
	if o.persistentVolumeClaimsLister == nil {
		return
	}
	pvcList, err := o.persistentVolumeClaimsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list pvcs: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processPersistentVolumeClaimList(pvcList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process pvc list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(pvcList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sPersistentVolumeClaim,
	}
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sPersistentVolumeClaim), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sPersistentVolumeClaim))
}

func (o *OrchestratorCheck) processRoles(sender aggregator.Sender) {
	if o.rolesLister == nil {
		return
	}
	roleList, err := o.rolesLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list roles: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processRoleList(roleList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process role list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(roleList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sRole,
	}
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sRole), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sRole))
}

func (o *OrchestratorCheck) processRoleBindings(sender aggregator.Sender) {
	if o.roleBindingsLister == nil {
		return
	}
	roleBindingList, err := o.roleBindingsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list role bindings: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processRoleBindingList(roleBindingList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process role binding list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(roleBindingList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sRoleBinding,
	}
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sRoleBinding), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sRoleBinding))
}

func (o *OrchestratorCheck) processClusterRoles(sender aggregator.Sender) {
	if o.clusterRolesLister == nil {
		return
	}
	clusterRoleList, err := o.clusterRolesLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list cluster roles: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processClusterRoleList(clusterRoleList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process cluster role list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(clusterRoleList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sClusterRole,
	}
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sClusterRole), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sClusterRole))
}

func (o *OrchestratorCheck) processClusterRoleBindings(sender aggregator.Sender) {
	if o.clusterRoleBindingsLister == nil {
		return
	}
	clusterRoleBindingList, err := o.clusterRoleBindingsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list cluster role bindings: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processClusterRoleBindingList(clusterRoleBindingList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process cluster role binding list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(clusterRoleBindingList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sClusterRoleBinding,
	}
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sClusterRoleBinding), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sClusterRoleBinding))
}

func (o *OrchestratorCheck) processServiceAccounts(sender aggregator.Sender) {
	if o.serviceAccountsLister == nil {
		return
	}
	serviceAccountList, err := o.serviceAccountsLister.List(labels.Everything())
	if err != nil {
		_ = o.Warnf("Unable to list service accounts: %s", err)
		return
	}
	groupID := atomic.AddInt32(&o.groupID, 1)

	messages, err := processServiceAccountList(serviceAccountList, groupID, o.orchestratorConfig, o.clusterID)
	if err != nil {
		_ = o.Warnf("Unable to process service account list: %s", err)
	}

	stats := orchestrator.CheckStats{
		CacheHits: len(serviceAccountList) - len(messages),
		CacheMiss: len(messages),
		NodeType:  orchestrator.K8sServiceAccount,
	}
	orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sServiceAccount), stats, orchestrator.NoExpiration)

	sender.OrchestratorMetadata(messages, o.clusterID, int(orchestrator.K8sServiceAccount))
}

// Cancel cancels the orchestrator check
func (o *OrchestratorCheck) Cancel() {
	log.Infof("Shutting down informers used by the check '%s'", o.ID())
	close(o.stopCh)
	o.CommonCancel()
}
