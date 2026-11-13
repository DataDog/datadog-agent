// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package logsimpl implements the DatadogInstrumentation logs handler component.
package logsimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/instrumentation/common"
	logs "github.com/DataDog/datadog-agent/comp/instrumentation/logs/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	logsReadyConditionType = "LogsReady"
	logsCheckName          = "logs"
)

// logsHandler translates DatadogInstrumentation log sections into logs-only
// integration.Config entries stored in the shared check store.
type logsHandler struct {
	checkStore common.ConfigWriter
}

// Requires defines the dependencies of the logs handler.
type Requires struct {
	CheckStore common.ConfigWriter
}

// Provides exposes the logs handler component.
type Provides struct {
	Comp logs.Component
}

// NewComponent constructs the logs handler.
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: &logsHandler{checkStore: reqs.CheckStore}}
}

// Name returns the unique handler name.
func (h *logsHandler) Name() string {
	return "logs"
}

// HasSection reports whether the CR contains log configuration.
func (h *logsHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && len(cr.Spec.Config.Logs) > 0
}

// SupportsTarget returns whether log delivery supports the target kind.
func (h *logsHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	case kubernetes.DeploymentKind, kubernetes.DaemonSetKind, kubernetes.StatefulSetKind, kubernetes.CronJobKind, kubernetes.JobKind, kubernetes.RolloutKind, kubernetes.StrimziPodSetKind:
		return true
	default:
		return false
	}
}

// Validate reports per-log validation errors against spec.config.logs.
func (h *logsHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if cr == nil {
		return nil
	}
	var errs []instrumentation.ValidationError
	for i, logConfig := range cr.Spec.Config.Logs {
		if strings.TrimSpace(logConfig.ContainerName) == "" {
			errs = append(errs, h.logValidationError(i, "containerName", "InvalidContainerName", "container name must not be empty"))
		}
	}
	return errs
}

func (h *logsHandler) logValidationError(index int, field, reason, message string) instrumentation.ValidationError {
	fieldPath := fmt.Sprintf("spec.config.logs[%d]", index)
	if field != "" {
		fieldPath += "." + field
	}
	return instrumentation.ValidationError{
		Type:        logsReadyConditionType,
		Reason:      reason,
		Message:     message,
		Field:       fieldPath,
		HandlerName: h.Name(),
	}
}

// Handle translates log configs on Create/Update, removes them on Delete,
// and reports a LogsReady status.
func (h *logsHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	if cr == nil {
		return instrumentation.HandlerStatus{
			Type:    logsReadyConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "MissingResource",
			Message: "DatadogInstrumentation resource is nil",
		}, nil
	}

	key := logsStoreKey(cr.Namespace + "/" + cr.Name)

	if event == instrumentation.EventDelete {
		h.checkStore.DeleteConfigs(key)
		return instrumentation.HandlerStatus{
			Type:    logsReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "Deleted",
			Message: fmt.Sprintf("logs removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	configs := make([]integration.Config, 0, len(cr.Spec.Config.Logs))
	for i, logConfig := range cr.Spec.Config.Logs {
		cfg, err := translateWorkloadLog(cr, logConfig)
		if err != nil {
			return instrumentation.HandlerStatus{
				Type:    logsReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "TranslationFailed",
				Message: fmt.Sprintf("logs[%d]: %s", i, err),
			}, nil
		}
		configs = append(configs, cfg)
	}

	h.checkStore.WriteConfigs(key, cr, configs)

	return instrumentation.HandlerStatus{
		Type:    logsReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "Configured",
		Message: fmt.Sprintf("%d log config(s) configured for %s/%s", len(configs), cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
	}, nil
}

func translateWorkloadLog(cr *datadoghq.DatadogInstrumentation, logConfig datadoghq.DatadogInstrumentationLogConfig) (integration.Config, error) {
	containerName := strings.TrimSpace(logConfig.ContainerName)
	if containerName == "" {
		return integration.Config{}, errors.New("container name must not be empty")
	}

	config, err := json.Marshal([]datadoghq.DatadogInstrumentationLogFields{logConfig.DatadogInstrumentationLogFields})
	if err != nil {
		return integration.Config{}, err
	}

	return integration.Config{
		Name:          logsCheckName,
		ADIdentifiers: []string{adtypes.KubeContainerNameIdentifier(containerName)},
		LogsConfig:    config,
		CELSelector:   common.RootOwnerCELFilter(cr.Spec.TargetRef, cr.Namespace),
		Source:        fmt.Sprintf("%s:%s/%s", common.AutodiscoveryProvider, cr.Namespace, cr.Name),
	}, nil
}

func logsStoreKey(crKey string) string {
	return crKey + "/logs"
}
