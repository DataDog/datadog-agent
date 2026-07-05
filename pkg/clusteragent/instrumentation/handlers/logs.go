// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	logsReadyConditionType = "LogsReady"
	logsCheckName          = "logs"
)

// LogsHandler translates DatadogInstrumentation log sections into logs-only
// integration.Config entries stored in the shared check store.
type LogsHandler struct {
	checkStore *CheckStore
}

// NewLogsHandler returns the logs DatadogInstrumentation handler.
func NewLogsHandler(dep *Deps) *LogsHandler {
	return &LogsHandler{
		checkStore: dep.CheckStore,
	}
}

// Name returns the unique handler name.
func (h *LogsHandler) Name() string {
	return "logs"
}

// HasSection reports whether the CR contains log configuration.
func (h *LogsHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && len(cr.Spec.Config.Logs) > 0
}

// SupportsTarget returns whether log delivery supports the target kind.
func (h *LogsHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	case kubernetes.DeploymentKind, kubernetes.DaemonSetKind, kubernetes.StatefulSetKind, kubernetes.CronJobKind, kubernetes.JobKind:
		return true
	default:
		return false
	}
}

// Validate reports per-log validation errors against spec.config.logs.
func (h *LogsHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
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

func (h *LogsHandler) logValidationError(index int, field, reason, message string) instrumentation.ValidationError {
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
func (h *LogsHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
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
		h.checkStore.deleteConfigs(key)
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

	h.checkStore.writeConfigs(key, cr, configs)

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
		CELSelector:   rootOwnerCELFilter(cr.Spec.TargetRef, cr.Namespace),
		Source:        fmt.Sprintf("%s:%s/%s", autodiscoveryProvider, cr.Namespace, cr.Name),
	}, nil
}

func logsStoreKey(crKey string) string {
	return crKey + "/logs"
}
