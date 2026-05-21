// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	externalmetricsmodel "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	// hpaTargetIndexName is the name of the informer index that maps
	// "<namespace>/<scaleTargetRef.name>" → HPA objects.
	hpaTargetIndexName = "hpa-scale-target-ref"
)

var (
	// hpaGVR is the GroupVersionResource for autoscaling/v2 HorizontalPodAutoscalers.
	hpaGVR = schema.GroupVersionResource{
		Group:    "autoscaling",
		Version:  "v2",
		Resource: "horizontalpodautoscalers",
	}

	// cpuUsageQueryKeywords lists the Datadog metric names that represent CPU usage.
	// When a DatadogMetric query contains any of these, the DPA is configured with
	// a PodResource CPU objective (AbsoluteValue) instead of a generic CustomQuery.
	cpuUsageQueryKeywords = []string{"container.cpu.usage", "kubernetes.cpu.usage"}

	defaultCustomQueryWindow = 5 * time.Minute
)

// hpaByTargetRefIndex is the cache.IndexFunc used by the HPA informer to build a reverse
// map from "<namespace>/<scaleTargetRef.name>" to the matching HPA objects.
func hpaByTargetRefIndex(obj interface{}) ([]string, error) {
	unstrHPA, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("hpaByTargetRefIndex: expected *unstructured.Unstructured, got %T", obj)
	}

	targetName, found, err := unstructured.NestedString(unstrHPA.Object, "spec", "scaleTargetRef", "name")
	if err != nil || !found || targetName == "" {
		return nil, nil
	}

	return []string{unstrHPA.GetNamespace() + "/" + targetName}, nil
}

// HPAConfig holds the relevant configuration extracted from an HPA for
// auto-populating a DatadogPodAutoscaler created from a ClusterProfile (UC2).
type HPAConfig struct {
	MinReplicas *int32
	MaxReplicas int32
	// PodCPUUtilization is the pod-level CPU utilization target (0-100), if set.
	PodCPUUtilization *int32
	// ContainerCPUTargets holds per-container CPU utilization targets, if any.
	ContainerCPUTargets []ContainerCPUTarget
	// ExternalMetrics holds configs resolved from HPA external metrics that reference
	// a DatadogMetric CRD (datadogmetric@<namespace>:<name> format).
	ExternalMetrics []ExternalMetricConfig
}

// ContainerCPUTarget is a per-container CPU utilization objective extracted from an HPA.
type ContainerCPUTarget struct {
	ContainerName  string
	CPUUtilization int32
}

// ExternalMetricConfig holds a resolved DatadogMetric configuration for one HPA external metric.
type ExternalMetricConfig struct {
	// Query is DatadogMetric.Spec.Query — the raw Datadog metrics query string.
	Query string
	// TargetValue is the per-pod average target extracted from the HPA metric target.
	// Nil when no explicit target is configured on the HPA.
	TargetValue *resource.Quantity
	// Window is the query evaluation window, derived from DatadogMetric.Spec.MaxAge/TimeWindow.
	Window time.Duration
	// IsCPUUsage is true when the query contains container.cpu.usage or kubernetes.cpu.usage.
	// In that case the DPA objective uses PodResource CPU (AbsoluteValue) rather than CustomQuery.
	IsCPUUsage bool
}

// findHPAForTarget looks up the single HPA in the informer cache whose scaleTargetRef name
// matches targetName. Returns nil, nil when no HPA is found. Returns an error when multiple
// HPAs target the same resource (ambiguous — UC5).
func findHPAForTarget(indexer cache.Indexer, namespace, targetName string) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	objs, err := indexer.ByIndex(hpaTargetIndexName, namespace+"/"+targetName)
	if err != nil {
		return nil, fmt.Errorf("failed to look up HPA by target in cache: %w", err)
	}

	var matches []*autoscalingv2.HorizontalPodAutoscaler
	for _, obj := range objs {
		rtObj, ok := obj.(runtime.Object)
		if !ok {
			continue
		}
		hpa := &autoscalingv2.HorizontalPodAutoscaler{}
		if err := autoscaling.FromUnstructured(rtObj, hpa); err != nil {
			return nil, fmt.Errorf("failed to convert cached HPA object: %w", err)
		}
		matches = append(matches, hpa)
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return matches[0], nil
	default:
		names := make([]string, 0, len(matches))
		for _, h := range matches {
			names = append(names, h.Name)
		}
		return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonAmbiguousHPA,
			"found %d HPAs targeting %q: %v — only one HPA per target is supported for migration",
			len(matches), targetName, names)
	}
}

// hpaHasDatadogMetricRefs returns true when at least one of the HPA's metrics is an External
// metric referencing a DatadogMetric CRD (datadogmetric@<namespace>:<name> format).
// Used to gate the DatadogMetric informer sync check: if the HPA has no such refs we skip the
// check entirely, so migration works even when the DatadogMetric CRD is not installed.
func hpaHasDatadogMetricRefs(hpa *autoscalingv2.HorizontalPodAutoscaler) bool {
	for _, m := range hpa.Spec.Metrics {
		if m.Type == autoscalingv2.ExternalMetricSourceType &&
			m.External != nil &&
			strings.HasPrefix(strings.ToLower(m.External.Metric.Name), "datadogmetric@") {
			return true
		}
	}
	return false
}

// isCPUUsageQuery returns true when the Datadog metrics query references CPU usage metrics
// (container.cpu.usage or kubernetes.cpu.usage).
func isCPUUsageQuery(query string) bool {
	lower := strings.ToLower(query)
	for _, kw := range cpuUsageQueryKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// resolveDatadogMetricFromCache looks up a DatadogMetric by namespace/name in the informer cache
// and returns its spec. The indexer must be backed by the shared DynamicSharedInformerFactory
// for the datadogmetrics GVR.
// Returns ConditionReasonDatadogMetricNotFound when the object is absent from the cache.
func resolveDatadogMetricFromCache(indexer cache.Indexer, namespace, name string) (*datadoghqv1alpha1.DatadogMetricSpec, error) {
	key := namespace + "/" + name
	obj, exists, err := indexer.GetByKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to look up DatadogMetric %s in cache: %w", key, err)
	}
	if !exists {
		return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonDatadogMetricNotFound,
			"DatadogMetric %s/%s not found in informer cache (not yet synced or does not exist)", namespace, name)
	}

	rtObj, ok := obj.(runtime.Object)
	if !ok {
		return nil, fmt.Errorf("DatadogMetric %s: unexpected object type %T in cache", key, obj)
	}
	var metric datadoghqv1alpha1.DatadogMetric
	if err := autoscaling.FromUnstructured(rtObj, &metric); err != nil {
		return nil, fmt.Errorf("failed to convert DatadogMetric %s from cache: %w", key, err)
	}
	return &metric.Spec, nil
}

// validateHPAMetrics checks that every HPA metric is supported for DPA migration:
//   - Resource CPU with Utilization or AverageValue target (UC1/UC2/UC9)
//   - ContainerResource CPU with Utilization or AverageValue target (UC1/UC2/UC9)
//   - External metric referencing a DatadogMetric CRD via "datadogmetric@<ns>:<name>" (UC8)
//
// Any other metric type or configuration is rejected (UC6). AverageValue targets are
// converted to a Utilization percentage at import time from the workload pod template;
// see extractHPAConfig.
func validateHPAMetrics(hpa *autoscalingv2.HorizontalPodAutoscaler) error {
	for _, m := range hpa.Spec.Metrics {
		switch m.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if m.Resource == nil || m.Resource.Name != corev1.ResourceCPU {
				return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
					"HPA %q uses a non-CPU resource metric — only CPU is supported for migration",
					hpa.Name)
			}
			if m.Resource.Target.Type != autoscalingv2.UtilizationMetricType &&
				m.Resource.Target.Type != autoscalingv2.AverageValueMetricType {
				return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
					"HPA %q uses CPU metric with target type %q — only Utilization and AverageValue are supported for migration",
					hpa.Name, m.Resource.Target.Type)
			}
		case autoscalingv2.ContainerResourceMetricSourceType:
			if m.ContainerResource == nil || m.ContainerResource.Name != corev1.ResourceCPU {
				return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
					"HPA %q uses a non-CPU container resource metric — only CPU is supported for migration",
					hpa.Name)
			}
			if m.ContainerResource.Target.Type != autoscalingv2.UtilizationMetricType &&
				m.ContainerResource.Target.Type != autoscalingv2.AverageValueMetricType {
				return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
					"HPA %q uses CPU container metric with target type %q — only Utilization and AverageValue are supported for migration",
					hpa.Name, m.ContainerResource.Target.Type)
			}
		case autoscalingv2.ExternalMetricSourceType:
			if m.External == nil {
				return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
					"HPA %q has an external metric source with no metric definition", hpa.Name)
			}
			if !strings.HasPrefix(strings.ToLower(m.External.Metric.Name), "datadogmetric@") {
				return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
					"HPA %q uses external metric %q — only DatadogMetric references (datadogmetric@<namespace>:<name>) are supported for migration",
					hpa.Name, m.External.Metric.Name)
			}
		default:
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
				"HPA %q uses metric type %q — only CPU resource utilization and DatadogMetric external metrics are supported for migration",
				hpa.Name, m.Type)
		}
	}
	return nil
}

// disableHPA stores the original HPA spec as a JSON annotation and neutralises the HPA
// by setting both scaleUp and scaleDown selectPolicy to Disabled.
// The operation is idempotent: if HPAManagedByDPAAnnotation is already set the call is a no-op.
func disableHPA(ctx context.Context, client k8sclient.Interface, hpa *autoscalingv2.HorizontalPodAutoscaler, dpaNamespace, dpaName string) error {
	if hpa.Annotations[model.HPAManagedByDPAAnnotation] != "" {
		return nil
	}

	originalSpec, err := json.Marshal(hpa.Spec)
	if err != nil {
		return fmt.Errorf("failed to serialise original HPA spec for %s/%s: %w", hpa.Namespace, hpa.Name, err)
	}

	disabled := autoscalingv2.DisabledPolicySelect
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				model.HPAOriginalSpecAnnotation: string(originalSpec),
				model.HPAManagedByDPAAnnotation: dpaNamespace + "/" + dpaName,
			},
		},
		"spec": map[string]interface{}{
			"behavior": map[string]interface{}{
				"scaleUp": map[string]interface{}{
					"selectPolicy": string(disabled),
				},
				"scaleDown": map[string]interface{}{
					"selectPolicy": string(disabled),
				},
			},
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to build HPA disable patch for %s/%s: %w", hpa.Namespace, hpa.Name, err)
	}

	_, err = client.AutoscalingV2().HorizontalPodAutoscalers(hpa.Namespace).Patch(
		ctx, hpa.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to disable HPA %s/%s: %w", hpa.Namespace, hpa.Name, err)
	}

	log.Infof("HPA migration: disabled HPA %s/%s (managed by DPA %s/%s)", hpa.Namespace, hpa.Name, dpaNamespace, dpaName)
	return nil
}

// restoreHPA reads the original spec from the HPAOriginalSpecAnnotation on cachedHPA and
// restores the live HPA to that spec, removing all migration-related annotations.
// It performs a live Get before Update to use the most recent ResourceVersion and avoid
// conflicts. If the annotation is absent or the HPA no longer exists the call is a no-op.
func restoreHPA(ctx context.Context, client k8sclient.Interface, cachedHPA *autoscalingv2.HorizontalPodAutoscaler) error {
	raw := cachedHPA.Annotations[model.HPAOriginalSpecAnnotation]
	if raw == "" {
		log.Infof("HPA migration: no original spec annotation on HPA %s/%s, nothing to restore", cachedHPA.Namespace, cachedHPA.Name)
		return nil
	}

	// Live Get to obtain the current ResourceVersion and avoid update conflicts.
	current, err := client.AutoscalingV2().HorizontalPodAutoscalers(cachedHPA.Namespace).Get(ctx, cachedHPA.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		log.Infof("HPA migration: HPA %s/%s no longer exists, skipping restore", cachedHPA.Namespace, cachedHPA.Name)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get HPA %s/%s before restore: %w", cachedHPA.Namespace, cachedHPA.Name, err)
	}

	var originalSpec autoscalingv2.HorizontalPodAutoscalerSpec
	if err := json.Unmarshal([]byte(raw), &originalSpec); err != nil {
		return fmt.Errorf("failed to decode original HPA spec from annotation on %s/%s: %w", cachedHPA.Namespace, cachedHPA.Name, err)
	}

	updated := current.DeepCopy()
	updated.Spec = originalSpec
	delete(updated.Annotations, model.HPAOriginalSpecAnnotation)
	delete(updated.Annotations, model.HPAManagedByDPAAnnotation)

	_, err = client.AutoscalingV2().HorizontalPodAutoscalers(cachedHPA.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to restore HPA %s/%s: %w", cachedHPA.Namespace, cachedHPA.Name, err)
	}

	log.Infof("HPA migration: restored HPA %s/%s to original spec", cachedHPA.Namespace, cachedHPA.Name)
	return nil
}

// extractHPAConfig reads the HPA configuration and returns an HPAConfig suitable for
// populating a DPA spec (UC2 auto-population). External DatadogMetric references are
// resolved from the informer cache via datadogMetricIndexer (UC8). For HPA metrics
// configured with target.type = AverageValue (UC9), the absolute CPU quantity is
// converted to a Utilization percentage by reading the workload's pod template via
// the dynamic client and the workloadGVRs table (kind-dispatch). If a referenced
// container has no resources.requests.cpu, ConditionReasonMissingCPURequest is returned.
func extractHPAConfig(
	ctx context.Context,
	datadogMetricIndexer cache.Indexer,
	dynamicCl dynamic.Interface,
	workloadGVRs map[string]schema.GroupVersionResource,
	hpa *autoscalingv2.HorizontalPodAutoscaler,
) (HPAConfig, error) {
	cfg := HPAConfig{
		MinReplicas: hpa.Spec.MinReplicas,
		MaxReplicas: hpa.Spec.MaxReplicas,
	}

	// Cache the workload pod template CPU requests so we issue at most one GET per migration
	// regardless of how many AverageValue CPU metrics the HPA declares.
	var (
		cachedRequests    map[string]resource.Quantity
		cachedRequestsErr error
		cachedRequestsHit bool
	)
	getRequests := func() (map[string]resource.Quantity, error) {
		if cachedRequestsHit {
			return cachedRequests, cachedRequestsErr
		}
		cachedRequestsHit = true
		cachedRequests, cachedRequestsErr = getContainerCPURequests(
			ctx, dynamicCl, workloadGVRs, hpa.Namespace,
			hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name)
		return cachedRequests, cachedRequestsErr
	}

	for _, m := range hpa.Spec.Metrics {
		switch m.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if m.Resource == nil || m.Resource.Name != corev1.ResourceCPU {
				continue
			}
			switch m.Resource.Target.Type {
			case autoscalingv2.UtilizationMetricType:
				if m.Resource.Target.AverageUtilization != nil {
					cfg.PodCPUUtilization = pointer.Ptr(*m.Resource.Target.AverageUtilization)
				}
			case autoscalingv2.AverageValueMetricType:
				if m.Resource.Target.AverageValue == nil {
					continue
				}
				reqs, err := getRequests()
				if err != nil {
					return cfg, err
				}
				totalMilli := sumMilliCPU(reqs)
				if totalMilli == 0 {
					return cfg, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonMissingCPURequest,
						"HPA %q uses Resource CPU AverageValue but the workload pod template has no container with resources.requests.cpu",
						hpa.Name)
				}
				cfg.PodCPUUtilization = pointer.Ptr(cpuPercentage(m.Resource.Target.AverageValue.MilliValue(), totalMilli))
			}
		case autoscalingv2.ContainerResourceMetricSourceType:
			if m.ContainerResource == nil || m.ContainerResource.Name != corev1.ResourceCPU {
				continue
			}
			containerName := m.ContainerResource.Container
			switch m.ContainerResource.Target.Type {
			case autoscalingv2.UtilizationMetricType:
				if m.ContainerResource.Target.AverageUtilization != nil {
					cfg.ContainerCPUTargets = append(cfg.ContainerCPUTargets, ContainerCPUTarget{
						ContainerName:  containerName,
						CPUUtilization: *m.ContainerResource.Target.AverageUtilization,
					})
				}
			case autoscalingv2.AverageValueMetricType:
				if m.ContainerResource.Target.AverageValue == nil {
					continue
				}
				reqs, err := getRequests()
				if err != nil {
					return cfg, err
				}
				req, ok := reqs[containerName]
				if !ok || req.MilliValue() == 0 {
					return cfg, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonMissingCPURequest,
						"HPA %q uses ContainerResource CPU AverageValue but container %q has no resources.requests.cpu in the workload pod template",
						hpa.Name, containerName)
				}
				cfg.ContainerCPUTargets = append(cfg.ContainerCPUTargets, ContainerCPUTarget{
					ContainerName:  containerName,
					CPUUtilization: cpuPercentage(m.ContainerResource.Target.AverageValue.MilliValue(), req.MilliValue()),
				})
			}
		case autoscalingv2.ExternalMetricSourceType:
			if m.External == nil {
				continue
			}
			extCfg, err := resolveExternalMetricConfig(datadogMetricIndexer, hpa.Namespace, m)
			if err != nil {
				return cfg, err
			}
			cfg.ExternalMetrics = append(cfg.ExternalMetrics, extCfg)
		}
	}

	return cfg, nil
}

// getContainerCPURequests resolves the workload referenced by (namespace, kind, name) via
// the dynamic client and returns a map of containerName → CPU request read from
// spec.template.spec.containers in the pod template. Init containers are intentionally
// excluded (matches Kubernetes' HPA pod-level Utilization arithmetic). Only kinds present
// in workloadGVRs are supported; an unknown kind returns ConditionReasonUnsupportedHPAMetric.
// A 404 from the API server returns ConditionReasonTargetNotFound.
func getContainerCPURequests(
	ctx context.Context,
	dynamicCl dynamic.Interface,
	workloadGVRs map[string]schema.GroupVersionResource,
	namespace, kind, name string,
) (map[string]resource.Quantity, error) {
	gvr, ok := workloadGVRs[kind]
	if !ok {
		return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
			"workload kind %q is not supported for HPA migration (only Deployment, StatefulSet, and Argo Rollout are supported)", kind)
	}

	obj, err := dynamicCl.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonTargetNotFound,
				"workload %s %s/%s not found while resolving HPA AverageValue CPU target", kind, namespace, name)
		}
		return nil, fmt.Errorf("failed to get workload %s %s/%s: %w", kind, namespace, name, err)
	}

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonMissingCPURequest,
			"workload %s %s/%s has no spec.template.spec.containers", kind, namespace, name)
	}

	requests := make(map[string]resource.Quantity, len(containers))
	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		containerName, _, _ := unstructured.NestedString(container, "name")
		if containerName == "" {
			continue
		}
		cpuStr, found, err := unstructured.NestedString(container, "resources", "requests", "cpu")
		if err != nil || !found || cpuStr == "" {
			continue
		}
		q, err := resource.ParseQuantity(cpuStr)
		if err != nil {
			continue
		}
		requests[containerName] = q
	}
	return requests, nil
}

// sumMilliCPU returns the sum of CPU requests across all containers (in milli-CPU).
func sumMilliCPU(requests map[string]resource.Quantity) int64 {
	var total int64
	for _, q := range requests {
		total += q.MilliValue()
	}
	return total
}

// cpuPercentage returns round-to-nearest((targetMilli / requestMilli) * 100), capped at int32.
// Callers must ensure requestMilli > 0.
func cpuPercentage(targetMilli, requestMilli int64) int32 {
	if requestMilli <= 0 {
		return 0
	}
	pct := (targetMilli*100 + requestMilli/2) / requestMilli
	if pct > int64(^uint32(0)>>1) { // > max int32
		return int32(^uint32(0) >> 1)
	}
	return int32(pct)
}

// resolveExternalMetricConfig resolves one HPA external metric spec into an ExternalMetricConfig
// by looking up the referenced DatadogMetric in the informer cache.
func resolveExternalMetricConfig(datadogMetricIndexer cache.Indexer, hpaNamespace string, m autoscalingv2.MetricSpec) (ExternalMetricConfig, error) {
	// Parse "datadogmetric@<namespace>:<name>". The metric name is already validated by
	// validateHPAMetrics to have the correct prefix and format.
	ref := strings.TrimPrefix(strings.ToLower(m.External.Metric.Name), "datadogmetric@")
	parts := strings.SplitN(ref, ":", 2)
	ddNS, ddName := parts[0], parts[1]
	if ddNS == "" {
		ddNS = hpaNamespace
	}

	spec, err := resolveDatadogMetricFromCache(datadogMetricIndexer, ddNS, ddName)
	if err != nil {
		return ExternalMetricConfig{}, err
	}

	// Resolve %%tag_kube_cluster_name%% and %%env_VAR%% placeholders in the query
	// before storing it in the DPA spec. The Datadog backend cannot resolve these
	// cluster-agent-side template variables.
	resolvedQuery, err := externalmetricsmodel.ResolveMetricQuery(spec.Query)
	if err != nil {
		return ExternalMetricConfig{}, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonDatadogMetricNotFound,
			"DatadogMetric %s/%s: failed to resolve query template variables: %v", ddNS, ddName, err)
	}

	// Extract the per-pod average target value; fall back to the absolute total value.
	var targetValue *resource.Quantity
	if m.External.Target.AverageValue != nil {
		v := m.External.Target.AverageValue.DeepCopy()
		targetValue = &v
	} else if m.External.Target.Value != nil {
		v := m.External.Target.Value.DeepCopy()
		targetValue = &v
	}
	if targetValue == nil {
		return ExternalMetricConfig{}, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedHPAMetric,
			"HPA external metric %q has no target value (neither targetAverageValue nor targetValue is set)",
			m.External.Metric.Name)
	}

	window := defaultCustomQueryWindow
	if spec.MaxAge.Duration > 0 {
		window = spec.MaxAge.Duration
	} else if spec.TimeWindow.Duration > 0 {
		window = spec.TimeWindow.Duration
	}

	return ExternalMetricConfig{
		Query:       resolvedQuery,
		TargetValue: targetValue,
		Window:      window,
		IsCPUUsage:  isCPUUsageQuery(resolvedQuery),
	}, nil
}

// applyHPAConfigToDPASpec merges the HPA-derived configuration into the DPA spec.
// Only fields not already set on the spec are overwritten.
func applyHPAConfigToDPASpec(spec *datadoghq.DatadogPodAutoscalerSpec, cfg HPAConfig) {
	if spec.Constraints == nil {
		spec.Constraints = &datadoghqcommon.DatadogPodAutoscalerConstraints{}
	}
	if spec.Constraints.MinReplicas == nil && cfg.MinReplicas != nil {
		spec.Constraints.MinReplicas = pointer.Ptr(*cfg.MinReplicas)
	}
	if spec.Constraints.MaxReplicas == nil && cfg.MaxReplicas > 0 {
		spec.Constraints.MaxReplicas = pointer.Ptr(cfg.MaxReplicas)
	}

	if len(spec.Objectives) > 0 {
		return
	}

	var objectives []datadoghqcommon.DatadogPodAutoscalerObjective

	// CPU utilization from native Resource/ContainerResource HPA metrics.
	// Pod-level CPU takes precedence over per-container targets when both are present.
	if cfg.PodCPUUtilization != nil {
		objectives = append(objectives, datadoghqcommon.DatadogPodAutoscalerObjective{
			Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
			PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
				Name: corev1.ResourceCPU,
				Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
					Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					Utilization: pointer.Ptr(*cfg.PodCPUUtilization),
				},
			},
		})
	} else {
		for _, ct := range cfg.ContainerCPUTargets {
			ct := ct
			objectives = append(objectives, datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
					Name:      corev1.ResourceCPU,
					Container: ct.ContainerName,
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(ct.CPUUtilization),
					},
				},
			})
		}
	}

	// Objectives from DatadogMetric external metrics (UC8).
	// CPU usage queries → PodResource CPU AbsoluteValue (DPA uses container.cpu.usage / kubernetes.cpu.usage
	// internally for its CPU recommendations).
	// Other queries → CustomQuery with the raw Datadog metrics query.
	for _, em := range cfg.ExternalMetrics {
		em := em
		if em.IsCPUUsage {
			objectives = append(objectives, datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: corev1.ResourceCPU,
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
						AbsoluteValue: em.TargetValue,
					},
				},
			})
		} else {
			objectives = append(objectives, datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType,
				CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{
					Request: datadoghqcommon.DatadogPodAutoscalerTimeseriesFormulaRequest{
						Queries: []datadoghqcommon.DatadogPodAutoscalerTimeseriesQuery{
							{
								Name:   "q",
								Source: datadoghqcommon.DatadogPodAutoscalerMetricsDataSourceMetrics,
								Metrics: &datadoghqcommon.DatadogPodAutoscalerMetricsTimeseriesQuery{
									Query: em.Query,
								},
							},
						},
					},
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
						AbsoluteValue: em.TargetValue,
					},
					Window: metav1.Duration{Duration: em.Window},
				},
			})
		}
	}

	spec.Objectives = objectives
}

// addDPAFinalizer adds HPAMigrationFinalizer to the DPA via a JSON merge patch.
func (c *Controller) addDPAFinalizer(ctx context.Context, ns, name string, existing []string) error {
	if slices.Contains(existing, model.HPAMigrationFinalizer) {
		return nil
	}
	return c.patchDPAFinalizers(ctx, ns, name, append(slices.Clone(existing), model.HPAMigrationFinalizer))
}

// removeDPAFinalizer removes HPAMigrationFinalizer from the DPA via a JSON merge patch.
func (c *Controller) removeDPAFinalizer(ctx context.Context, ns, name string, existing []string) error {
	newFinalizers := slices.DeleteFunc(slices.Clone(existing), func(f string) bool {
		return f == model.HPAMigrationFinalizer
	})
	if len(newFinalizers) == len(existing) {
		return nil
	}
	return c.patchDPAFinalizers(ctx, ns, name, newFinalizers)
}

func (c *Controller) patchDPAFinalizers(ctx context.Context, ns, name string, finalizers []string) error {
	finalizersJSON, err := json.Marshal(finalizers)
	if err != nil {
		return fmt.Errorf("failed to serialize finalizers: %w", err)
	}
	_, err = c.Client.Resource(podAutoscalerGVR).Namespace(ns).Patch(
		ctx, name, types.MergePatchType,
		[]byte(fmt.Sprintf(`{"metadata":{"finalizers":%s}}`, string(finalizersJSON))),
		metav1.PatchOptions{},
	)
	return err
}

// handleHPAMigrationCleanup is called when a DPA with HPAMigrationFinalizer is being deleted.
// It finds the associated HPA in the informer cache, restores it to its original spec, and
// removes the finalizer so Kubernetes can complete the DPA deletion.
// The store must already be locked when called; this function always unlocks it before returning.
func (c *Controller) handleHPAMigrationCleanup(
	ctx context.Context,
	key, ns, name string,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	storeUnlock func(),
) (autoscaling.ProcessResult, error) {
	log.Infof("HPA migration: running finalizer cleanup for DPA %s/%s", ns, name)

	hpa, err := findHPAForTarget(c.hpaIndexer, ns, podAutoscaler.Spec.TargetRef.Name)
	if err != nil {
		storeUnlock()
		return autoscaling.Requeue, fmt.Errorf("HPA migration cleanup: failed to find HPA for DPA %s/%s: %w", ns, name, err)
	}

	if hpa != nil {
		if restoreErr := restoreHPA(ctx, c.kubeClient, hpa); restoreErr != nil {
			storeUnlock()
			return autoscaling.Requeue, fmt.Errorf("HPA migration cleanup: %w", restoreErr)
		}
	} else {
		log.Infof("HPA migration: no HPA found for DPA %s/%s target %q, skipping restore", ns, name, podAutoscaler.Spec.TargetRef.Name)
	}

	if err := c.removeDPAFinalizer(ctx, ns, name, podAutoscaler.Finalizers); err != nil {
		storeUnlock()
		return autoscaling.Requeue, fmt.Errorf("HPA migration cleanup: failed to remove finalizer from DPA %s/%s: %w", ns, name, err)
	}

	c.store.UnlockDelete(key, c.ID)
	return autoscaling.NoRequeue, nil
}

// initiateHPAMigration is called once — when the DPA does not yet have HPAMigrationFinalizer — to:
//  1. Verify the HPA cache has synced; requeue if not.
//  2. Find and validate the existing HPA for the DPA's target.
//  3. Disable the HPA (set scaleUp/scaleDown selectPolicy: Disabled).
//  4. Add HPAMigrationFinalizer to the DPA.
//  5. (UC2) If profile-managed and not yet imported, auto-populate DPA spec from HPA config.
//
// Returns (true, nil) when a Kubernetes object was patched and the caller should stop the
// current reconcile, waiting for an informer re-queue.
func (c *Controller) initiateHPAMigration(
	ctx context.Context,
	ns, name string,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
) (bool, error) {
	// Guard: wait for the HPA cache before looking up the target.
	if !c.hpaInformerSynced() {
		log.Debugf("HPA migration: HPA informer not yet synced, re-queuing DPA %s/%s", ns, name)
		return true, nil
	}

	hpa, err := findHPAForTarget(c.hpaIndexer, ns, podAutoscaler.Spec.TargetRef.Name)
	if err != nil {
		return false, err
	}
	if hpa == nil {
		return false, nil
	}

	if err := validateHPAMetrics(hpa); err != nil {
		return false, err
	}

	// Guard: only wait for the DatadogMetric cache when the HPA actually references a
	// DatadogMetric CRD. This ensures migration proceeds normally for CPU-only HPAs even
	// when the DatadogMetric CRD is not installed in the cluster.
	if hpaHasDatadogMetricRefs(hpa) && !c.datadogMetricInformerSynced() {
		log.Debugf("HPA migration: DatadogMetric informer not yet synced, re-queuing DPA %s/%s", ns, name)
		return true, nil
	}

	// UC2: one-shot import of HPA config into profile-managed DPAs.
	if podAutoscaler.Annotations[model.HPAConfigImportedAnnotation] == "" &&
		podAutoscaler.Labels[model.ProfileLabelKey] != "" {
		cfg, err := extractHPAConfig(ctx, c.datadogMetricIndexer, c.Client, c.workloadGVRs, hpa)
		if err != nil {
			return false, err
		}
		specCopy := podAutoscaler.Spec.DeepCopy()
		applyHPAConfigToDPASpec(specCopy, cfg)

		updatedDPA := podAutoscaler.DeepCopy()
		updatedDPA.Spec = *specCopy
		if updatedDPA.Annotations == nil {
			updatedDPA.Annotations = make(map[string]string)
		}
		updatedDPA.Annotations[model.HPAConfigImportedAnnotation] = "true"

		obj, err := autoscaling.ToUnstructured(updatedDPA)
		if err != nil {
			return false, fmt.Errorf("HPA migration: failed to convert DPA to unstructured: %w", err)
		}
		if _, err := c.Client.Resource(podAutoscalerGVR).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
			return false, fmt.Errorf("HPA migration: failed to update DPA spec with HPA config: %w", err)
		}
		log.Infof("HPA migration: imported HPA config into DPA %s/%s", ns, name)
		return true, nil
	}

	if err := disableHPA(ctx, c.kubeClient, hpa, ns, name); err != nil {
		return false, err
	}

	if err := c.addDPAFinalizer(ctx, ns, name, podAutoscaler.Finalizers); err != nil {
		return false, fmt.Errorf("HPA migration: failed to add finalizer to DPA %s/%s: %w", ns, name, err)
	}

	log.Infof("HPA migration: HPA %s/%s neutralised, finalizer added to DPA %s/%s", hpa.Namespace, hpa.Name, ns, name)
	return true, nil
}
