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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	resyncPeriod     = 30 * time.Second
	filePollInterval = 10 * time.Second
	fieldManager     = "datadog-cluster-agent-hardening"
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
	go NewController(client, store, isLeader).Run(ctx)
	return store, nil
}

// Controller adds and removes request ids on the pod template of the
// Deployments that requests target. It acts only on the leader.
type Controller struct {
	client   kubernetes.Interface
	store    *Store
	isLeader func() bool
	now      func() time.Time

	// added holds the ids seen on their Deployment's template: true while
	// listed, false once removed by someone else, so they are never re-added.
	// ponytail: in memory, so a new leader may re-add a removed id once; M3 persists it.
	added map[string]bool
	// rejected maps a rejected request id to the hash of the rejected content.
	rejected map[string]string
}

// NewController returns a Controller.
func NewController(client kubernetes.Interface, store *Store, isLeader func() bool) *Controller {
	return &Controller{
		client:   client,
		store:    store,
		isLeader: isLeader,
		now:      time.Now,
		added:    map[string]bool{},
		rejected: map[string]string{},
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
		if ctx.Err() != nil {
			return
		}
		if err := c.reconcileRequest(ctx, req, now); err != nil {
			log.Warnf("hardening request %s: will retry: %v", req.ID, err)
		}
	}
}

func (c *Controller) reconcileRequest(ctx context.Context, req *Request, now time.Time) error {
	active := req.Active(now)
	if active && c.rejected[req.ID] == req.hash {
		return nil
	}
	d, err := c.client.AppsV1().Deployments(req.Target.Namespace).Get(ctx, req.Target.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if active {
			c.reject(req, "deployment not found")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if string(d.UID) != req.Target.UID {
		if active {
			c.reject(req, "deployment uid does not match")
		}
		return nil
	}

	ids := ParseIDs(d.Spec.Template.Annotations[RequestsAnnotation])
	listed := slices.Contains(ids, req.ID)

	if !active {
		if !listed {
			return nil
		}
		if err := c.patchTemplate(ctx, d, slices.DeleteFunc(ids, func(id string) bool { return id == req.ID }), false); err != nil {
			return err
		}
		delete(c.added, req.ID)
		outcome := "reverted"
		if req.Action == ActionTrial {
			outcome = "expired"
		}
		log.Infof("hardening request %s: %s on %s/%s", req.ID, outcome, d.Namespace, d.Name)
		return nil
	}

	if listed {
		c.added[req.ID] = true
		return nil
	}
	if stillListed, seen := c.added[req.ID]; seen {
		if stillListed {
			log.Infof("hardening request %s: removed from %s/%s by a rollback or a deploy tool, not re-adding", req.ID, d.Namespace, d.Name)
			c.added[req.ID] = false
		}
		return nil
	}

	sc, reason := Render(req, &d.Spec.Template.Spec)
	if reason != "" {
		c.reject(req, reason)
		return nil
	}
	if rolloutInProgress(d) {
		log.Debugf("hardening request %s: waiting for the rollout of %s/%s to complete", req.ID, d.Namespace, d.Name)
		return nil
	}
	newIDs := append(ids, req.ID)
	if err := c.patchTemplate(ctx, d, newIDs, true); err != nil {
		return c.rejectIfRefused(req, "dry run of the template change failed", err)
	}
	if err := c.dryRunSecurityContext(ctx, d, req.Target.Container, sc); err != nil {
		return c.rejectIfRefused(req, "dry run of the securityContext failed", err)
	}
	if err := c.patchTemplate(ctx, d, newIDs, false); err != nil {
		return err
	}
	c.added[req.ID] = true
	log.Infof("hardening request %s: applied %s to %s/%s container %s", req.ID, req.Control, d.Namespace, d.Name, req.Target.Container)
	return nil
}

func (c *Controller) reject(req *Request, reason string) {
	c.rejected[req.ID] = req.hash
	log.Infof("hardening request %s: rejected: %s", req.ID, reason)
}

// rejectIfRefused rejects req only when err means the request or the cluster's
// policy said no (the API server validated and refused the patch): invalid,
// forbidden, or a bad request. Every other error — a conflict, throttling, a
// 5xx, a timeout, a briefly unreachable webhook, ctx canceled — says nothing
// about whether the request is valid, so it is returned for reconcile to retry.
func (c *Controller) rejectIfRefused(req *Request, what string, err error) error {
	if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err) {
		c.reject(req, fmt.Sprintf("%s: %v", what, err))
		return nil
	}
	return err
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
func (c *Controller) dryRunSecurityContext(ctx context.Context, d *appsv1.Deployment, container string, sc any) error {
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
