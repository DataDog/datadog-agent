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

	reasonSettingsApplied   = "SettingsApplied"
	reasonSettingsRemoved   = "SettingsRemoved"
	reasonUnsupportedTarget = "UnsupportedTarget"
	reasonUnsupportedLang   = "UnsupportedLanguage"
	reasonInvalidTracerCfg  = "InvalidTracerConfig"

	ddEnvVarPrefix = "DD_"
)

// supportedAPMLanguages mirrors the autoinstrumentation supported languages.
// Keep in sync with pkg/clusteragent/admission/mutate/autoinstrumentation/language_versions.go.
var supportedAPMLanguages = map[string]bool{
	"java":   true,
	"js":     true,
	"python": true,
	"dotnet": true,
	"ruby":   true,
	"php":    true,
}

// supportedAPMKinds is the set of target workload kinds the APM handler can apply to.
var supportedAPMKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
}

// APMHandler reconciles the APM section of DatadogInstrumentation resources by
// publishing per-workload APM configuration to the shared CRStore.
type APMHandler struct {
	deps Deps
}

// NewAPMHandler returns a new APMHandler.
func NewAPMHandler(deps Deps) *APMHandler {
	return &APMHandler{deps: deps}
}

// Name returns the unique handler name.
func (h *APMHandler) Name() string {
	return "apm"
}

// HasSection reports whether the CR carries APM configuration.
func (h *APMHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && cr.Spec.Config.APM != nil
}

// SupportsTarget reports whether APM injection supports the target workload kind.
func (h *APMHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	return supportedAPMKinds[ref.Kind]
}

// Validate performs CRD-level validation for the APM section.
func (h *APMHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if cr == nil || cr.Spec.Config.APM == nil {
		return nil
	}

	var errs []instrumentation.ValidationError

	if !supportedAPMKinds[cr.Spec.TargetRef.Kind] {
		errs = append(errs, instrumentation.ValidationError{
			Type:        apmReadyConditionType,
			Reason:      reasonUnsupportedTarget,
			Message:     fmt.Sprintf("APM does not support target kind %q", cr.Spec.TargetRef.Kind),
			Field:       "spec.targetRef.kind",
			HandlerName: h.Name(),
		})
	}

	for lang := range cr.Spec.Config.APM.TracerVersions {
		if !supportedAPMLanguages[lang] {
			errs = append(errs, instrumentation.ValidationError{
				Type:        apmReadyConditionType,
				Reason:      reasonUnsupportedLang,
				Message:     fmt.Sprintf("unsupported APM tracer language %q", lang),
				Field:       fmt.Sprintf("spec.config.apm.ddTraceVersions[%s]", lang),
				HandlerName: h.Name(),
			})
		}
	}

	for i, envVar := range cr.Spec.Config.APM.TracerConfigs {
		if !strings.HasPrefix(envVar.Name, ddEnvVarPrefix) {
			errs = append(errs, instrumentation.ValidationError{
				Type:        apmReadyConditionType,
				Reason:      reasonInvalidTracerCfg,
				Message:     fmt.Sprintf("APM tracer config %q must start with %q", envVar.Name, ddEnvVarPrefix),
				Field:       fmt.Sprintf("spec.config.apm.ddTraceConfigs[%d].name", i),
				HandlerName: h.Name(),
			})
		}
	}

	return errs
}

// Handle reconciles the APM section by upserting or removing the entry in the
// CRStore. It returns a status condition reflecting whether the configuration
// was applied to the store. Actual pod-injection success is not reported here;
// per-pod injection feedback will be added in a follow-up.
func (h *APMHandler) Handle(_ context.Context, eventType instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	if cr == nil {
		return instrumentation.HandlerStatus{}, nil
	}
	crRef := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}

	if eventType == instrumentation.EventDelete {
		if h.deps.CRStore != nil {
			h.deps.CRStore.DeleteByCR(crRef)
		}
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  reasonSettingsRemoved,
			Message: "APM settings removed",
		}, nil
	}

	if !h.SupportsTarget(cr.Spec.TargetRef) {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  reasonUnsupportedTarget,
			Message: fmt.Sprintf("APM does not support target kind %q", cr.Spec.TargetRef.Kind),
		}, nil
	}

	if h.deps.CRStore == nil {
		return instrumentation.HandlerStatus{}, errors.New("APM handler: CRStore is not configured")
	}

	workload := crstore.WorkloadKey{
		Kind:      cr.Spec.TargetRef.Kind,
		Namespace: cr.Namespace,
		Name:      cr.Spec.TargetRef.Name,
	}
	h.deps.CRStore.UpsertAPM(workload, apmEntryFromCR(crRef, cr.Spec.Config.APM))

	return instrumentation.HandlerStatus{
		Type:    apmReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  reasonSettingsApplied,
		Message: "APM settings applied; awaiting pod restart",
	}, nil
}

func apmEntryFromCR(crRef types.NamespacedName, apm *datadoghq.DatadogInstrumentationAPMConfig) crstore.APMEntry {
	entry := crstore.APMEntry{
		CR:      crRef,
		Enabled: apm.Enabled,
	}
	if len(apm.TracerVersions) > 0 {
		entry.TracerVersions = make(map[string]string, len(apm.TracerVersions))
		for k, v := range apm.TracerVersions {
			entry.TracerVersions[k] = v
		}
	}
	if len(apm.TracerConfigs) > 0 {
		entry.TracerConfigs = append([]corev1.EnvVar(nil), apm.TracerConfigs...)
	}
	return entry
}
