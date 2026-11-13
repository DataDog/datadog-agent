// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package checksimpl implements the DatadogInstrumentation checks handler component.
package checksimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checks "github.com/DataDog/datadog-agent/comp/instrumentation/checks/def"
	"github.com/DataDog/datadog-agent/comp/instrumentation/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	checksReadyConditionType = "ChecksReady"
)

// checksHandler translates DatadogInstrumentation check sections into
// integration.Config entries. It supports both workload targets (Deployment,
// DaemonSet, etc.) and Service targets, branching internally on the target kind.
type checksHandler struct {
	checkStore           common.ConfigWriter
	templateStore        common.ServiceTemplateWriter
	serviceTargetEnabled bool
}

// Requires defines the dependencies of the checks handler.
type Requires struct {
	CheckStore                common.ConfigWriter
	ServiceCheckTemplateStore common.ServiceTemplateWriter
}

// Provides exposes the checks handler component.
type Provides struct {
	Comp checks.Component
}

// NewComponent constructs the checks handler.
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: &checksHandler{
		checkStore:    reqs.CheckStore,
		templateStore: reqs.ServiceCheckTemplateStore,
	}}
}

// SetServiceTargetEnabled configures Service target support before the handler is used.
func (h *checksHandler) SetServiceTargetEnabled(enabled bool) {
	h.serviceTargetEnabled = enabled
}

// Name returns the unique handler name.
func (h *checksHandler) Name() string {
	return "checks"
}

// HasSection reports whether the CR contains Autodiscovery check configuration.
func (h *checksHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && len(cr.Spec.Config.Checks) > 0
}

// SupportsTarget returns whether Autodiscovery check delivery supports the target kind.
func (h *checksHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	case kubernetes.DeploymentKind, kubernetes.DaemonSetKind, kubernetes.StatefulSetKind, kubernetes.CronJobKind, kubernetes.JobKind, kubernetes.RolloutKind, kubernetes.StrimziPodSetKind:
		return true
	case kubernetes.ServiceKind:
		// Service target support is backed by endpoint slices CR provider. If Endpointslice collection
		// is disabled then service targets can't be supported.
		return h.serviceTargetEnabled
	default:
		return false
	}
}

// Validate reports per-check validation errors against spec.config.checks.
func (h *checksHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if cr == nil {
		return nil
	}
	var errs []instrumentation.ValidationError
	for i, check := range cr.Spec.Config.Checks {
		if strings.TrimSpace(check.Integration) == "" {
			errs = append(errs, h.checkValidationError(i, "integration", "InvalidIntegration", "integration name must not be empty"))
		}
		if len(check.Instances) == 0 {
			errs = append(errs, h.checkValidationError(i, "instances", "InvalidInstances", "at least one instance is required"))
		}

		if !isService(cr) && !hasContainerName(check) {
			errs = append(errs, h.checkValidationError(i, "containerName", "InvalidContainerTarget", "container name is required"))
		}
	}
	return errs
}

func (h *checksHandler) checkValidationError(index int, field, reason, message string) instrumentation.ValidationError {
	fieldPath := fmt.Sprintf("spec.config.checks[%d]", index)
	if field != "" {
		fieldPath += "." + field
	}
	return instrumentation.ValidationError{
		Type:        checksReadyConditionType,
		Reason:      reason,
		Message:     message,
		Field:       fieldPath,
		HandlerName: h.Name(),
	}
}

// Handle translates check configs on Create/Update, removes them on Delete,
// and reports a ChecksReady status. For Service targets the configs are stored
// as templates; for workload targets they are stored directly.
func (h *checksHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
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
		if isService(cr) {
			h.templateStore.DeleteTemplates(key)
		} else {
			h.checkStore.DeleteConfigs(key)
		}

		return instrumentation.HandlerStatus{
			Type:    checksReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "Deleted",
			Message: fmt.Sprintf("checks removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	configs := make([]integration.Config, 0, len(cr.Spec.Config.Checks))
	for _, check := range cr.Spec.Config.Checks {
		var cfg integration.Config
		var err error
		if isService(cr) {
			cfg, err = translateServiceCheck(cr, check)
		} else {
			cfg, err = translateWorkloadCheck(cr, check)
		}
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

	if isService(cr) {
		h.templateStore.WriteTemplates(key, cr, configs)
	} else {
		h.checkStore.WriteConfigs(key, cr, configs)
	}

	return instrumentation.HandlerStatus{
		Type:    checksReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "Configured",
		Message: fmt.Sprintf("%d check(s) configured for %s/%s", len(configs), cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
	}, nil
}

func translateWorkloadCheck(cr *datadoghq.DatadogInstrumentation, check datadoghq.DatadogInstrumentationCheckConfig) (integration.Config, error) {
	initConfig, instances, err := translateCheckFields(check)
	if err != nil {
		return integration.Config{}, err
	}

	var adIdentifiers []string
	if hasContainerName(check) {
		adIdentifiers = []string{adtypes.KubeContainerNameIdentifier(strings.TrimSpace(check.ContainerName))}
	}

	return integration.Config{
		Name:                    check.Integration,
		ADIdentifiers:           adIdentifiers,
		InitConfig:              initConfig,
		Instances:               instances,
		IgnoreAutodiscoveryTags: check.IgnoreAutodiscoveryTags,
		CheckTagCardinality:     check.CheckTagCardinality,
		CELSelector:             common.RootOwnerCELFilter(cr.Spec.TargetRef, cr.Namespace),
		Source:                  fmt.Sprintf("%s:%s/%s", common.AutodiscoveryProvider, cr.Namespace, cr.Name),
	}, nil
}

func translateServiceCheck(cr *datadoghq.DatadogInstrumentation, check datadoghq.DatadogInstrumentationCheckConfig) (integration.Config, error) {
	initConfig, instances, err := translateCheckFields(check)
	if err != nil {
		return integration.Config{}, err
	}
	return integration.Config{
		Name:                    check.Integration,
		InitConfig:              initConfig,
		Instances:               instances,
		IgnoreAutodiscoveryTags: check.IgnoreAutodiscoveryTags,
		CheckTagCardinality:     check.CheckTagCardinality,
		Source:                  fmt.Sprintf("%s:%s/%s", common.AutodiscoveryProvider, cr.Namespace, cr.Name),
	}, nil
}

func translateCheckFields(check datadoghq.DatadogInstrumentationCheckConfig) (integration.Data, []integration.Data, error) {
	initConfig, err := rawExtensionToData(check.InitConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("init_config: %w", err)
	}
	if len(initConfig) == 0 {
		initConfig = integration.Data("{}")
	}

	instances := make([]integration.Data, 0, len(check.Instances))
	for j, raw := range check.Instances {
		data, err := rawExtensionToData(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("instances[%d]: %w", j, err)
		}
		instances = append(instances, data)
	}

	return initConfig, instances, nil
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

func hasContainerName(check datadoghq.DatadogInstrumentationCheckConfig) bool {
	return strings.TrimSpace(check.ContainerName) != ""
}

func isService(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && cr.Spec.TargetRef.Kind == "Service"
}
