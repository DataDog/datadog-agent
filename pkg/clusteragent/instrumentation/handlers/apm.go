// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/ssi/crstore"
)

const (
	apmReadyConditionType = "APMReady"

	reasonAPMConfigured        = "Configured"
	reasonAPMDeleted           = "Deleted"
	reasonAPMUnsupportedTarget = "UnsupportedTarget"
	reasonAPMUnsupportedLang   = "UnsupportedLanguage"
	reasonAPMInvalidConfig     = "InvalidTracerConfig"
	reasonAPMStoreUnavailable  = "StoreUnavailable"
)

var supportedAPMLanguages = map[string]struct{}{
	"java":   {},
	"js":     {},
	"python": {},
	"dotnet": {},
	"ruby":   {},
	"php":    {},
	"c":      {},
}

// APMHandler translates DatadogInstrumentation APM sections into SSI admission
// webhook configuration.
type APMHandler struct {
	apmStore *crstore.Store
}

// NewAPMHandler returns the APM DatadogInstrumentation handler.
func NewAPMHandler(deps *Deps) *APMHandler {
	return &APMHandler{apmStore: deps.APMStore}
}

// Name returns the unique handler name.
func (h *APMHandler) Name() string {
	return "apm"
}

// HasSection reports whether the CR contains APM configuration.
func (h *APMHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && cr.Spec.Config.APM != nil
}

// SupportsTarget returns whether APM SSI supports the target kind.
func (h *APMHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	case "Deployment", "DaemonSet", "StatefulSet":
		return true
	default:
		return false
	}
}

// Validate reports validation errors against spec.config.apm.
func (h *APMHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if cr == nil || cr.Spec.Config.APM == nil {
		return nil
	}

	var errs []instrumentation.ValidationError
	for lang := range cr.Spec.Config.APM.TracerVersions {
		if _, ok := supportedAPMLanguages[lang]; !ok {
			errs = append(errs, instrumentation.ValidationError{
				Type:        apmReadyConditionType,
				Reason:      reasonAPMUnsupportedLang,
				Message:     fmt.Sprintf("unsupported APM tracer language %q", lang),
				Field:       fmt.Sprintf("spec.config.apm.ddTraceVersions[%s]", lang),
				HandlerName: h.Name(),
			})
		}
	}

	for i, envVar := range cr.Spec.Config.APM.TracerConfigs {
		if !strings.HasPrefix(envVar.Name, "DD_") {
			errs = append(errs, instrumentation.ValidationError{
				Type:        apmReadyConditionType,
				Reason:      reasonAPMInvalidConfig,
				Message:     fmt.Sprintf("APM tracer config %q must start with \"DD_\"", envVar.Name),
				Field:       fmt.Sprintf("spec.config.apm.ddTraceConfigs[%d].name", i),
				HandlerName: h.Name(),
			})
		}
	}

	return errs
}

// Handle applies or removes APM configuration in the shared CR store.
func (h *APMHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	if cr == nil {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "MissingResource",
			Message: "DatadogInstrumentation resource is nil",
		}, nil
	}

	crRef := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}
	if event == instrumentation.EventDelete {
		if h.apmStore != nil {
			h.apmStore.DeleteByCR(crRef)
		}
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMDeleted,
			Message: fmt.Sprintf("APM settings removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	if !h.SupportsTarget(cr.Spec.TargetRef) {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  reasonAPMUnsupportedTarget,
			Message: fmt.Sprintf("APM does not support target kind %q", cr.Spec.TargetRef.Kind),
		}, nil
	}

	if h.apmStore == nil {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  reasonAPMStoreUnavailable,
			Message: "APM CR store is not configured",
		}, errors.New("APM CR store is not configured")
	}

	target := crstore.WorkloadTarget{
		Kind:      cr.Spec.TargetRef.Kind,
		Namespace: cr.Namespace,
		Name:      cr.Spec.TargetRef.Name,
	}
	h.apmStore.UpsertAPM(target, apmConfigFromCR(crRef, cr.Spec.Config.APM))

	return instrumentation.HandlerStatus{
		Type:    apmReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  reasonAPMConfigured,
		Message: fmt.Sprintf("APM settings configured for %s/%s; awaiting pod restart", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
	}, nil
}

func apmConfigFromCR(crRef types.NamespacedName, apm *datadoghq.DatadogInstrumentationAPMConfig) crstore.APMConfig {
	config := crstore.APMConfig{
		CR:      crRef,
		Enabled: apm.Enabled,
	}
	if len(apm.TracerVersions) > 0 {
		config.TracerVersions = make(map[string]string, len(apm.TracerVersions))
		for lang, version := range apm.TracerVersions {
			config.TracerVersions[lang] = version
		}
	}
	if len(apm.TracerConfigs) > 0 {
		config.TracerConfigs = append([]corev1.EnvVar(nil), apm.TracerConfigs...)
	}
	return config
}
