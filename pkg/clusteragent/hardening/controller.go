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
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	fieldManager     = "datadog-cluster-agent-hardening"
	// replicaSyncWait is Remote Config's default poll interval. The leader waits
	// this long after it first sees a trial, so that every replica has loaded it
	// before the first pod listing it is created.
	replicaSyncWait = time.Minute
)

// Start loads hardening requests from Remote Config and the requests file, and
// runs the leader controller. It returns the Store the webhook reads.
func Start(ctx context.Context, requestsFile, clusterID string, client kubernetes.Interface, isLeader func() bool, rcClient *rcclient.Client) (*Store, error) {
	if rcClient == nil && requestsFile == "" {
		return nil, errors.New("workload hardening needs remote config or admission_controller.hardening.requests_file")
	}
	store := NewStore(clusterID)
	if rcClient != nil {
		store.OnRCUpdate(rcClient.GetConfigs(state.ProductCWSWorkloadHardening), rcClient.UpdateApplyStatus)
		rcClient.Subscribe(state.ProductCWSWorkloadHardening, store.OnRCUpdate)
	}
	if requestsFile != "" {
		go store.WatchFile(ctx, requestsFile, filePollInterval)
	}
	c := NewController(client, store, isLeader)
	c.self = selfDeployment()
	go c.Run(ctx)
	return store, nil
}

// selfDeployment returns the Cluster Agent's own Deployment. The name is empty
// when the pod name is unknown, so that the leader refuses the whole namespace.
func selfDeployment() types.NamespacedName {
	self := types.NamespacedName{Namespace: namespace.GetMyNamespace()}
	pod, err := apicommon.GetSelfPodName()
	if err != nil {
		log.Warnf("hardening: cannot find the Cluster Agent's own Deployment, refusing trials in namespace %s: %v", self.Namespace, err)
		return self
	}
	self.Name = kubeutil.ParseDeploymentForPodName(pod)
	return self
}

// Controller adds and removes request ids on the pod template of the
// Deployments that requests target. It acts only on the leader.
type Controller struct {
	client   kubernetes.Interface
	store    *Store
	isLeader func() bool
	now      func() time.Time
	// self is the Cluster Agent's own Deployment, which is never trialled.
	self     types.NamespacedName
	syncWait time.Duration

	// firstSeen maps a trial id to when this leader first saw it.
	firstSeen map[string]time.Time
	// done maps a request id to the hash of the content that reached a final
	// status, so it is not reconciled again until that content changes.
	done map[string]string
}

// NewController returns a Controller.
func NewController(client kubernetes.Interface, store *Store, isLeader func() bool) *Controller {
	return &Controller{
		client:    client,
		store:     store,
		isLeader:  isLeader,
		now:       time.Now,
		syncWait:  replicaSyncWait,
		firstSeen: map[string]time.Time{},
		done:      map[string]string{},
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
	for _, req := range c.store.List() {
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
}

func (c *Controller) reconcileRequest(ctx context.Context, req *Request, now time.Time) error {
	if c.done[req.ID] == req.hash {
		return nil
	}
	d, reason, err := c.deployment(ctx, req)
	if err != nil {
		return err
	}
	retire := req.Action == ActionRevert || (req.Action == ActionTrial && !req.Active(now))
	switch {
	case d == nil && retire:
		c.done[req.ID] = req.hash // nothing to remove
	case d == nil:
		c.finish(req, "rejected", reason)
	case retire:
		return c.retire(ctx, req, d)
	case req.Action == ActionValidate:
		return c.validate(ctx, req, d)
	default:
		return c.reconcileTrial(ctx, req, d, now)
	}
	return nil
}

// deployment returns the Deployment req targets, or the reason there is none.
func (c *Controller) deployment(ctx context.Context, req *Request) (*appsv1.Deployment, string, error) {
	d, err := c.client.AppsV1().Deployments(req.Target.Namespace).Get(ctx, req.Target.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "deployment not found", nil
	}
	if err != nil {
		return nil, "", err
	}
	// In place of a uid, which findings do not carry: a Deployment recreated
	// under the same name is not the one the finding is about.
	if d.CreationTimestamp.After(req.CreatedAt) {
		return nil, "deployment was created after the request", nil
	}
	return d, "", nil
}

// retire removes a reverted or expired trial from the pod template.
func (c *Controller) retire(ctx context.Context, req *Request, d *appsv1.Deployment) error {
	ids := ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation])
	if !slices.Contains(ids, req.ID) {
		c.done[req.ID] = req.hash
		return nil
	}
	if err := c.patchTemplate(ctx, d, without(ids, req.ID), false); err != nil {
		return err
	}
	status := "reverted"
	if req.Action == ActionTrial {
		status = "expired"
	}
	c.finish(req, status, "")
	return nil
}

// validate runs a trial's pre-flight checks and dry runs, and writes nothing.
func (c *Controller) validate(ctx context.Context, req *Request, d *appsv1.Deployment) error {
	sc, reason := c.preflight(req, d)
	if reason == "" {
		ids := append(ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation]), req.ID)
		var err error
		if reason, err = c.dryRun(ctx, d, ids, req.Target.Container, sc); err != nil {
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

// reconcileTrial adds an active trial to the pod template, once, or watches
// it once added.
func (c *Controller) reconcileTrial(ctx context.Context, req *Request, d *appsv1.Deployment, now time.Time) error {
	ids := ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation])
	if slices.Contains(ids, req.ID) {
		return c.watch(ctx, req, d, ids)
	}
	history, err := c.replicaSets(ctx, d)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(history, func(rs appsv1.ReplicaSet) bool { return lists(rs.Spec.Template.Annotations, req.ID) }) {
		c.finish(req, "removed", "an older ReplicaSet lists it but the pod template does not, so a rollback or a deploy tool removed it; not re-adding")
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
	if reason, err := c.dryRun(ctx, d, newIDs, req.Target.Container, sc); reason != "" || err != nil {
		if reason != "" {
			c.finish(req, "rejected", reason)
		}
		return err
	}
	if err := c.patchTemplate(ctx, d, newIDs, false); err != nil {
		return err
	}
	c.report(req, "applied", "")
	return nil
}

// watch supersedes an added trial: in place when the pod template now sets
// the control itself, so that retiring it costs no restart, and by removing it
// when the Deployment runs another build than the one the finding is about.
func (c *Controller) watch(ctx context.Context, req *Request, d *appsv1.Deployment, ids []string) error {
	if setsControl(req, &d.Spec.Template.Spec) {
		c.finish(req, "superseded", "the pod template sets the control itself")
		return nil
	}
	reason, err := c.otherBuild(ctx, req, d)
	if reason == "" || err != nil {
		return err
	}
	if err := c.patchTemplate(ctx, d, without(ids, req.ID), false); err != nil {
		return err
	}
	c.finish(req, "superseded", reason)
	return nil
}

// otherBuild returns why d no longer runs the build req's image_id names, or
// "": a deploy changed the container's image since the ReplicaSet that first
// listed req, or a pod listing req runs another digest, which also catches a
// tag pushed again for a new build.
func (c *Controller) otherBuild(ctx context.Context, req *Request, d *appsv1.Deployment) (string, error) {
	history, err := c.replicaSets(ctx, d)
	if err != nil {
		return "", err
	}
	image := containerImage(&d.Spec.Template.Spec, req.Target.Container)
	if i := slices.IndexFunc(history, func(rs appsv1.ReplicaSet) bool { return lists(rs.Spec.Template.Annotations, req.ID) }); i >= 0 {
		if first := containerImage(&history[i].Spec.Template.Spec, req.Target.Container); first != image {
			return fmt.Sprintf("the container image changed from %s to %s", first, image), nil
		}
	}
	pods, err := c.client.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector(d)})
	if err != nil {
		return "", err
	}
	owned := map[types.UID]bool{}
	for _, rs := range history {
		owned[rs.UID] = true
	}
	want := Digest(req.Target.ImageID)
	for _, p := range pods.Items {
		if ref := metav1.GetControllerOf(&p); ref == nil || !owned[ref.UID] || !lists(p.Annotations, req.ID) {
			continue
		}
		for _, s := range p.Status.ContainerStatuses {
			if got := Digest(s.ImageID); s.Name == req.Target.Container && got != "" && got != want {
				return fmt.Sprintf("pod %s runs digest %s, not image_id's", p.Name, got), nil
			}
		}
	}
	return "", nil
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
	spec := &d.Spec.Template.Spec
	sc, reason := Render(req, spec)
	if reason != "" {
		return nil, reason
	}
	container := FindContainer(spec, req.Target.Container) // Render found it
	switch {
	case Digest(container.Image) != "" && Digest(container.Image) != Digest(req.Target.ImageID):
		return nil, "the template image pins another digest than image_id"
	// minReadySeconds does not count: without a probe the kubelet reports a running
	// container Ready, so a crash it is slow to notice outlasts minReadySeconds.
	case container.ReadinessProbe == nil && container.StartupProbe == nil:
		return nil, "the container has no readinessProbe or startupProbe, so pods that crash at start could replace every replica"
	case ptr.Deref(d.Spec.RevisionHistoryLimit, 1) == 0:
		return nil, "revisionHistoryLimit is 0, so a removed trial could not be told from a new one"
	}
	if reason := rolloutSafetyReason(d); reason != "" {
		return nil, reason
	}
	return sc, ""
}

// waitReason returns what a trial that passed pre-flight waits for, or "".
func (c *Controller) waitReason(req *Request, d *appsv1.Deployment, now time.Time) string {
	seen, ok := c.firstSeen[req.ID]
	if !ok {
		seen = now
		c.firstSeen[req.ID] = now
	}
	switch {
	case now.Sub(seen) < c.syncWait:
		return "every Cluster Agent replica to load it"
	case d.Spec.Paused:
		return fmt.Sprintf("%s/%s to be resumed", d.Namespace, d.Name)
	case rolloutInProgress(d):
		return fmt.Sprintf("the rollout of %s/%s to complete", d.Namespace, d.Name)
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

// report logs one of the request statuses the RFC defines.
// ponytail: logs only; status events on the secinfo track come with cws-api.
func (c *Controller) report(req *Request, status, reason string) {
	if reason != "" {
		status += ": " + reason
	}
	log.Infof("hardening request %s (%s on %s/%s container %s): %s", req.ID, req.Control, req.Target.Namespace, req.Target.Name, req.Target.Container, status)
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

func containerImage(spec *corev1.PodSpec, name string) string {
	if c := FindContainer(spec, name); c != nil {
		return c.Image
	}
	return ""
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

// rolloutSafetyReason reports why d's rollout strategy cannot be trusted to
// roll back a failing trial, or "" when it can: a plain RollingUpdate always
// keeps at least one replica on the old (unhardened) template while the
// Deployment controller notices the new one is unhealthy, so it undoes the
// change on its own. Recreate tears down every replica before starting new
// ones, and a RollingUpdate whose maxUnavailable covers every replica does
// the same in effect, so neither has that safety net.
func rolloutSafetyReason(d *appsv1.Deployment) string {
	if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
		return "deployment uses the Recreate strategy, so a failing trial would take every pod down"
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
// template, so a new trial would stack on an unfinished change.
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
