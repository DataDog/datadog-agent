// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	kubeutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	resyncPeriod     = 30 * time.Second
	filePollInterval = 10 * time.Second
	informerResync   = 10 * time.Minute
	fieldManager     = "datadog-cluster-agent-hardening"
	// replicaSyncWait is Remote Config's default poll interval. The leader
	// waits this long after it first sees a policy list its cluster, so that
	// every replica has loaded it before the first marked pod is created.
	replicaSyncWait = time.Minute
	// heartbeatPeriod is how often the leader reports every status again,
	// since enforcement outlives the status events' retention.
	heartbeatPeriod = time.Hour
)

// Start loads hardening configs from Remote Config and the configs file, and
// runs the leader controller. It returns the Store the webhook reads. client
// writes; informerClient, which has no request timeout, feeds the informers.
func Start(ctx context.Context, configsFile, clusterID string, client, informerClient kubernetes.Interface, isLeader func() bool, rcClient *rcclient.Client) (*Store, error) {
	if rcClient == nil && configsFile == "" {
		return nil, errors.New("workload hardening needs remote config or admission_controller.hardening.configs_file")
	}
	store := NewStore(clusterID)
	if rcClient != nil {
		store.OnRCUpdate(rcClient.GetConfigs(state.ProductCWSProfiles), rcClient.UpdateApplyStatus)
		rcClient.Subscribe(state.ProductCWSProfiles, store.OnRCUpdate)
	}
	if configsFile != "" {
		go store.WatchFile(ctx, configsFile, filePollInterval)
	}
	factory := informers.NewSharedInformerFactory(informerClient, informerResync)
	// Only the pods of marked templates, which carry the label.
	podFactory := informers.NewSharedInformerFactoryWithOptions(informerClient, informerResync,
		informers.WithTweakListOptions(func(o *metav1.ListOptions) { o.LabelSelector = EnabledLabel + "=true" }))
	deployments := factory.Apps().V1().Deployments()
	pods := podFactory.Core().V1().Pods()
	c := NewController(client, deployments.Lister(), pods.Lister(), store, isLeader)
	c.self = selfDeployment()
	synced := []cache.InformerSynced{deployments.Informer().HasSynced, pods.Informer().HasSynced}
	go func() {
		factory.Start(ctx.Done())
		podFactory.Start(ctx.Done())
		if !cache.WaitForCacheSync(ctx.Done(), synced...) {
			return
		}
		c.Run(ctx)
	}()
	return store, nil
}

// selfDeployment returns the Cluster Agent's own Deployment. The name is empty
// when the pod name is unknown, so that the leader skips the whole namespace.
func selfDeployment() types.NamespacedName {
	self := types.NamespacedName{Namespace: namespace.GetMyNamespace()}
	pod, err := apicommon.GetSelfPodName()
	if err != nil {
		log.Warnf("hardening: cannot find the Cluster Agent's own Deployment, skipping namespace %s: %v", self.Namespace, err)
		return self
	}
	self.Name = kubeutil.ParseDeploymentForPodName(pod)
	return self
}

// Controller marks the Deployments each policy covers, a few at a time, and
// watches the rollouts it starts. It acts only on the leader.
type Controller struct {
	client      kubernetes.Interface
	deployments appslisters.DeploymentLister
	// pods lists the pods of marked templates only.
	pods     corelisters.PodLister
	store    *Store
	isLeader func() bool
	now      func() time.Time
	// self is the Cluster Agent's own Deployment, which is never hardened.
	self     types.NamespacedName
	syncWait time.Duration

	// firstSeen maps a policy id to when this leader first saw it active.
	firstSeen map[string]time.Time
	// ponytail: pacing, rejections and statuses are in memory, so a new
	// leader starts from a small batch and re-checks every Deployment.
	pacing   map[string]*pacing
	rejected map[rejectionKey]int64
	status   map[statusKey]string
	// lastHeartbeat is when every status was last reported again.
	lastHeartbeat time.Time
}

type rejectionKey struct {
	policy     string
	deployment types.UID
}

// NewController returns a Controller.
func NewController(client kubernetes.Interface, deployments appslisters.DeploymentLister, pods corelisters.PodLister, store *Store, isLeader func() bool) *Controller {
	return &Controller{
		client:      client,
		deployments: deployments,
		pods:        pods,
		store:       store,
		isLeader:    isLeader,
		now:         time.Now,
		syncWait:    replicaSyncWait,
		firstSeen:   map[string]time.Time{},
		pacing:      map[string]*pacing{},
		rejected:    map[rejectionKey]int64{},
		status:      map[statusKey]string{},
	}
}

// Run reconciles on every config change and every resyncPeriod, until ctx is done.
func (c *Controller) Run(ctx context.Context) {
	ticker := time.NewTicker(resyncPeriod)
	defer ticker.Stop()
	for {
		c.reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-c.store.Changes():
		}
	}
}

func (c *Controller) reconcile(ctx context.Context) {
	if !c.isLeader() {
		return
	}
	now := c.now()
	deployments, err := c.deployments.List(labels.Everything())
	if err != nil {
		log.Warnf("hardening: cannot list Deployments: %v", err)
		return
	}
	slices.SortFunc(deployments, func(a, b *appsv1.Deployment) int { return strings.Compare(key(a), key(b)) })
	// A Deployment changes at most once per pass: the listed copies are stale after a patch.
	touched := map[types.UID]bool{}
	for _, d := range deployments {
		if err := c.optOut(ctx, d, touched, now); err != nil {
			log.Warnf("hardening: %s: will retry: %v", key(d), err)
		}
	}
	for _, p := range c.store.Policies() {
		if err := c.reconcilePolicy(ctx, p, deployments, touched, now); err != nil {
			log.Warnf("hardening policy %s: will retry: %v", p.ID, err)
		}
		if ctx.Err() != nil {
			return
		}
	}
	c.heartbeat(now)
}

func (c *Controller) reconcilePolicy(ctx context.Context, p *Policy, deployments []*appsv1.Deployment, touched map[types.UID]bool, now time.Time) error {
	pc := c.pacingFor(p)
	if !p.Active() {
		delete(c.firstSeen, p.ID)
		return c.unmarkInactive(ctx, p, pc, deployments, touched, now)
	}
	if _, ok := c.firstSeen[p.ID]; !ok {
		c.firstSeen[p.ID] = now
	}
	exc, ready := c.store.Exceptions(p.Control, p.MinExceptionsGeneration)
	var candidates []*appsv1.Deployment
	sum := passSummary{}
	for _, d := range deployments {
		if touched[d.UID] || optedOut(d) {
			continue
		}
		containers := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])[p.ID]
		if len(containers) == 0 {
			candidates = append(candidates, d)
			continue
		}
		if err := c.watch(ctx, p, pc, exc, d, containers, touched, &sum, now); err != nil {
			return err
		}
	}
	c.settle(p, pc, deployments)
	if reason := c.markWait(p, pc, ready, &sum, deployments, candidates, now); reason != "" {
		log.Debugf("hardening policy %s: not marking: %s", p.ID, reason)
		return nil
	}
	return c.markBatch(ctx, p, pc, exc, candidates, touched, now)
}

// optedOut reports whether a person set the opt-out annotation on d.
func optedOut(d *appsv1.Deployment) bool {
	return d.Annotations[OptOutAnnotation] == "true"
}

// optOut removes every mark from a Deployment a person opted out, at once, and
// records the removed policies so they outlive the annotation.
func (c *Controller) optOut(ctx context.Context, d *appsv1.Deployment, touched map[types.UID]bool, now time.Time) error {
	marks := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])
	if !optedOut(d) || !marks.hardening() {
		return nil
	}
	exclusions := parseExclusions(d.Annotations[ExcludedAnnotation])
	var removed []string
	for id, containers := range marks {
		if len(containers) > 0 {
			exclusions = append(exclusions, Exclusion{Policy: id, Containers: containers, Reason: ReasonRemoved, At: now})
			removed = append(removed, id)
		}
	}
	if err := c.patchMarks(ctx, d, Marks{}, exclusions); err != nil {
		return err
	}
	touched[d.UID] = true
	for _, id := range removed {
		c.report(id, d, "removed", "the Deployment was opted out")
	}
	return nil
}

func key(d *appsv1.Deployment) string {
	return d.Namespace + "/" + d.Name
}
