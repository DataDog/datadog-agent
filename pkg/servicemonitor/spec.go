package servicemonitor

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogServiceMonitorSpec defines the desired state of DatadogServiceMonitor
type DatadogServiceMonitorSpec struct {
	// Name is the name of the target. It will be appended to the pod annotations to identify the target that was used.
	// +optional
	Name string `json:"name,omitempty"`
	// PodSelector is the pod selector to match the pods to apply the auto instrumentation to. It will be used in
	// conjunction with the NamespaceSelector to match the pods.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// NamespaceSelector is the namespace selector to match the namespaces to apply the auto instrumentation to. It will
	// be used in conjunction with the Selector to match the pods.
	// +optional
	NamespaceSelector *NamespaceSelector `json:"namespaceSelector,omitempty"`
	// TracerVersions is a map of tracer versions to inject for workloads that match the target. The key is the tracer
	// name and the value is the version to inject.
	// +optional
	TracerVersions map[string]string `json:"ddTraceVersions,omitempty"`
	// TracerConfigs is a list of configuration options to use for the installed tracers. These options will be added
	// as environment variables in addition to the injected tracer.
	// +optional
	// +listType=map
	// +listMapKey=name
	TracerConfigs []corev1.EnvVar `json:"ddTraceConfigs,omitempty"`
	// Priority is the priority of the service monitor.
	// +optional
	Priority int `json:"priority,omitempty"`
}

// NamespaceSelector is a struct to store the configuration for the namespace selector. It can be used to match the
// namespaces to apply the auto instrumentation to.
type NamespaceSelector struct {
	// MatchNames is a list of namespace names to match. If empty, all namespaces are matched.
	// +optional
	MatchNames []string `json:"matchNames,omitempty"`
	// MatchLabels is a map of key-value pairs to match the labels of the namespace. The labels and expressions are
	// ANDed. This cannot be used with MatchNames.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	// MatchExpressions is a list of label selector requirements to match the labels of the namespace. The labels and
	// expressions are ANDed. This cannot be used with MatchNames.
	// +optional
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// DatadogServiceMonitorStatus defines the observed state of DatadogServiceMonitor
type DatadogServiceMonitorStatus struct {
	// Conditions Represents the latest available observations of a DatadogServiceMonitor's current state.
	// +listType=map
	// +listMapKey=type
	Conditions []DatadogServiceMonitorCondition `json:"conditions,omitempty"`
}

// DatadogServiceMonitorCondition describes the state of a DatadogServiceMonitor at a certain point.
// +k8s:openapi-gen=true
type DatadogServiceMonitorCondition struct {
	// Type of DatadogServiceMonitor condition.
	Type DatadogServiceMonitorConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Last time the condition was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// DatadogServiceMonitorConditionType type use to represent a DatadogServiceMonitor condition
type DatadogServiceMonitorConditionType string

const (
	// DatadogServiceMonitorConditionTypeActive DatadogServiceMonitor is active (referenced by an HPA), Datadog will only be queried for active metrics
	DatadogServiceMonitorConditionTypeActive DatadogServiceMonitorConditionType = "Active"
	// DatadogServiceMonitorConditionTypeUpdated DatadogServiceMonitor is updated
	DatadogServiceMonitorConditionTypeUpdated DatadogServiceMonitorConditionType = "Updated"
	// DatadogServiceMonitorConditionTypeValid DatadogServiceMonitor.spec.podSelector is invalid
	DatadogServiceMonitorConditionTypeValid DatadogServiceMonitorConditionType = "Valid"
	// DatadogServiceMonitorConditionTypeError the controller wasn't able to handle this DatadogServiceMonitor
	DatadogServiceMonitorConditionTypeError DatadogServiceMonitorConditionType = "Error"
)

// DatadogServiceMonitor allows a user to define and manage datadog service monitors from Kubernetes cluster.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogservicemonitors,scope=Namespaced
// +kubebuilder:printcolumn:name="active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
// +kubebuilder:printcolumn:name="valid",type="string",JSONPath=".status.conditions[?(@.type=='Valid')].status"
// +kubebuilder:printcolumn:name="update time",type="date",JSONPath=".status.conditions[?(@.type=='Updated')].lastUpdateTime"
// +k8s:openapi-gen=true
// +genclient
type DatadogServiceMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogServiceMonitorSpec   `json:"spec,omitempty"`
	Status DatadogServiceMonitorStatus `json:"status,omitempty"`
}

// DatadogServiceMonitorList contains a list of DatadogServiceMonitor
// +kubebuilder:object:root=true
type DatadogServiceMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogServiceMonitor `json:"items"`
}
