// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

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
	// nodeListInterval is how often each replica lists the nodes' runtimes.
	// ponytail: replicas may disagree for this long after a node with another
	// runtime joins; watch the nodes if that matters.
	nodeListInterval = 10 * time.Minute
	fieldManager     = "datadog-cluster-agent-hardening"
	// replicaSyncWait is Remote Config's default poll interval. The leader waits
	// this long after it first sees a request, so that every replica has loaded
	// it before the first pod listing it is created.
	replicaSyncWait = time.Minute
	// heartbeatPeriod is how often the leader reports every request's status
	// again, since the status scope keeps events for one day.
	heartbeatPeriod = time.Hour
	// maxRolloutsInProgress and maxAppliesPerHour bound how much of a cluster
	// one leader changes at once, whatever the backend sends.
	maxRolloutsInProgress = 3
	maxAppliesPerHour     = 10
)

// Start loads hardening requests from Remote Config and the requests file, and
// runs the leader controller. It returns the Store the webhook reads.
func Start(ctx context.Context, requestsFile, clusterID string, client kubernetes.Interface, isLeader func() bool, rcClient *rcclient.Client) (*Store, error) {
	if rcClient == nil && requestsFile == "" {
		return nil, errors.New("workload hardening needs remote config or admission_controller.hardening.requests_file")
	}
	store := NewStore(clusterID)
	// Listed before any request loads, so that every replica agrees on it.
	refreshDefaults(ctx, client, store)
	go watchNodeRuntimes(ctx, client, store, nodeListInterval)
	if rcClient != nil {
		store.OnRCUpdate(rcClient.GetConfigs(state.ProductCWSProfiles), rcClient.UpdateApplyStatus)
		rcClient.Subscribe(state.ProductCWSProfiles, store.OnRCUpdate)
	}
	if requestsFile != "" {
		go store.WatchFile(ctx, requestsFile, filePollInterval)
	}
	c := NewController(client, store, isLeader)
	c.self = selfDeployment()
	go c.Run(ctx)
	return store, nil
}

// watchNodeRuntimes refreshes store's runtime defaults every interval
// until ctx is done.
func watchNodeRuntimes(ctx context.Context, client kubernetes.Interface, store *Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshDefaults(ctx, client, store)
		}
	}
}

// refreshDefaults sets store's runtime defaults from the nodes' container
// runtimes. A failed listing keeps the previous ones.
func refreshDefaults(ctx context.Context, client kubernetes.Interface, store *Store) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{ResourceVersion: "0"})
	if err != nil {
		log.Warnf("hardening: cannot list nodes, keeping the runtime defaults: %v", err)
		return
	}
	runtimes := make([]string, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		runtimes = append(runtimes, n.Status.NodeInfo.ContainerRuntimeVersion)
	}
	d := RuntimeDefaults(runtimes)
	if store.setDefaults(d) {
		log.Infof("hardening: runtime default capabilities are %s (exact: %t)", strings.Join(d.Capabilities, ","), d.Exact)
	}
}

// selfDeployment returns the Cluster Agent's own Deployment. The name is empty
// when the pod name is unknown, so that the leader refuses the whole namespace.
func selfDeployment() types.NamespacedName {
	self := types.NamespacedName{Namespace: namespace.GetMyNamespace()}
	pod, err := apicommon.GetSelfPodName()
	if err != nil {
		log.Warnf("hardening: cannot find the Cluster Agent's own Deployment, refusing requests in namespace %s: %v", self.Namespace, err)
		return self
	}
	self.Name = kubeutil.ParseDeploymentForPodName(pod)
	return self
}

// Controller adds and removes request ids on the pod template of the
// Deployments that requests target, and watches the rollouts it starts. It
// acts only on the leader.
type Controller struct {
	client   kubernetes.Interface
	store    *Store
	isLeader func() bool
	now      func() time.Time
	// self is the Cluster Agent's own Deployment, which is never hardened.
	self     types.NamespacedName
	syncWait time.Duration

	// firstSeen maps a request id to when this leader first saw it.
	firstSeen map[string]time.Time
	// done maps a request id to the hash of the content that reached a final
	// status, so it is not reconciled again until that content changes.
	done map[string]string
	// status maps a request id to the last status reported for it.
	status        map[string]string
	lastHeartbeat time.Time
	// rollingOut holds the requests whose rollout this leader started and has
	// not seen complete, and applied when it applied requests in the last hour.
	// ponytail: in memory, so a new leader starts both counts from zero.
	rollingOut map[string]bool
	applied    []time.Time
}

// NewController returns a Controller.
func NewController(client kubernetes.Interface, store *Store, isLeader func() bool) *Controller {
	return &Controller{
		client:     client,
		store:      store,
		isLeader:   isLeader,
		now:        time.Now,
		syncWait:   replicaSyncWait,
		firstSeen:  map[string]time.Time{},
		done:       map[string]string{},
		status:     map[string]string{},
		rollingOut: map[string]bool{},
	}
}

// Run reconciles on every request change and every resyncPeriod, until ctx is done.
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
	reqs := c.store.List()
	maps.DeleteFunc(c.rollingOut, func(id string, _ bool) bool { _, ok := c.store.Get(id); return !ok })
	for _, req := range reqs {
		if err := c.reconcileRequest(ctx, req, now); err != nil {
			log.Warnf("hardening request %s: will retry: %v", req.ID, err)
		}
		// Checked after, not before: a request already in flight finishes
		// (Run() promises one full reconcile pass even with an already-done
		// ctx), but a shutdown signalled mid-loop still skips the rest
		// rather than logging bogus "will retry" warnings for them.
		if ctx.Err() != nil {
			return
		}
	}
	c.heartbeat(reqs, now)
}

func (c *Controller) reconcileRequest(ctx context.Context, req *Request, now time.Time) error {
	if c.done[req.ID] == req.hash {
		return nil
	}
	validate := req.Action == ActionValidate
	if validate && !(req.Listed() && req.inForce(now)) {
		c.done[req.ID] = req.hash
		return nil
	}
	d, reason, err := c.deployment(ctx, req)
	if err != nil {
		return err
	}
	enforce := req.Active(now)
	switch {
	case d == nil && (enforce || validate):
		c.finish(req, "rejected", reason)
	case d == nil:
		c.done[req.ID] = req.hash // nothing to remove
	case validate:
		return c.validate(ctx, req, d)
	case enforce:
		return c.reconcileEnforce(ctx, req, d, now)
	default:
		return c.retire(ctx, req, d, now)
	}
	return nil
}

// deployment returns the Deployment req targets, or the reason there is none.
func (c *Controller) deployment(ctx context.Context, req *Request) (*appsv1.Deployment, string, error) {
	d, err := c.client.AppsV1().Deployments(req.Workload.Namespace).Get(ctx, req.Workload.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "deployment not found", nil
	}
	if err != nil {
		return nil, "", err
	}
	// A Deployment recreated under the same name is not the one the findings are about.
	if req.Listed() && d.CreationTimestamp.After(req.addedAt) {
		return nil, "deployment was created after this cluster was added to the request", nil
	}
	return d, "", nil
}

// retire removes a request this cluster no longer enforces (reverted,
// expired, or no longer listed) from the pod template, unless the template
// sets the control itself, so that retiring it costs no rollout.
func (c *Controller) retire(ctx context.Context, req *Request, d *appsv1.Deployment, now time.Time) error {
	ids := ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation])
	if !slices.Contains(ids, req.ID) {
		c.done[req.ID] = req.hash
		return nil
	}
	if setsControl(req, &d.Spec.Template.Spec) {
		c.finish(req, "superseded", "the pod template sets the control itself, so the request is left in place")
		return nil
	}
	if err := c.patchTemplate(ctx, d, without(ids, req.ID), false); err != nil {
		return err
	}
	status := "reverted"
	if req.Action == ActionEnforce && req.Listed() && !req.inForce(now) {
		status = "expired"
	}
	c.finish(req, status, "")
	return nil
}

// validate runs a request's pre-flight checks and dry runs, and writes nothing.
func (c *Controller) validate(ctx context.Context, req *Request, d *appsv1.Deployment) error {
	sc, reason := c.preflight(req, d)
	if reason == "" {
		ids := append(ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation]), req.ID)
		var err error
		if reason, err = c.dryRun(ctx, d, ids, req.Workload.Container, sc); err != nil {
			return err
		}
	}
	if reason != "" {
		c.finish(req, "rejected", reason)
		return nil
	}
	c.finish(req, "validated", "")
	return nil
}

// reconcileEnforce adds an active request to the pod template, once, or
// watches it once added.
func (c *Controller) reconcileEnforce(ctx context.Context, req *Request, d *appsv1.Deployment, now time.Time) error {
	history, err := c.replicaSets(ctx, d)
	if err != nil {
		return err
	}
	ids := ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation])
	if slices.Contains(ids, req.ID) {
		return c.watch(ctx, req, d, history, now)
	}
	if firstListing(history, req.ID) != nil {
		c.finish(req, "removed", "an older ReplicaSet lists it but the pod template does not, so a rollback or a deploy tool removed it; not re-adding")
		return nil
	}
	if setsControl(req, &d.Spec.Template.Spec) {
		c.finish(req, "superseded", "the pod template sets the control itself, so no rollout is needed")
		return nil
	}
	sc, reason := c.preflight(req, d)
	if reason != "" {
		c.finish(req, "rejected", reason)
		return nil
	}
	if wait := c.waitReason(req, d, now); wait != "" {
		log.Debugf("hardening request %s: waiting for %s", req.ID, wait)
		return nil
	}
	newIDs := append(ids, req.ID)
	if reason, err := c.dryRun(ctx, d, newIDs, req.Workload.Container, sc); reason != "" || err != nil {
		if reason != "" {
			c.finish(req, "rejected", reason)
		}
		return err
	}
	if err := c.patchTemplate(ctx, d, newIDs, false); err != nil {
		return err
	}
	c.rollingOut[req.ID] = true
	c.applied = append(c.applied, now)
	c.report(req, "applied", "")
	return nil
}

// watch follows an applied request. Inside its confirmation window, counted
// from the ReplicaSet that first listed it, a failure reverts it, unless the
// customer changed the template since. After the window, or during a
// customer's rollout, a failure is only reported as degraded.
func (c *Controller) watch(ctx context.Context, req *Request, d *appsv1.Deployment, history []appsv1.ReplicaSet, now time.Time) error {
	if setsControl(req, &d.Spec.Template.Spec) {
		c.finish(req, "superseded", "the pod template sets the control itself")
		return nil
	}
	first := firstListing(history, req.ID)
	if first == nil {
		return nil // the Deployment controller has not created the ReplicaSet yet
	}
	if !rolloutInProgress(d) {
		delete(c.rollingOut, req.ID)
	}
	// Pods of older ReplicaSets, such as the ones a rollback is scaling down,
	// say nothing about the current template.
	var pods []corev1.Pod
	if rs := currentReplicaSet(history, d); rs != nil {
		var err error
		if pods, err = c.pods(ctx, d, rs); err != nil {
			return err
		}
	}
	problem := failure(d, pods, req)
	ours := sameTemplate(&first.Spec.Template, &d.Spec.Template, true)
	switch {
	case problem != "" && ours && inWindow(req, first, now):
		return c.rollBack(ctx, d, history, problem, now)
	case problem != "":
		c.report(req, "degraded", problem)
	case !ours && rolloutSafetyReason(d, req.Workload.Container) != "":
		c.report(req, "degraded", "the customer's template change makes the rollout unsafe to watch: "+rolloutSafetyReason(d, req.Workload.Container))
	case unhardened(pods, req) != "":
		c.report(req, "partial", unhardened(pods, req))
	case !inWindow(req, first, now):
		c.report(req, "confirmed", "")
	}
	return nil
}

func inWindow(req *Request, first *appsv1.ReplicaSet, now time.Time) bool {
	return now.Before(first.CreationTimestamp.Add(req.confirmFor))
}

// rollBack removes the requests a failure on d is pinned on, and reports them rolled_back.
func (c *Controller) rollBack(ctx context.Context, d *appsv1.Deployment, history []appsv1.ReplicaSet, problem string, now time.Time) error {
	culprits := c.culprits(d, history, now)
	ids := slices.DeleteFunc(ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation]), func(id string) bool {
		return slices.ContainsFunc(culprits, func(r *Request) bool { return r.ID == id })
	})
	if err := c.patchTemplate(ctx, d, ids, false); err != nil {
		return err
	}
	if len(culprits) > 1 {
		problem += "; the failure cannot be pinned on one request, so every request on the container still in its confirmation window was removed"
	}
	for _, r := range culprits {
		delete(c.rollingOut, r.ID)
		c.finish(r, "rolled_back", problem)
	}
	return nil
}

// culprits returns the requests on d's template still in their confirmation
// window. A failure during the rollout one of them started is pinned on that
// one; any other failure cannot be pinned, so it is on all of them.
func (c *Controller) culprits(d *appsv1.Deployment, history []appsv1.ReplicaSet, now time.Time) []*Request {
	var open []*Request
	for _, id := range ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation]) {
		r, ok := c.store.Get(id)
		if !ok || !r.Active(now) {
			continue
		}
		first := firstListing(history, id)
		if first != nil && inWindow(r, first, now) && sameTemplate(&first.Spec.Template, &d.Spec.Template, true) {
			open = append(open, r)
		}
	}
	if rolloutInProgress(d) {
		latest := history[len(history)-1].UID
		for _, r := range open {
			if firstListing(history, r.ID).UID == latest {
				return []*Request{r}
			}
		}
	}
	return open
}

// failure returns the first sign that req's hardened pods fail, or "".
func failure(d *appsv1.Deployment, pods []corev1.Pod, req *Request) string {
	if d.Status.ObservedGeneration >= d.Generation {
		for _, cond := range d.Status.Conditions {
			switch {
			case cond.Type == appsv1.DeploymentProgressing && cond.Reason == "ProgressDeadlineExceeded":
				return "the rollout stalled (ProgressDeadlineExceeded)"
			case cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionFalse:
				return "the Deployment lost availability (Available=False)"
			}
		}
	}
	for _, p := range pods {
		if applied(&p)[req.ID] != "applied" {
			continue
		}
		for _, s := range p.Status.ContainerStatuses {
			// An OOM kill says nothing about the control.
			if t := s.LastTerminationState.Terminated; s.Name == req.Workload.Container && s.RestartCount > 0 && t != nil && t.Reason != "OOMKilled" {
				return fmt.Sprintf("container %s of pod %s restarted (%s, exit code %d)", s.Name, p.Name, t.Reason, t.ExitCode)
			}
		}
	}
	return ""
}

// unhardened returns why a pod that lists req started without its settings, or "".
func unhardened(pods []corev1.Pod, req *Request) string {
	for _, p := range pods {
		if !lists(p.Annotations, req.ID) {
			continue
		}
		if result := applied(&p)[req.ID]; result != "applied" {
			return fmt.Sprintf("pod %s started without the settings (%q)", p.Name, result)
		}
	}
	return ""
}

// applied decodes a pod's AppliedAnnotation; a pod the webhook never saw has none.
func applied(p *corev1.Pod) map[string]string {
	var results map[string]string
	_ = json.Unmarshal([]byte(p.Annotations[AppliedAnnotation]), &results)
	return results
}

// sameTemplate reports whether two pod templates are the same, apart from a
// ReplicaSet's pod-template-hash label and, with ignoreHardening, the
// hardening keys: then a Deployment's rollout since a ReplicaSet with the
// same template was Datadog's alone.
func sameTemplate(a, b *corev1.PodTemplateSpec, ignoreHardening bool) bool {
	strip := func(t *corev1.PodTemplateSpec) *corev1.PodTemplateSpec {
		t = t.DeepCopy()
		delete(t.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		if ignoreHardening {
			delete(t.Labels, EnabledLabel)
			delete(t.Annotations, RequestsAnnotation)
		}
		return t
	}
	return equality.Semantic.DeepEqual(strip(a), strip(b))
}

// currentReplicaSet returns the ReplicaSet of history that runs d's pod template, or nil.
func currentReplicaSet(history []appsv1.ReplicaSet, d *appsv1.Deployment) *appsv1.ReplicaSet {
	for i := len(history) - 1; i >= 0; i-- {
		if sameTemplate(&history[i].Spec.Template, &d.Spec.Template, false) {
			return &history[i]
		}
	}
	return nil
}

// firstListing returns the oldest ReplicaSet of history whose template lists id:
// the one the request's rollout created.
func firstListing(history []appsv1.ReplicaSet, id string) *appsv1.ReplicaSet {
	if i := slices.IndexFunc(history, func(rs appsv1.ReplicaSet) bool { return lists(rs.Spec.Template.Annotations, id) }); i >= 0 {
		return &history[i]
	}
	return nil
}

// pods returns the pods of rs, a ReplicaSet of d.
func (c *Controller) pods(ctx context.Context, d *appsv1.Deployment, rs *appsv1.ReplicaSet) ([]corev1.Pod, error) {
	list, err := c.client.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector(d)})
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list.Items, func(p corev1.Pod) bool {
		ref := metav1.GetControllerOf(&p)
		return ref == nil || ref.UID != rs.UID
	}), nil
}

// replicaSets returns d's ReplicaSets, oldest first: its revision history.
// ponytail: one List per added trial per resync; use an informer if trials become numerous.
func (c *Controller) replicaSets(ctx context.Context, d *appsv1.Deployment) ([]appsv1.ReplicaSet, error) {
	list, err := c.client.AppsV1().ReplicaSets(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector(d)})
	if err != nil {
		return nil, err
	}
	history := slices.DeleteFunc(list.Items, func(rs appsv1.ReplicaSet) bool {
		ref := metav1.GetControllerOf(&rs)
		return ref == nil || ref.UID != d.UID
	})
	slices.SortFunc(history, func(a, b appsv1.ReplicaSet) int { return a.CreationTimestamp.Compare(b.CreationTimestamp.Time) })
	return history, nil
}

// preflight returns the securityContext req renders on d's pod template, or
// the reason the leader refuses req.
func (c *Controller) preflight(req *Request, d *appsv1.Deployment) (*corev1.SecurityContext, string) {
	if d.Namespace == c.self.Namespace && (c.self.Name == "" || d.Name == c.self.Name) {
		return nil, "the target may be the Cluster Agent's own Deployment"
	}
	sc, reason := Render(req, &d.Spec.Template.Spec, c.store.Defaults())
	if reason != "" {
		return nil, reason
	}
	if ptr.Deref(d.Spec.RevisionHistoryLimit, 1) == 0 {
		return nil, "revisionHistoryLimit is 0, so a removed request could not be told from a new one"
	}
	if reason := rolloutSafetyReason(d, req.Workload.Container); reason != "" {
		return nil, reason
	}
	return sc, ""
}

// waitReason returns what a request that passed pre-flight waits for, or "".
func (c *Controller) waitReason(req *Request, d *appsv1.Deployment, now time.Time) string {
	seen, ok := c.firstSeen[req.ID]
	if !ok {
		seen = now
		c.firstSeen[req.ID] = now
	}
	c.applied = slices.DeleteFunc(c.applied, func(t time.Time) bool { return now.Sub(t) >= time.Hour })
	switch {
	case now.Sub(seen) < c.syncWait:
		return "every Cluster Agent replica to load it"
	case d.Spec.Paused:
		return fmt.Sprintf("%s/%s to be resumed", d.Namespace, d.Name)
	case rolloutInProgress(d):
		return fmt.Sprintf("the rollout of %s/%s to complete", d.Namespace, d.Name)
	case len(c.rollingOut) >= maxRolloutsInProgress:
		return fmt.Sprintf("fewer than %d hardening rollouts in progress", maxRolloutsInProgress)
	case len(c.applied) >= maxAppliesPerHour:
		return fmt.Sprintf("fewer than %d requests applied in the last hour", maxAppliesPerHour)
	}
	return ""
}

// dryRun dry-runs the template change to ids and the rendered securityContext,
// and persists neither. It returns the reason when one is refused.
func (c *Controller) dryRun(ctx context.Context, d *appsv1.Deployment, ids []string, container string, sc *corev1.SecurityContext) (string, error) {
	if err := c.patchTemplate(ctx, d, ids, true); err != nil {
		return refusal("dry run of the template change failed", err)
	}
	if err := c.dryRunSecurityContext(ctx, d, container, sc); err != nil {
		return refusal("dry run of the securityContext failed", err)
	}
	return "", nil
}

// refusal returns err as a rejection reason when the API server or the
// cluster's policy refused the patch: invalid, forbidden, or a bad request.
// Every other error — a conflict, throttling, a 5xx, a timeout, a briefly
// unreachable webhook, ctx canceled — says nothing about whether the request
// is valid, so it is returned for reconcile to retry.
func refusal(what string, err error) (string, error) {
	if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err) {
		return fmt.Sprintf("%s: %v", what, err), nil
	}
	return "", err
}

// report records one of the request statuses the RFC defines, and logs it
// when it changed.
// ponytail: logs only; status events on the secinfo track need an Event Platform pipeline in the Cluster Agent.
func (c *Controller) report(req *Request, status, reason string) {
	if reason != "" {
		status += ": " + reason
	}
	if c.status[req.ID] == status {
		return
	}
	c.status[req.ID] = status
	emit(req, status)
}

func emit(req *Request, status string) {
	w := req.Workload
	log.Infof("hardening request %s (%s on %s/%s container %s): %s", req.ID, req.Control, w.Namespace, w.Name, w.Container, status)
}

// heartbeat reports the status of every request again each heartbeatPeriod.
func (c *Controller) heartbeat(reqs []*Request, now time.Time) {
	if now.Sub(c.lastHeartbeat) < heartbeatPeriod {
		return
	}
	c.lastHeartbeat = now
	for _, req := range reqs {
		if status, ok := c.status[req.ID]; ok {
			emit(req, status)
		}
	}
}

// finish reports a final status, and stops reconciling req until its content changes.
func (c *Controller) finish(req *Request, status, reason string) {
	c.done[req.ID] = req.hash
	c.report(req, status, reason)
}

func lists(annotations map[string]string, id string) bool {
	return slices.Contains(ParseIDs(annotations[RequestsAnnotation]), id)
}

func without(ids []string, id string) []string {
	return slices.DeleteFunc(ids, func(x string) bool { return x == id })
}

// selector returns d's label selector; an invalid one selects everything,
// which the callers' owner checks then narrow.
func selector(d *appsv1.Deployment) string {
	sel, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return ""
	}
	return sel.String()
}

// patchTemplate sets the pod template's request list to ids, and removes the
// label with the last id. The patch carries the resourceVersion ids were read
// from, so a concurrent change fails with a conflict instead of being overwritten.
func (c *Controller) patchTemplate(ctx context.Context, d *appsv1.Deployment, ids []string, dryRun bool) error {
	var label, annotation any // null removes the key in a merge patch
	if len(ids) > 0 {
		label, annotation = "true", strings.Join(ids, ",")
	}
	return c.patch(ctx, d, types.MergePatchType, map[string]any{
		"metadata": map[string]any{"resourceVersion": d.ResourceVersion},
		"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{
			"labels":      map[string]any{EnabledLabel: label},
			"annotations": map[string]any{RequestsAnnotation: annotation},
		}}},
	}, dryRun)
}

// dryRunSecurityContext checks that the API server and the cluster's admission
// policies accept the rendered securityContext on the pod template.
func (c *Controller) dryRunSecurityContext(ctx context.Context, d *appsv1.Deployment, container string, sc *corev1.SecurityContext) error {
	return c.patch(ctx, d, types.StrategicMergePatchType, map[string]any{
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
			"containers": []any{map[string]any{"name": container, "securityContext": sc}},
		}}},
	}, true)
}

func (c *Controller) patch(ctx context.Context, d *appsv1.Deployment, pt types.PatchType, body any, dryRun bool) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	opts := metav1.PatchOptions{FieldManager: fieldManager}
	if dryRun {
		opts.DryRun = []string{metav1.DryRunAll}
	}
	_, err = c.client.AppsV1().Deployments(d.Namespace).Patch(ctx, d.Name, pt, data, opts)
	return err
}

// defaultMaxUnavailable is the Deployment controller's default RollingUpdate
// maxUnavailable, used when the Deployment leaves the field unset.
var defaultMaxUnavailable = intstr.FromString("25%")

// rolloutSafetyReason reports why d's rollout cannot catch a control that
// breaks container, or "" when it can. A RollingUpdate keeps old pods serving
// only while new ones are not available, and a container with no probe is
// available as soon as it starts. Recreate tears down every replica first,
// and a maxUnavailable that covers every replica does the same in effect.
func rolloutSafetyReason(d *appsv1.Deployment, container string) string {
	// minReadySeconds does not count: without a probe the kubelet reports a running
	// container Ready, so a crash it is slow to notice outlasts minReadySeconds.
	if c := FindContainer(&d.Spec.Template.Spec, container); c != nil && c.ReadinessProbe == nil && c.StartupProbe == nil {
		return "the container has no readinessProbe or startupProbe, so pods that crash at start could replace every replica"
	}
	if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
		return "deployment uses the Recreate strategy, so a failing control would take every pod down"
	}
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	if replicas == 0 {
		return "" // nothing to take down
	}
	maxUnavailable := &defaultMaxUnavailable
	if ru := d.Spec.Strategy.RollingUpdate; ru != nil && ru.MaxUnavailable != nil {
		maxUnavailable = ru.MaxUnavailable
	}
	// Round down, as the Deployment controller itself does.
	n, err := intstr.GetScaledValueFromIntOrPercent(maxUnavailable, int(replicas), false)
	if err != nil {
		return fmt.Sprintf("cannot resolve maxUnavailable: %v", err)
	}
	if n >= int(replicas) {
		return "maxUnavailable allows every pod to be down"
	}
	return ""
}

// rolloutInProgress reports whether d has not finished rolling out its current
// template, so a new request would stack on an unfinished change.
func rolloutInProgress(d *appsv1.Deployment) bool {
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	s := d.Status
	return s.ObservedGeneration < d.Generation ||
		s.UpdatedReplicas < replicas ||
		s.Replicas > s.UpdatedReplicas ||
		s.AvailableReplicas < s.UpdatedReplicas
}
