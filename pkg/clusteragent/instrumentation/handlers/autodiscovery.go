// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

const (
	checksReadyConditionType = "ChecksReady"

	// autodiscoveryProvider is the integration.Config Source value used for configs
	// translated from a DatadogInstrumentation CR by the Autodiscovery handler.
	autodiscoveryProvider = "datadoginstrumentation"
)

type CheckStore struct {
	mu      sync.RWMutex
	configs map[string][]integration.Config
	// states maps namespace/name → "uid:generation". Including the UID ensures that a
	// delete+recreate of a CR with the same name is detected even when the new CR
	// starts at generation 1.
	states     map[string]string
	configHash uint64
}

func NewCheckStore() *CheckStore {
	return &CheckStore{
		configs:    make(map[string][]integration.Config),
		states:     make(map[string]string),
		configHash: fnv.New64a().Sum64(),
	}
}

// AutodiscoveryHandler translates DatadogInstrumentation check sections into
// integration.Config entries and stores them in memory for delivery to node agents.
type AutodiscoveryHandler struct {
	checkStore *CheckStore
}

// NewAutodiscoveryHandler returns the Autodiscovery DatadogInstrumentation handler.
func NewAutodiscoveryHandler(dep *Deps) *AutodiscoveryHandler {
	return &AutodiscoveryHandler{
		checkStore: dep.CheckStore,
	}
}

// Name returns the unique handler name.
func (h *AutodiscoveryHandler) Name() string {
	return "autodiscovery"
}

// HasSection reports whether the CR contains Autodiscovery check configuration.
func (h *AutodiscoveryHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && len(cr.Spec.Config.Checks) > 0
}

// SupportsTarget returns whether Autodiscovery check delivery supports the target kind.
func (h *AutodiscoveryHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	// 'Service' kind isn't supported but will be in the future.
	case "Deployment", "DaemonSet", "StatefulSet", "CronJob", "Job":
		return true
	default:
		return false
	}
}

// Validate reports per-check validation errors against spec.config.checks.
func (h *AutodiscoveryHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if cr == nil {
		return nil
	}
	var errs []instrumentation.ValidationError
	for i, check := range cr.Spec.Config.Checks {
		if strings.TrimSpace(check.Integration) == "" {
			errs = append(errs, instrumentation.ValidationError{
				Type:        checksReadyConditionType,
				Reason:      "InvalidIntegration",
				Message:     "integration name must not be empty",
				Field:       fmt.Sprintf("spec.config.checks[%d].integration", i),
				HandlerName: h.Name(),
			})
		}
		if len(check.Instances) == 0 && len(check.Logs) == 0 {
			errs = append(errs, instrumentation.ValidationError{
				Type:        checksReadyConditionType,
				Reason:      "InvalidInstances",
				Message:     "at least one instance or log config is required",
				Field:       fmt.Sprintf("spec.config.checks[%d].instances", i),
				HandlerName: h.Name(),
			})
		}

		if len(check.ContainerImage) == 0 {
			errs = append(errs, instrumentation.ValidationError{
				Type:        checksReadyConditionType,
				Reason:      "InvalidContainerImage",
				Message:     "at least one container image is required",
				Field:       fmt.Sprintf("spec.config.checks[%d].containerImage", i),
				HandlerName: h.Name(),
			})
		}
	}
	return errs
}

// Handle translates check configs into integration.Config entries on Create/Update,
// removes them on Delete, and reports a ChecksReady status.
func (h *AutodiscoveryHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	if cr == nil {
		return instrumentation.HandlerStatus{
			Type:    checksReadyConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "MissingResource",
			Message: "DatadogInstrumentation resource is nil",
		}, nil
	}

	key := cr.Namespace + "/" + cr.Name

	if event == instrumentation.EventDelete {
		h.checkStore.deleteConfigs(key)
		return instrumentation.HandlerStatus{
			Type:    checksReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "Deleted",
			Message: fmt.Sprintf("checks removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	configs := make([]integration.Config, 0, len(cr.Spec.Config.Checks))
	for _, check := range cr.Spec.Config.Checks {
		cfg, err := translateCheck(cr, check)
		if err != nil {
			return instrumentation.HandlerStatus{
				Type:    checksReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "TranslationFailed",
				Message: err.Error(),
			}, nil
		}
		configs = append(configs, cfg)
	}

	h.checkStore.setConfigs(key, configs, cr.Generation, string(cr.UID))

	return instrumentation.HandlerStatus{
		Type:    checksReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "Configured",
		Message: fmt.Sprintf("%d check(s) configured for %s/%s", len(configs), cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
	}, nil
}

func (c *CheckStore) ListConfigs() ([]integration.Config, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]integration.Config, 0)
	for _, cfgs := range c.configs {
		out = append(out, cfgs...)
	}
	return out, c.configHash
}

// ConfigHash returns a deterministic hash of the current set of (key, generation) pairs,
// consistent across all cluster agent replicas for the same CR state.
func (c *CheckStore) Hash() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configHash
}

func (c *CheckStore) setConfigs(key string, configs []integration.Config, generation int64, uid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(configs) == 0 {
		delete(c.configs, key)
		delete(c.states, key)
	} else {
		c.configs[key] = configs
		c.states[key] = fmt.Sprintf("%s:%d", uid, generation)
	}
	c.configHash = c.hashStates()
}

func (c *CheckStore) deleteConfigs(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.configs, key)
	delete(c.states, key)
	c.configHash = c.hashStates()
}

// hashStates computes a deterministic hash from the sorted set of
// "key:uid:generation" entries in the store. Including the UID ensures that a
// recreation of a CR with the same namespace/name is detected even when
// the new CR starts at generation 1.
func (c *CheckStore) hashStates() uint64 {
	keys := make([]string, 0, len(c.states))
	for k := range c.states {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := fnv.New64a()
	for _, k := range keys {
		fmt.Fprintf(h, "%s:%s\n", k, c.states[k]) //nolint:errcheck
	}
	return h.Sum64()
}

func translateCheck(cr *datadoghq.DatadogInstrumentation, check datadoghq.DatadogInstrumentationCheckConfig) (integration.Config, error) {
	initConfig, err := rawExtensionToData(check.InitConfig)
	if err != nil {
		return integration.Config{}, fmt.Errorf("init_config: %w", err)
	}
	if len(initConfig) == 0 {
		initConfig = integration.Data("{}")
	}

	instances := make([]integration.Data, 0, len(check.Instances))
	for j, raw := range check.Instances {
		data, err := rawExtensionToData(raw)
		if err != nil {
			return integration.Config{}, fmt.Errorf("instances[%d]: %w", j, err)
		}
		instances = append(instances, data)
	}

	logsConfig, err := marshalLogs(check.Logs)
	if err != nil {
		return integration.Config{}, fmt.Errorf("logs: %w", err)
	}

	return integration.Config{
		Name:          check.Integration,
		ADIdentifiers: check.ContainerImage,
		InitConfig:    initConfig,
		Instances:     instances,
		LogsConfig:    logsConfig,
		CELSelector:   buildCELSelector(cr.Spec.TargetRef, cr.Namespace),
		Source:        fmt.Sprintf("%s:%s/%s", autodiscoveryProvider, cr.Namespace, cr.Name),
	}, nil
}

func rawExtensionToData(raw runtime.RawExtension) (integration.Data, error) {
	if len(raw.Raw) > 0 {
		return raw.Raw, nil
	}
	if raw.Object == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw.Object)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func marshalLogs(logs []datadoghq.DatadogInstrumentationLogConfig) (integration.Data, error) {
	if len(logs) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(logs)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func buildCELSelector(ref autoscalingv2.CrossVersionObjectReference, namespace string) workloadfilter.Rules {
	expr := fmt.Sprintf(
		`container.pod.rootowner.kind == %q && container.pod.rootowner.name == %q && container.pod.namespace == %q`,
		ref.Kind, ref.Name, namespace,
	)
	return workloadfilter.Rules{
		Containers: []string{expr},
	}
}
