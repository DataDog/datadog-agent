// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/ssi/crstore"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	apmReadyConditionType = "APMReady"

	reasonAPMConfigured      = "Configured"
	reasonAPMDeleted         = "Deleted"
	reasonAPMUnsupportedLang = "UnsupportedLanguage"
	reasonAPMInvalidConfig   = "InvalidTracerConfig"

	reasonAPMRolloutTriggered  = "RolloutTriggered"
	reasonAPMRolloutCurrent    = "RolloutCurrent"
	reasonAPMRolloutSkipped    = "RolloutSkipped"
	reasonAPMRolloutFailed     = "RolloutFailed"
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
	apmStore       *crstore.Store
	rolloutPatcher APMRolloutPatcher
}

// NewAPMHandler returns the APM DatadogInstrumentation handler.
func NewAPMHandler(deps *Deps) *APMHandler {
	var rolloutPatcher APMRolloutPatcher
	if deps.UpdateClient != nil {
		rolloutPatcher = NewAPMDeploymentRolloutPatcher(deps.UpdateClient, deps.IsLeader)
	}

	return &APMHandler{
		apmStore:       deps.APMStore,
		rolloutPatcher: rolloutPatcher,
	}
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
	case kubernetes.DeploymentKind, kubernetes.DaemonSetKind, kubernetes.StatefulSetKind, kubernetes.CronJobKind, kubernetes.JobKind:
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
func (h *APMHandler) Handle(ctx context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
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
		h.apmStore.DeleteByCR(crRef)
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMDeleted,
			Message: fmt.Sprintf("APM settings removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	target := crstore.WorkloadTarget{
		Kind:      cr.Spec.TargetRef.Kind,
		Namespace: cr.Namespace,
		Name:      cr.Spec.TargetRef.Name,
	}
	apmConfig := apmConfigFromCR(crRef, cr.Spec.Config.APM)
	h.apmStore.UpsertAPM(target, apmConfig)

	if cr.Spec.TargetRef.Kind != kubernetes.DeploymentKind {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMConfigured,
			Message: fmt.Sprintf("APM settings configured for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	if !apmConfig.Enabled {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMConfigured,
			Message: fmt.Sprintf("APM settings configured for %s/%s; rollout not triggered because APM is disabled", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	if h.rolloutPatcher == nil {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMConfigured,
			Message: fmt.Sprintf("APM settings configured for %s/%s; awaiting pod restart", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	result, err := h.rolloutPatcher.RolloutDeployment(ctx, target.Namespace, target.Name, apmRolloutConfigHash(target, apmConfig))
	if err != nil {
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  reasonAPMRolloutFailed,
			Message: fmt.Sprintf("APM settings configured for %s/%s; failed to trigger rollout: %v", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name, err),
		}, err
	}

	switch result.State {
	case APMRolloutTriggered:
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMRolloutTriggered,
			Message: fmt.Sprintf("APM settings configured for %s/%s; rollout triggered", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	case APMRolloutAlreadyCurrent:
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMRolloutCurrent,
			Message: fmt.Sprintf("APM settings configured for %s/%s; rollout already reflects current configuration", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	case APMRolloutSkipped:
		return instrumentation.HandlerStatus{
			Type:    apmReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAPMRolloutSkipped,
			Message: fmt.Sprintf("APM settings configured for %s/%s; rollout skipped because this Cluster Agent is not the leader", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

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

func apmRolloutConfigHash(target crstore.WorkloadTarget, config crstore.APMConfig) string {
	h := sha256.New()

	writeHashField := func(value string) {
		h.Write([]byte(value))
		h.Write([]byte{0})
	}

	writeHashField(config.CR.Namespace)
	writeHashField(config.CR.Name)
	writeHashField(target.Kind)
	writeHashField(target.Namespace)
	writeHashField(target.Name)
	writeHashField(fmt.Sprintf("%t", config.Enabled))

	langs := make([]string, 0, len(config.TracerVersions))
	for lang := range config.TracerVersions {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		writeHashField(lang)
		writeHashField(config.TracerVersions[lang])
	}

	for _, envVar := range config.TracerConfigs {
		payload, err := json.Marshal(envVar)
		if err != nil {
			writeHashField(envVar.Name)
			writeHashField(envVar.Value)
			continue
		}
		writeHashField(string(payload))
	}

	return hex.EncodeToString(h.Sum(nil))
}
