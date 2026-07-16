// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
	autoscalingstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/store"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type store = autoscalingstore.Store[model.NodePoolInternal]

var (
	nodePoolGVR = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}

	// Supported node class GVRs, tried in order during discovery
	ec2NodeClassGVR = schema.GroupVersionResource{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"}
	eksNodeClassGVR = schema.GroupVersionResource{Group: "eks.amazonaws.com", Version: "v1", Resource: "nodeclasses"}

	// nodeClassGVRByGroup maps a NodeClassReference Group to its GVR
	nodeClassGVRByGroup = map[string]schema.GroupVersionResource{
		ec2NodeClassGVR.Group: ec2NodeClassGVR,
		eksNodeClassGVR.Group: eksNodeClassGVR,
	}

	controllerID autoscalingstore.SenderID = "dca-c"
)

type Controller struct {
	*autoscaling.Controller

	clusterID     string
	clock         clock.Clock
	eventRecorder record.EventRecorder
	rcClient      RcClient
	store         *store
	storeUpdated  *bool
	localSender   sender.Sender
}

// NewController returns a new cluster autoscaling controller
func NewController(
	clock clock.Clock,
	clusterID string,
	eventRecorder record.EventRecorder,
	rcClient RcClient,
	dynamicClient dynamic.Interface,
	dynamicInformer dynamicinformer.DynamicSharedInformerFactory,
	isLeaderFunc func() bool,
	store *store,
	storeUpdated *bool,
	localSender sender.Sender,
) (*Controller, error) {
	c := &Controller{
		clusterID:     clusterID,
		clock:         clock,
		eventRecorder: eventRecorder,
		rcClient:      rcClient,
		localSender:   localSender,
	}

	autoscalingWorkqueue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedItemBasedRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{
			Name:            subsystem,
			MetricsProvider: autoscalingQueueMetricsProvider,
		},
	)

	baseController, err := autoscaling.NewController(controllerID, c, dynamicClient, dynamicInformer, nodePoolGVR, isLeaderFunc, store, autoscalingWorkqueue)
	if err != nil {
		return nil, err
	}

	c.Controller = baseController

	// TODO add later, if needed, when adding more telemetry
	// store.RegisterObserver(autoscalingstore.Observer{
	// 	DeleteFunc: unsetTelemetry,
	// })

	c.store = store
	c.storeUpdated = storeUpdated

	return c, nil
}

// PreStart is called before the controller starts
func (c *Controller) PreStart(ctx context.Context) {
	autoscaling.StartLocalTelemetry(ctx, c.localSender, "cluster", []string{"orch_cluster_id:" + c.clusterID})
}

// Process implements the Processor interface (so required to be public)
// this processes what's in the workqueue, comes from the store or cluster
func (c *Controller) Process(ctx context.Context, _, _, name string) autoscaling.ProcessResult {
	if !c.IsLeader() || !*c.storeUpdated {
		// Requeue in case of a delay in leader election or the store being updated
		return autoscaling.Requeue
	}

	// Try to get Datadog-managed NodePool from cluster
	datadogNp := &karpenterv1.NodePool{}
	npUnstr, err := c.Lister.Get(name)
	if err == nil {
		err = autoscaling.FromUnstructured(npUnstr, datadogNp)
	}

	switch {
	case apierrors.IsNotFound(err):
		// Ignore not found error as it will be created later
		datadogNp = nil
	case err != nil:
		log.Errorf("Unable to retrieve NodePool: %v", err)
		return autoscaling.Requeue
	case npUnstr == nil:
		log.Errorf("Could not parse empty NodePool from local cache")
		return autoscaling.Requeue
	}

	return c.syncNodePool(ctx, name, datadogNp)
}

func (c *Controller) syncNodePool(ctx context.Context, name string, datadogNp *karpenterv1.NodePool) autoscaling.ProcessResult {
	item, foundInStore := c.store.Get(name)
	defer item.Release()

	if foundInStore {
		npi := item.Value()

		// Get Target NodePool from Lister if needed
		var targetNp *karpenterv1.NodePool
		if npi.TargetName() != "" {
			targetNp = &karpenterv1.NodePool{}
			targetNpUnstr, err := c.Lister.Get(npi.TargetName())
			if err != nil {
				log.Errorf("Error retrieving Target NodePool: %v", err)
				return autoscaling.Requeue
			}
			err = autoscaling.FromUnstructured(targetNpUnstr, targetNp)
			if err != nil {
				log.Errorf("Error converting Target NodePool: %v", err)
				return autoscaling.Requeue
			}

			// Only create or update if the TargetHash has not changed
			if npi.TargetHash() != targetNp.GetAnnotations()[model.KarpenterNodePoolHashAnnotationKey] {
				log.Infof("NodePool: %s TargetHash (%s) has changed since recommendation was generated; no action will be applied.", npi.Name(), npi.TargetHash())
				return autoscaling.NoRequeue
			}
		}

		if datadogNp == nil {
			// Present in store but not found in cluster; create it
			if err := c.createNodePool(ctx, targetNp, npi); err != nil {
				log.Errorf("Error creating NodePool: %v", err)
				return autoscaling.Requeue
			}
		} else {
			// Present in store and found in cluster; update it
			if err := c.updateNodePool(ctx, targetNp, datadogNp, npi); err != nil {
				log.Errorf("Error updating NodePool: %v", err)
				return autoscaling.Requeue
			}
		}
	} else {
		if datadogNp != nil && isCreatedByDatadog(datadogNp.GetLabels()) {
			// Not present in store, and the cluster NodePool is fully managed, then delete the NodePool
			if err := c.deleteNodePool(ctx, name, datadogNp); err != nil {
				log.Errorf("Error deleting NodePool: %v", err)
				return autoscaling.Requeue
			}
		} else {
			// Not present in store and the cluster NodePool is not fully managed, do nothing
			log.Debugf("NodePool %s not found in store and is not fully managed, nothing to do", name)
		}
	}

	return autoscaling.NoRequeue
}

func (c *Controller) createNodePool(ctx context.Context, targetNp *karpenterv1.NodePool, npi model.NodePoolInternal) error {
	log.Infof("Creating NodePool: %s", npi.Name())

	knp := npi.KarpenterNodePool()
	if knp == nil {
		return fmt.Errorf("NodePool %s has no manifest, cannot create", npi.Name())
	}
	knp = knp.DeepCopy()
	// If the manifest omits NodeClassRef and a target NodePool exists, prefer its NodeClassRef
	if knp.Spec.Template.Spec.NodeClassRef == nil && targetNp != nil {
		knp.Spec.Template.Spec.NodeClassRef = targetNp.Spec.Template.Spec.NodeClassRef.DeepCopy()
	}
	var err error
	knp, err = c.checkValidNodeClass(ctx, knp)
	if err != nil {
		return fmt.Errorf("unable to update NodePool with node class: %s, err: %v", npi.Name(), err)
	}
	// Update the weight if replica NodePool
	if knp.Spec.Weight == nil && targetNp != nil {
		knp.Spec.Weight = model.GetNodePoolWeight(targetNp)
	}
	// Ensure Datadog autoscaling node label is always present
	if knp.Spec.Template.ObjectMeta.Labels == nil {
		knp.Spec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	knp.Spec.Template.ObjectMeta.Labels[kubernetes.AutoscalingLabelKey] = "true"
	// add Datadog labels and annotations
	if knp.Labels == nil {
		knp.Labels = make(map[string]string)
	}
	knp.Labels[model.DatadogCreatedLabelKey] = "true"
	if knp.Annotations == nil {
		knp.Annotations = make(map[string]string)
	}
	if npi.TargetName() != "" {
		knp.Annotations[model.DatadogReplicaAnnotationKey] = npi.TargetName()
	}

	npUnstr, err := convertNodePoolToUnstructured(knp)
	if err != nil {
		return fmt.Errorf("unable to convert NodePool to unstructured: %s, err: %v", npi.Name(), err)
	}
	_, err = c.Client.Resource(nodePoolGVR).Create(ctx, npUnstr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create NodePool: %s, err: %v", npi.Name(), err)
	}
	c.eventRecorder.Eventf(knp, corev1.EventTypeNormal, model.SuccessfulNodepoolCreateEventReason, "Created NodePool %q", npi.Name())
	return nil
}

func (c *Controller) updateNodePool(ctx context.Context, targetNp *karpenterv1.NodePool, datadogNp *karpenterv1.NodePool, npi model.NodePoolInternal) error {
	desired := npi.KarpenterNodePool()
	if desired == nil {
		return fmt.Errorf("NodePool %s has no manifest, cannot update", npi.Name())
	}
	desired = desired.DeepCopy()
	if desired.Labels == nil {
		desired.Labels = make(map[string]string)
	}
	desired.Labels[model.DatadogCreatedLabelKey] = "true"

	// Use the NodeClass in the live NodePool if the manifest omits it
	if desired.Spec.Template.Spec.NodeClassRef == nil && datadogNp.Spec.Template.Spec.NodeClassRef != nil {
		desired.Spec.Template.Spec.NodeClassRef = datadogNp.Spec.Template.Spec.NodeClassRef.DeepCopy()
	}
	var err error
	desired, err = c.checkValidNodeClass(ctx, desired)
	if err != nil {
		return fmt.Errorf("unable to update NodePool with node class: %s, err: %v", npi.Name(), err)
	}

	// Update the weight if replica NodePool
	if desired.Spec.Weight == nil && targetNp != nil {
		desired.Spec.Weight = model.GetNodePoolWeight(targetNp)
	}

	// Ensure Datadog autoscaling node label is always present
	if desired.Spec.Template.ObjectMeta.Labels == nil {
		desired.Spec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	desired.Spec.Template.ObjectMeta.Labels[kubernetes.AutoscalingLabelKey] = "true"

	// Use merge-patch for spec comparison so fields added to NodePool by default do not trigger unnecessary updates
	liveSpecJSON, err := json.Marshal(datadogNp.Spec)
	if err != nil {
		return fmt.Errorf("unable to marshal live NodePool spec: %s, err: %v", npi.Name(), err)
	}
	desiredSpecJSON, err := json.Marshal(desired.Spec)
	if err != nil {
		return fmt.Errorf("unable to marshal desired NodePool spec: %s, err: %v", npi.Name(), err)
	}
	mergedSpecJSON, err := jsonpatch.MergePatch(liveSpecJSON, desiredSpecJSON)
	if err != nil {
		return fmt.Errorf("unable to compute spec merge patch for NodePool: %s, err: %v", npi.Name(), err)
	}

	// Ensure DatadogReplicaAnnotationKey is always present when a target exists
	if desired.Annotations == nil {
		desired.Annotations = make(map[string]string)
	}
	if npi.TargetName() != "" {
		desired.Annotations[model.DatadogReplicaAnnotationKey] = npi.TargetName()
	}

	// Ignore any annotations managed by Karpenter
	annotationsMatch := true
	for k, v := range desired.Annotations {
		if datadogNp.Annotations[k] != v {
			annotationsMatch = false
			break
		}
	}

	if bytes.Equal(liveSpecJSON, mergedSpecJSON) &&
		maps.Equal(datadogNp.Labels, desired.Labels) &&
		annotationsMatch {
		log.Debugf("NodePool: %s has not changed, no action will be applied.", npi.Name())
		return nil
	}

	log.Infof("Updating NodePool: %s", npi.Name())
	desired.ResourceVersion = datadogNp.ResourceVersion
	updatedUnstr, err := convertNodePoolToUnstructured(desired)
	if err != nil {
		c.eventRecorder.Eventf(datadogNp, corev1.EventTypeWarning, model.FailedNodepoolUpdateEventReason, "Failed to convert NodePool: %v", err)
		return fmt.Errorf("error converting NodePool to unstructured: %s, err: %v", npi.Name(), err)
	}
	_, err = c.Client.Resource(nodePoolGVR).Update(ctx, updatedUnstr, metav1.UpdateOptions{})
	if err != nil {
		c.eventRecorder.Eventf(datadogNp, corev1.EventTypeWarning, model.FailedNodepoolUpdateEventReason, "Failed to update NodePool: %v", err)
		return fmt.Errorf("unable to update NodePool: %s, err: %v", npi.Name(), err)
	}
	c.eventRecorder.Eventf(datadogNp, corev1.EventTypeNormal, model.SuccessfulNodepoolUpdateEventReason, "Updated NodePool %q", npi.Name())
	return nil
}

func (c *Controller) deleteNodePool(ctx context.Context, name string, knp *karpenterv1.NodePool) error {
	log.Infof("Deleting NodePool: %s", name)

	err := c.Client.Resource(nodePoolGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		c.eventRecorder.Eventf(knp, corev1.EventTypeWarning, model.FailedNodepoolDeleteEventReason, "Failed to delete NodePool: %v", err)
		return fmt.Errorf("Unable to delete NodePool: %s, err: %v", name, err)
	}

	c.eventRecorder.Eventf(knp, corev1.EventTypeNormal, model.SuccessfulNodepoolDeleteEventReason, "Deleted NodePool: %s", name)
	return nil
}

func (c *Controller) checkValidNodeClass(ctx context.Context, knp *karpenterv1.NodePool) (*karpenterv1.NodePool, error) {
	nc := knp.Spec.Template.Spec.NodeClassRef
	if nc != nil {
		gvr, ok := nodeClassGVRByGroup[nc.Group]
		if !ok {
			return nil, fmt.Errorf("unknown NodeClassRef group %q", nc.Group)
		}
		_, err := c.Client.Resource(gvr).Get(ctx, nc.Name, metav1.GetOptions{})
		if err == nil { // nodeClassRef is valid, keep it
			return knp, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("unable to validate NodeClassRef %q: %w", nc.Name, err)
		}
		log.Debugf("NodeClass %s not found, falling back to an existing NodeClass", nc.Name)
	}

	// Get NodeClass. If there's none, or more than one that can't be unambiguously resolved by
	// os/arch, then we should not create the NodePool
	nodeClassRef, err := c.discoverNodeClass(ctx, knp)
	if err != nil {
		return nil, err
	}
	knp.Spec.Template.Spec.NodeClassRef = nodeClassRef
	return knp, nil
}

// discoverNodeClass attempts to find a single node class from supported providers.
// It tries manual Karpenter (EC2NodeClass) first, then falls back to EKS Auto Mode (NodeClass).
// If more than one NodeClass is found, it attempts to disambiguate using the NodePool's os/arch
// requirements. Returns the NodeClassReference for the discovered node class, or an error if none
// are found or the ambiguity can't be resolved.
func (c *Controller) discoverNodeClass(ctx context.Context, knp *karpenterv1.NodePool) (*karpenterv1.NodeClassReference, error) {
	for _, provider := range []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{gvr: ec2NodeClassGVR, kind: "EC2NodeClass"},
		{gvr: eksNodeClassGVR, kind: "NodeClass"},
	} {
		ncList, err := c.Client.Resource(provider.gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Debugf("NodeClass CRD %s/%s not found, trying next provider", provider.gvr.Group, provider.kind)
				continue
			}
			return nil, fmt.Errorf("unable to list %s/%s NodeClasses: %w", provider.gvr.Group, provider.kind, err)
		}

		if len(ncList.Items) == 0 {
			continue
		}

		name := ncList.Items[0].GetName()
		if len(ncList.Items) > 1 {
			matched, found := c.attemptNodeClassMatch(ncList.Items, knp)
			if !found {
				return nil, fmt.Errorf("too many %s NodeClasses found (%d), NodePool cannot be created", provider.gvr.Group, len(ncList.Items))
			}
			log.Infof("Multiple %s NodeClasses found for NodePool %s, matched %q based on os/arch requirements, passed over: %s",
				provider.gvr.Group, knp.Name, matched, strings.Join(otherNames(ncList.Items, matched), ", "))
			name = matched
		}

		return &karpenterv1.NodeClassReference{
			Kind:  provider.kind,
			Name:  name,
			Group: provider.gvr.Group,
		}, nil
	}

	return nil, errors.New("no NodeClasses found from any supported provider, NodePool cannot be created")
}

func (c *Controller) attemptNodeClassMatch(ncList []unstructured.Unstructured, knp *karpenterv1.NodePool) (string, bool) {
	// Extract the desired OS and architecture from the nodepool. Only requirements that pin
	// down a single desired value (Operator: In, with exactly one distinct value across all
	// matching requirements) are usable here: NotIn/Exists/Gt/etc. don't tell us which value
	// the NodeClass name should contain, and a requirement accepting several values (e.g.
	// arch In [amd64, arm64]) doesn't tell us which one a matched NodeClass must cover, so
	// guessing one would risk silently binding the NodePool to a NodeClass that doesn't
	// support the other accepted values.
	var os, arch []string
	for _, req := range knp.Spec.Template.Spec.Requirements {
		if req.Operator != corev1.NodeSelectorOpIn || len(req.Values) == 0 {
			continue
		}
		switch req.Key {
		case corev1.LabelOSStable:
			os = append(os, req.Values...)
		case corev1.LabelArchStable:
			arch = append(arch, req.Values...)
		}
	}
	os = singleValue(os)
	arch = singleValue(arch)
	if len(os) == 0 && len(arch) == 0 {
		return "", false
	}

	// Require a name match against every known dimension at once: nameMatchesAllGroups treats an
	// unset (nil) dimension as vacuously satisfied, so this naturally degrades to matching on
	// arch alone or os alone when only one of them is known.
	names := tokenizeNames(ncList)
	if name, ok := uniqueNameMatch(names, [][]string{arch, os}); ok {
		return name, true
	}

	// If both dimensions are known but no NodeClass name satisfies both, fall back to matching a
	// single dimension alone -- but only among NodeClasses whose name doesn't explicitly name a
	// conflicting value for the *other* known dimension (e.g. a NodeClass named "windows-amd64"
	// must not be picked via arch alone when os=linux is required). A NodeClass name that simply
	// doesn't mention the other dimension at all (e.g. "ec2nodeclass-amd64") is fine to fall back
	// on, since it doesn't contradict anything.
	if len(arch) > 0 && len(os) > 0 {
		archName, archOK := uniqueNameMatch(excludingContradictions(names, knownOSValues, os[0]), [][]string{arch})
		osName, osOK := uniqueNameMatch(excludingContradictions(names, knownArchValues, arch[0]), [][]string{os})
		switch {
		case archOK && osOK && archName == osName:
			return archName, true
		case archOK && osOK:
			// The two single-dimension fallbacks disagree on which NodeClass to pick -- that's a
			// genuine ambiguity, not something we should guess between.
			return "", false
		case archOK:
			return archName, true
		case osOK:
			return osName, true
		}
	}

	return "", false
}

// knownOSValues and knownArchValues are the os/arch values Kubernetes nodes report via the
// kubernetes.io/os and kubernetes.io/arch labels (i.e. all GOOS/GOARCH values Go itself
// supports), used to tell a NodeClass name that explicitly names a *different* os/arch (a real
// conflict) apart from one that simply doesn't mention that dimension at all (not a conflict).
var (
	knownOSValues   = []string{"linux", "windows"}
	knownArchValues = []string{
		"386", "amd64", "arm", "arm64", "arm64be", "armbe", "loong64", "mips", "mips64",
		"mips64le", "mips64p32", "mips64p32le", "mipsle", "ppc", "ppc64", "ppc64le",
		"riscv", "riscv64", "s390", "s390x", "sparc", "sparc64", "wasm",
	}
)

// excludingContradictions returns the subset of names whose parts don't contain a segment
// matching (case-insensitively) any of knownValues other than want.
func excludingContradictions(names []namedTokens, knownValues []string, want string) []namedTokens {
	conflicting := make([]string, 0, len(knownValues))
	for _, v := range knownValues {
		if !strings.EqualFold(v, want) {
			conflicting = append(conflicting, v)
		}
	}

	filtered := make([]namedTokens, 0, len(names))
	for _, n := range names {
		if !nameMatchesAnyToken(n.parts, conflicting) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// otherNames returns the names in ncList other than exclude.
func otherNames(ncList []unstructured.Unstructured, exclude string) []string {
	others := make([]string, 0, len(ncList))
	for _, nc := range ncList {
		if name := nc.GetName(); name != exclude {
			others = append(others, name)
		}
	}
	return others
}

// singleValue returns values unchanged if it contains exactly one distinct value, or nil otherwise.
func singleValue(values []string) []string {
	distinct := make(map[string]struct{}, len(values))
	for _, v := range values {
		distinct[v] = struct{}{}
	}
	if len(distinct) != 1 {
		return nil
	}
	return values[:1]
}

// nameSeparators are the characters commonly used to delimit tokens in a NodeClass name (e.g. "linux-amd64-nodeclass").
func nameSeparators(r rune) bool {
	return r == '-' || r == '_' || r == '.'
}

// namedTokens pairs a NodeClass name with its name segments (split on nameSeparators), so the
// split only needs to happen once per name even though uniqueNameMatch may be called multiple
// times against the same NodeClass list.
type namedTokens struct {
	name  string
	parts []string
}

// tokenizeNames splits each NodeClass name in ncList into segments on nameSeparators.
func tokenizeNames(ncList []unstructured.Unstructured) []namedTokens {
	names := make([]namedTokens, len(ncList))
	for i, nc := range ncList {
		name := nc.GetName()
		names[i] = namedTokens{name: name, parts: strings.FieldsFunc(name, nameSeparators)}
	}
	return names
}

// uniqueNameMatch returns the name of the single NodeClass whose name contains, for every
// non-empty group in tokenGroups, a segment matching (case-insensitively) at least one token
// in that group (segments are produced by splitting on nameSeparators, so a NodeClass named
// e.g. "team-amd64x-shared" doesn't incorrectly match the token "amd64"). If zero or more than
// one NodeClass match, it returns false to avoid an ambiguous pick.
func uniqueNameMatch(names []namedTokens, tokenGroups [][]string) (string, bool) {
	var match string
	count := 0
	for _, n := range names {
		if nameMatchesAllGroups(n.parts, tokenGroups) {
			match = n.name
			count++
		}
	}
	return match, count == 1
}

// nameMatchesAllGroups reports whether parts has, for every non-empty group in tokenGroups, at
// least one segment matching (case-insensitively) one of that group's tokens.
func nameMatchesAllGroups(parts []string, tokenGroups [][]string) bool {
	for _, tokens := range tokenGroups {
		if len(tokens) == 0 {
			continue
		}
		if !nameMatchesAnyToken(parts, tokens) {
			return false
		}
	}
	return true
}

// nameMatchesAnyToken reports whether any of parts case-insensitively equals any of tokens.
func nameMatchesAnyToken(parts, tokens []string) bool {
	for _, part := range parts {
		for _, token := range tokens {
			if strings.EqualFold(part, token) {
				return true
			}
		}
	}
	return false
}

func isCreatedByDatadog(labels map[string]string) bool {
	if _, ok := labels[model.DatadogCreatedLabelKey]; ok {
		return true
	}
	return false
}

// Helper function to convert a typed Karpenter NodePool object to unstructured. Handles custom Go types gracefully
func convertNodePoolToUnstructured(np interface{}) (*unstructured.Unstructured, error) {
	// Marshal the structured object to JSON bytes.
	jsonBytes, err := json.Marshal(np)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON bytes into a map[string]interface{}.
	var unstructuredMap map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &unstructuredMap); err != nil {
		return nil, err
	}

	// Wrap the map in unstructured.Unstructured.
	unstructuredObj := &unstructured.Unstructured{
		Object: unstructuredMap,
	}

	return unstructuredObj, nil
}
