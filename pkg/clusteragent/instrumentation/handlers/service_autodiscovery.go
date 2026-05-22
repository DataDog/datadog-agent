// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"fmt"
	"sync"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

const (
	serviceAutodiscoveryProvider = "datadoginstrumentation"
	serviceAutodiscoveryName     = "service-autodiscovery"
)

// serviceTemplateEntry links a DDI CR key to a target service and its check templates.
type serviceTemplateEntry struct {
	serviceNamespace string
	serviceName      string
	templates        []integration.Config
}

// ServiceCheckTemplateStore holds check templates for Service-targeted DDI CRs.
// The handler writes templates here; a separate AD config provider reads them and
// combines with EndpointSlice data to produce per-endpoint configs.
type ServiceCheckTemplateStore struct {
	mu sync.RWMutex
	// entries maps DDI CR key (namespace/name) to the target service and templates.
	entries map[string]serviceTemplateEntry
	// onChange is called when templates are added or removed.
	onChange func()
}

// NewServiceCheckTemplateStore creates a new ServiceCheckTemplateStore.
func NewServiceCheckTemplateStore() *ServiceCheckTemplateStore {
	return &ServiceCheckTemplateStore{
		entries: make(map[string]serviceTemplateEntry),
	}
}

// SetOnChange registers a callback invoked when the template set changes.
func (s *ServiceCheckTemplateStore) SetOnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// HasService reports whether any templates target the given service.
func (s *ServiceCheckTemplateStore) HasService(namespace, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.entries {
		if entry.serviceNamespace == namespace && entry.serviceName == name {
			return true
		}
	}
	return false
}

// AllTemplatesByService returns all templates grouped by "namespace/name" service key
// in a single pass. This avoids repeated lock acquisitions per service.
func (s *ServiceCheckTemplateStore) AllTemplatesByService() map[string][]integration.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]integration.Config)
	for _, entry := range s.entries {
		key := entry.serviceNamespace + "/" + entry.serviceName
		out[key] = append(out[key], entry.templates...)
	}
	return out
}

// setTemplates stores check templates for a DDI CR targeting a specific service.
// The onChange callback is invoked after the lock is released to avoid nested lock acquisition.
func (s *ServiceCheckTemplateStore) setTemplates(ddiKey, serviceNamespace, serviceName string, templates []integration.Config) {
	s.mu.Lock()
	if len(templates) == 0 {
		delete(s.entries, ddiKey)
	} else {
		s.entries[ddiKey] = serviceTemplateEntry{
			serviceNamespace: serviceNamespace,
			serviceName:      serviceName,
			templates:        templates,
		}
	}
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

// deleteTemplates removes all templates for a DDI CR.
func (s *ServiceCheckTemplateStore) deleteTemplates(ddiKey string) {
	s.mu.Lock()
	delete(s.entries, ddiKey)
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

// templatesForService returns all check templates targeting a given service.
func (s *ServiceCheckTemplateStore) templatesForService(namespace, name string) []integration.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []integration.Config
	for _, entry := range s.entries {
		if entry.serviceNamespace == namespace && entry.serviceName == name {
			out = append(out, entry.templates...)
		}
	}
	return out
}

// trackServices returns the set of service namespace/name keys that have templates.
func (s *ServiceCheckTemplateStore) trackServices() map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	trackedServices := make(map[string]struct{})
	for _, entry := range s.entries {
		trackedServices[entry.serviceNamespace+"/"+entry.serviceName] = struct{}{}
	}
	return trackedServices
}

// ServiceAutodiscoveryHandler translates DatadogInstrumentation check sections
// targeting Services into check templates stored in a ServiceCheckTemplateStore.
// A separate AD config provider resolves these templates against EndpointSlice data
// to produce per-endpoint integration.Config entries for the cluster check dispatcher.
type ServiceAutodiscoveryHandler struct {
	templateStore *ServiceCheckTemplateStore
}

// NewServiceAutodiscoveryHandler returns a new ServiceAutodiscoveryHandler.
func NewServiceAutodiscoveryHandler(dep *Deps) *ServiceAutodiscoveryHandler {
	return &ServiceAutodiscoveryHandler{
		templateStore: dep.ServiceCheckTemplateStore,
	}
}

// Name returns the unique handler name.
func (h *ServiceAutodiscoveryHandler) Name() string {
	return serviceAutodiscoveryName
}

// HasSection reports whether the CR contains Autodiscovery check configuration.
func (h *ServiceAutodiscoveryHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && len(cr.Spec.Config.Checks) > 0 && cr.Spec.TargetRef.Kind == "Service"
}

// SupportsTarget returns true only for Service kind targets.
func (h *ServiceAutodiscoveryHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	return ref.Kind == "Service"
}

// Validate reports per-check validation errors against spec.config.checks.
func (h *ServiceAutodiscoveryHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	return validateChecks(cr, h.Name())
}

// Handle translates check configs into templates on Create/Update,
// removes them on Delete, and reports a ChecksReady status.
func (h *ServiceAutodiscoveryHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
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
		h.templateStore.deleteTemplates(key)
		return instrumentation.HandlerStatus{
			Type:    checksReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "Deleted",
			Message: fmt.Sprintf("checks removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	templates := make([]integration.Config, 0, len(cr.Spec.Config.Checks))
	for _, check := range cr.Spec.Config.Checks {
		cfg, err := translateServiceCheck(cr, check)
		if err != nil {
			return instrumentation.HandlerStatus{
				Type:    checksReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "TranslationFailed",
				Message: err.Error(),
			}, nil
		}
		templates = append(templates, cfg)
	}

	h.templateStore.setTemplates(key, cr.Namespace, cr.Spec.TargetRef.Name, templates)

	return instrumentation.HandlerStatus{
		Type:    checksReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "Configured",
		Message: fmt.Sprintf("%d check(s) configured for %s/%s", len(templates), cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
	}, nil
}

func translateServiceCheck(cr *datadoghq.DatadogInstrumentation, check datadoghq.DatadogInstrumentationCheckConfig) (integration.Config, error) {
	initConfig, instances, logsConfig, err := translateConfigChecks(check)
	if err != nil {
		return integration.Config{}, err
	}

	return integration.Config{
		Name:       check.Integration,
		InitConfig: initConfig,
		Instances:  instances,
		LogsConfig: logsConfig,
		Source:     fmt.Sprintf("%s:%s/%s", serviceAutodiscoveryProvider, cr.Namespace, cr.Name),
	}, nil
}
