// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
)

// profileHashInput is used to compute a hash that covers both the profile
// template spec and the annotations that are forwarded to generated DPAs
// (currently the PreviewAnnotation). Including the annotation ensures that
// toggling preview features (e.g. burstable) triggers a DPA reconcile even
// when the template spec itself has not changed.
type profileHashInput struct {
	Template datadoghq.DatadogPodAutoscalerTemplate `json:"template"`
	Preview  string                                 `json:"preview,omitempty"`
}

const (
	// ProfileLabelKey is the label key used to associate a workload with a profile
	// and to mark generated DPAs as profile-managed. Setting this label to
	// ProfileExcludedValue opts the workload out of namespace-level profile discovery.
	ProfileLabelKey = "autoscaling.datadoghq.com/profile"

	// ProfileExcludedValue is the sentinel value for ProfileLabelKey that excludes
	// a workload from namespace-level autoscaling profile discovery.
	ProfileExcludedValue = "excluded"

	// ProfileTemplateHashAnnotation stores the applied profile template hash on
	// profile-managed DPA objects so it survives controller restarts.
	ProfileTemplateHashAnnotation = "autoscaling.datadoghq.com/profile-hash"
)

// dummyTargetRef is used for validation only — the profile template doesn't
// include a target ref, but ValidateAutoscalerSpec needs a full spec.
var dummyTargetRef = autoscalingv2.CrossVersionObjectReference{
	Kind:       "Deployment",
	Name:       "dummy",
	APIVersion: "apps/v1",
}

// NamespacedObjectReference identifies a namespaced workload with its API version information.
type NamespacedObjectReference struct {
	schema.GroupKind
	Version   string
	Namespace string
	Name      string
}

// APIVersion returns the full API version string (e.g. "apps/v1").
func (r NamespacedObjectReference) APIVersion() string {
	if r.Group == "" {
		return r.Version
	}
	return r.Group + "/" + r.Version
}

// PodAutoscalerProfileInternal holds the parsed/validated profile data.
type PodAutoscalerProfileInternal struct {
	// name is the profile name (cluster-scoped, so no namespace)
	name string

	// generation is the received generation of the profile CRD
	generation int64

	// template is the autoscaling template from the profile spec
	template *datadoghq.DatadogPodAutoscalerTemplate

	// valid indicates whether the profile template passed validation
	valid bool

	// validationError holds the validation error if the profile is invalid
	validationError error

	// previewAnnotation holds the raw value of the preview annotation from the profile
	// (e.g. `{"burstable":true}`), forwarded transparently to all generated DPA resources.
	previewAnnotation string

	// computed fields

	// templateHash is a deterministic hash of the template AND forwarded annotations
	// so that toggling preview features triggers a DPA reconcile.
	templateHash string

	// workloads holds the list of workload references discovered by the WorkloadWatcher
	// key is the DPA key (namespace/name)
	workloads map[string]NamespacedObjectReference
}

// NewPodAutoscalerProfileInternal creates a new PodAutoscalerProfileInternal.
func NewPodAutoscalerProfileInternal(profile *datadoghq.DatadogPodAutoscalerClusterProfile) (PodAutoscalerProfileInternal, error) {
	p := PodAutoscalerProfileInternal{
		name: profile.Name,
	}

	err := p.UpdateFromProfile(profile)
	if err != nil {
		return PodAutoscalerProfileInternal{}, err
	}
	p.UpdateFromStatus(&profile.Status)

	return p, nil
}

func (p *PodAutoscalerProfileInternal) UpdateFromProfile(profile *datadoghq.DatadogPodAutoscalerClusterProfile) error {
	p.generation = profile.Generation
	p.template = &profile.Spec.Template
	p.previewAnnotation = profile.Annotations[PreviewAnnotationKey]

	// Include the preview annotation in the hash so that toggling preview
	// features (e.g. burstable) triggers a DPA reconcile even when the
	// template spec is unchanged.
	hash, err := autoscaling.ObjectHash(profileHashInput{
		Template: profile.Spec.Template,
		Preview:  profile.Annotations[PreviewAnnotationKey],
	})
	if err != nil {
		return err
	}
	p.templateHash = hash

	spec := BuildDPASpecFromProfile(p.template, dummyTargetRef)
	err = ValidateAutoscalerSpec(&spec)
	if err != nil {
		p.valid = false
		p.validationError = err
	} else {
		p.valid = true
		p.validationError = nil
	}

	// Only hash error is returned, otherwise the profile is usable
	return nil
}

// UpdateFromStatus restores internal state from the persisted profile status conditions.
func (p *PodAutoscalerProfileInternal) UpdateFromStatus(status *datadoghq.DatadogPodAutoscalerProfileStatus) {
	for _, cond := range status.Conditions {
		switch {
		case cond.Type == string(datadoghq.DatadogPodAutoscalerProfileValidCondition):
			if cond.Status == metav1.ConditionFalse {
				p.valid = false
				p.validationError = profileErrorFromCondition(cond)
			} else {
				p.valid = true
				p.validationError = nil
			}
		}
	}
}

// Name returns the profile name.
func (p *PodAutoscalerProfileInternal) Name() string { return p.name }

// Generation returns the generation of the profile CRD.
func (p *PodAutoscalerProfileInternal) Generation() int64 { return p.generation }

// Template returns the autoscaling template.
func (p *PodAutoscalerProfileInternal) Template() *datadoghq.DatadogPodAutoscalerTemplate {
	return p.template
}

// TemplateHash returns the hash of the template.
func (p *PodAutoscalerProfileInternal) TemplateHash() string { return p.templateHash }

// Valid returns whether the profile template is valid.
func (p *PodAutoscalerProfileInternal) Valid() bool { return p.valid }

// PreviewAnnotation returns the raw value of the preview annotation from the profile
// (e.g. `{"burstable":true}`), forwarded transparently to all generated DPA resources.
// Returns empty string when no preview annotation is set on the profile.
func (p *PodAutoscalerProfileInternal) PreviewAnnotation() string { return p.previewAnnotation }

// Workloads returns the list of workload references associated with this profile.
func (p *PodAutoscalerProfileInternal) Workloads() map[string]NamespacedObjectReference {
	return p.workloads
}

// UpdateWorkloads updates the list of workload references associated with this profile.
// Returns true if the workloads have changed.
func (p *PodAutoscalerProfileInternal) UpdateWorkloads(workloads []NamespacedObjectReference) bool {
	changes := len(p.workloads) != len(workloads)

	newWorkloads := make(map[string]NamespacedObjectReference)
	for _, workload := range workloads {
		dpaKey := workload.Namespace + "/" + generateDPAName(workload)
		newWorkloads[dpaKey] = workload

		if !changes {
			if _, alreadyExists := p.workloads[dpaKey]; !alreadyExists {
				changes = true
			}
		}
	}

	if changes {
		p.workloads = newWorkloads
	}

	return changes
}

// BuildStatus constructs a DatadogPodAutoscalerClusterProfileStatus from the internal state.
func (p *PodAutoscalerProfileInternal) BuildStatus(currentTime metav1.Time, currentStatus *datadoghq.DatadogPodAutoscalerProfileStatus) datadoghq.DatadogPodAutoscalerProfileStatus {
	status := datadoghq.DatadogPodAutoscalerProfileStatus{
		TemplateHash:          p.templateHash,
		ControlledAutoscalers: int32(len(p.workloads)),
	}

	existingConditions := map[string]*metav1.Condition{
		string(datadoghq.DatadogPodAutoscalerProfileValidCondition): nil,
	}
	if currentStatus != nil {
		for i := range currentStatus.Conditions {
			cond := &currentStatus.Conditions[i]
			if _, ok := existingConditions[cond.Type]; ok {
				existingConditions[cond.Type] = cond
			}
		}
	}

	status.Conditions = append(status.Conditions,
		newProfileConditionFromError(false, currentTime, p.generation, p.validationError,
			string(datadoghq.DatadogPodAutoscalerProfileValidCondition), existingConditions),
	)

	return status
}

// profileErrorFromCondition reconstructs an error from a persisted profile condition,
// preserving the programmatic Reason when available.
func profileErrorFromCondition(cond metav1.Condition) error {
	message := cond.Message
	if message == "" {
		message = cond.Reason
		if message == "" {
			return errors.New("unknown error")
		}
		return errors.New(message)
	}

	if cond.Reason != "" {
		return autoscaling.NewConditionError(autoscaling.ConditionReasonType(cond.Reason), errors.New(message))
	}
	return errors.New(message)
}

func newProfileConditionFromError(
	trueOnError bool,
	currentTime metav1.Time,
	generation int64,
	err error,
	conditionType string,
	existingConditions map[string]*metav1.Condition,
) metav1.Condition {
	var status metav1.ConditionStatus
	var reason, message string

	if err != nil {
		reason, message = reasonAndMessageFromError(err)
		if trueOnError {
			status = metav1.ConditionTrue
		} else {
			status = metav1.ConditionFalse
		}
	} else {
		if trueOnError {
			status = metav1.ConditionFalse
		} else {
			status = metav1.ConditionTrue
		}
	}

	if reason == "" {
		reason = string(conditionType)
	}

	return newProfileCondition(status, reason, message, currentTime, generation, conditionType, existingConditions)
}

func newProfileCondition(
	status metav1.ConditionStatus,
	reason, message string,
	currentTime metav1.Time,
	generation int64,
	conditionType string,
	existingConditions map[string]*metav1.Condition,
) metav1.Condition {
	cond := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}

	prev := existingConditions[conditionType]
	if prev == nil || prev.Status != cond.Status {
		cond.LastTransitionTime = currentTime
	} else {
		cond.LastTransitionTime = prev.LastTransitionTime
	}

	return cond
}

func generateDPAName(ref NamespacedObjectReference) string {
	h := sha256.Sum256([]byte(ref.GroupKind.String() + "/" + ref.Namespace + "/" + ref.Name))
	shortHash := hex.EncodeToString(h[:4])
	return ref.Name + "-" + shortHash
}
