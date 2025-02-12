// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/DataDog/datadog-agent/comp/core/config"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config is a struct to store the configuration for the autoinstrumentation logic. It can be populated using the
// datadog config through NewConfig.
type Config struct {
	// Webhook is the configuration for the autoinstrumentation webhook
	Webhook *WebhookConfig

	// LanguageDetection is the configuration for the language detection
	LanguageDetection *LanguageDetectionConfig

	// Instrumentation is the configuration for the autoinstrumentation logic
	Instrumentation *InstrumentationConfig

	// containerRegistry is the container registry to use for the autoinstrumentation logic
	containerRegistry string

	// precomputed mutators for the security and profiling products
	securityClientLibraryPodMutators  []podMutator
	profilingClientLibraryPodMutators []podMutator

	// initResources is the resource requirements for the init container
	initResources initResourceRequirementConfiguration

	// initSecurityContext is the security context for the init container
	initSecurityContext *corev1.SecurityContext

	// defaultResourceRequirements is the default resource requirements for the init container
	defaultResourceRequirements initResourceRequirementConfiguration

	// version is the version of the autoinstrumentation logic to use. We don't expose this option to the user, and V1
	// is deprecated and slated for removal.
	version version
}

// NewConfig creates a new Config from the datadog config. It returns an error if the configuration is invalid.
func NewConfig(datadogConfig config.Component) (*Config, error) {
	instrumentationConfig, err := NewInstrumentationConfig(datadogConfig)
	if err != nil {
		return nil, err
	}

	version, err := instrumentationVersion(instrumentationConfig.Version)
	if err != nil {
		return nil, fmt.Errorf("invalid version for key apm_config.instrumentation.version: %w", err)
	}

	initResources, err := initDefaultResources(datadogConfig)
	if err != nil {
		return nil, err
	}

	initSecurityContext, err := parseInitSecurityContext(datadogConfig)
	if err != nil {
		return nil, err
	}

	defaultResourceRequirements, err := initDefaultResources(datadogConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to parse init-container's resources from configuration: %w", err)
	}

	containerRegistry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.auto_instrumentation.container_registry")
	return &Config{
		Webhook:                           NewWebhookConfig(datadogConfig),
		LanguageDetection:                 NewLanguageDetectionConfig(datadogConfig),
		Instrumentation:                   instrumentationConfig,
		containerRegistry:                 containerRegistry,
		initResources:                     initResources,
		initSecurityContext:               initSecurityContext,
		defaultResourceRequirements:       defaultResourceRequirements,
		securityClientLibraryPodMutators:  securityClientLibraryConfigMutators(datadogConfig),
		profilingClientLibraryPodMutators: profilingClientLibraryConfigMutators(datadogConfig),
		version:                           version,
	}, nil
}

// WebhookConfig use to store options from the config.Component for the autoinstrumentation webhook
type WebhookConfig struct {
	// IsEnabled is the flag to enable the autoinstrumentation webhook.
	IsEnabled bool
	// Endpoint is the endpoint to use for the autoinstrumentation webhook.
	Endpoint string
}

// NewWebhookConfig retrieves the configuration for the autoinstrumentation webhook from the datadog config
func NewWebhookConfig(datadogConfig config.Component) *WebhookConfig {
	return &WebhookConfig{
		IsEnabled: datadogConfig.GetBool("admission_controller.auto_instrumentation.enabled"),
		Endpoint:  datadogConfig.GetString("admission_controller.auto_instrumentation.endpoint"),
	}
}

// LanguageDetectionConfig is a struct to store the configuration for the language detection. It can be populated using
// the datadog config through NewLanguageDetectionConfig.
type LanguageDetectionConfig struct {
	// Enabled is a flag to enable the language detection. If false, the language detection is disabled. Full config
	// key: language_detection.enabled
	Enabled bool
	// ReportingEnabled is a flag to enable the language detection reporting. If false, the language detection reporting
	// is disabled. Full config key: language_detection.reporting_enabled
	ReportingEnabled bool
	// InjectDetected is a flag to enable the injection of the detected language. If false, the detected language is not
	// injected. Full config key: admission_controller.auto_instrumentation.inject_auto_detected_libraries
	InjectDetected bool
}

// NewLanguageDetectionConfig creates a new LanguageDetectionConfig from the datadog config.
func NewLanguageDetectionConfig(datadogConfig config.Component) *LanguageDetectionConfig {
	return &LanguageDetectionConfig{
		Enabled:          datadogConfig.GetBool("language_detection.enabled"),
		ReportingEnabled: datadogConfig.GetBool("language_detection.reporting.enabled"),
		InjectDetected:   datadogConfig.GetBool("admission_controller.auto_instrumentation.inject_auto_detected_libraries"),
	}
}

// InstrumentationConfig is a struct to store the configuration for the autoinstrumentation logic. It can be populated
// using the datadog config through NewInstrumentationConfig.
type InstrumentationConfig struct {
	// Enabled is a flag to enable the auto instrumentation. If false, the auto instrumentation is disabled with the
	// caveat of the annotation based instrumentation. Full config
	// key: apm_config.instrumentation.enabled
	Enabled bool `mapstructure:"enabled"`
	// EnabledNamespaces is a list of namespaces where the autoinstrumentation is enabled. If empty, it is enabled in
	// all namespaces. EnabledNamespace and DisabledNamespaces are mutually exclusive and cannot be set together. Full
	// config key: apm_config.instrumentation.enabled_namespaces
	EnabledNamespaces []string `mapstructure:"enabled_namespaces"`
	// DisabledNamespaces is a list of namespaces where the autoinstrumentation is disabled. If empty, it is enabled in
	// all namespaces. EnabledNamespace and DisabledNamespaces are mutually exclusive and cannot be set together. Full
	// config key: apm_config.instrumentation.disabled_namespaces
	DisabledNamespaces []string `mapstructure:"disabled_namespaces"`
	// LibVersions is a map of tracer libraries to inject with their versions. The key is the language and the value is
	// the version of the library to inject. If empty, the auto instrumentation will inject all libraries. Full config
	// key: apm_config.instrumentation.lib_versions
	LibVersions map[string]string `mapstructure:"lib_versions"`
	// Version is the version of the autoinstrumentation logic to use. We don't expose this option to the user, and V1
	// is deprecated and slated for removal. Full config key: apm_config.instrumentation.version
	Version string `mapstructure:"version"`
	// InjectorImageTag is the tag of the image to use for the auto instrumentation injector library. Full config key:
	// apm_config.instrumentation.injector_image_tag
	InjectorImageTag string `mapstructure:"injector_image_tag"`
	// Targets is a list of targets to apply the auto instrumentation to. The first target that matches the pod will be
	// used. If no target matches, the auto instrumentation will not be applied. Full config key:
	// apm_config.instrumentation.targets
	Targets []Target `mapstructure:"targets"`
}

// NewInstrumentationConfig creates a new InstrumentationConfig from the datadog config. It returns an error if the
// configuration is invalid.
func NewInstrumentationConfig(datadogConfig config.Component) (*InstrumentationConfig, error) {
	cfg := &InstrumentationConfig{}
	err := datadogConfig.UnmarshalKey("apm_config.instrumentation", cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to parse apm_config.instrumentation: %w", err)
	}

	// Ensure both enabled and disabled namespaces are not set together.
	if len(cfg.EnabledNamespaces) > 0 && len(cfg.DisabledNamespaces) > 0 {
		return nil, fmt.Errorf("apm.instrumentation.enabled_namespaces and apm.instrumentation.disabled_namespaces are mutually exclusive and cannot be set together")
	}

	// Ensure both enabled namespaces and targets are not set together.
	if len(cfg.EnabledNamespaces) > 0 && len(cfg.Targets) > 0 {
		return nil, fmt.Errorf("apm.instrumentation.enabled_namespaces and apm.instrumentation.targets are mutually exclusive and cannot be set together")
	}

	// Ensure both library versions and targets are not set together.
	if len(cfg.LibVersions) > 0 && len(cfg.Targets) > 0 {
		return nil, fmt.Errorf("apm.instrumentation.lib_versions and apm.instrumentation.targets are mutually exclusive and cannot be set together")
	}

	// Ensure both namespace names and labels are not set together.
	for _, target := range cfg.Targets {
		if len(target.NamespaceSelector.MatchNames) > 0 && (len(target.NamespaceSelector.MatchLabels) > 0 || len(target.NamespaceSelector.MatchExpressions) > 0) {
			return nil, fmt.Errorf("apm.instrumentation.targets[].namespaceSelector.matchNames and apm.instrumentation.targets[].namespaceSelector.matchLabels/matchExpressions are mutually exclusive and cannot be set together")
		}
	}

	return cfg, nil
}

// Target is a rule to apply the auto instrumentation to a specific workload using the pod and namespace selectors.
// Full config key: apm_config.instrumentation.targets to get the list of targets.
type Target struct {
	// Name is the name of the target. It will be appended to the pod annotations to identify the target that was used.
	// Full config key: apm_config.instrumentation.targets[].name
	Name string `mapstructure:"name"`
	// PodSelector is the pod selector to match the pods to apply the auto instrumentation to. It will be used in
	// conjunction with the NamespaceSelector to match the pods. Full config key:
	// apm_config.instrumentation.targets[].selector
	PodSelector PodSelector `mapstructure:"podSelector"`
	// NamespaceSelector is the namespace selector to match the namespaces to apply the auto instrumentation to. It will
	// be used in conjunction with the Selector to match the pods. Full config key:
	// apm_config.instrumentation.targets[].namespaceSelector
	NamespaceSelector NamespaceSelector `mapstructure:"namespaceSelector"`
	// TracerVersions is a map of tracer versions to inject for workloads that match the target. The key is the tracer
	// name and the value is the version to inject. Full config key:
	// apm_config.instrumentation.targets[].ddTraceVersions
	TracerVersions map[string]string `mapstructure:"ddTraceVersions"`
	// TracerConfigs is a list of configuration options to use for the installed tracers. These options will be added
	// as environment variables in addition to the injected tracer. Full config key:
	// apm_config.instrumentation.targets[].ddTraceConfigs
	TracerConfigs []TracerConfig `mapstructure:"ddTraceConfigs" json:"ddTraceConfigs"`
}

// PodSelector is a reconstruction of the metav1.LabelSelector struct to be able to unmarshal the configuration. It
// can be converted to a metav1.LabelSelector using the AsLabelSelector method. Full config key:
// apm_config.instrumentation.targets[].selector
type PodSelector struct {
	// MatchLabels is a map of key-value pairs to match the labels of the pod. The labels and expressions are ANDed.
	// Full config key: apm_config.instrumentation.targets[].podSelector.matchLabels
	MatchLabels map[string]string `mapstructure:"matchLabels"`
	// MatchExpressions is a list of label selector requirements to match the labels of the pod. The labels and
	// expressions are ANDed. Full config key: apm_config.instrumentation.targets[].podSelector.matchExpressions
	MatchExpressions []SelectorMatchExpression `mapstructure:"matchExpressions"`
}

// AsLabelSelector converts the PodSelector to a labels.Selector. It returns an error if the conversion fails.
func (p PodSelector) AsLabelSelector() (labels.Selector, error) {
	labelSelector := &metav1.LabelSelector{
		MatchLabels:      p.MatchLabels,
		MatchExpressions: make([]metav1.LabelSelectorRequirement, len(p.MatchExpressions)),
	}
	for i, expr := range p.MatchExpressions {
		labelSelector.MatchExpressions[i] = metav1.LabelSelectorRequirement{
			Key:      expr.Key,
			Operator: expr.Operator,
			Values:   expr.Values,
		}
	}

	return metav1.LabelSelectorAsSelector(labelSelector)
}

// SelectorMatchExpression is a reconstruction of the metav1.LabelSelectorRequirement struct to be able to unmarshal
// the configuration.
type SelectorMatchExpression struct {
	// Key is the key of the label to match.
	Key string `mapstructure:"key"`
	// Operator is the operator to use to match the label. Valid values are In, NotIn, Exists, DoesNotExist.
	Operator metav1.LabelSelectorOperator `mapstructure:"operator"`
	// Values is a list of values to match the label against. If the operator is Exists or DoesNotExist, the values
	// should be empty. If the operator is In or NotIn, the values should be non-empty.
	Values []string `mapstructure:"values"`
}

// NamespaceSelector is a struct to store the configuration for the namespace selector. It can be used to match the
// namespaces to apply the auto instrumentation to. Full config key:
// apm_config.instrumentation.targets[].namespaceSelector
type NamespaceSelector struct {
	// MatchNames is a list of namespace names to match. If empty, all namespaces are matched. Full config key:
	// apm_config.instrumentation.targets[].namespaceSelector.matchNames
	MatchNames []string `mapstructure:"matchNames"`
	// MatchLabels is a map of key-value pairs to match the labels of the namespace. The labels and expressions are
	// ANDed. This cannot be used with MatchNames. Full config key:
	// apm_config.instrumentation.targets[].namespaceSelector.matchLabels
	MatchLabels map[string]string `mapstructure:"matchLabels"`
	// MatchExpressions is a list of label selector requirements to match the labels of the namespace. The labels and
	// expressions are ANDed. This cannot be used with MatchNames. Full config key:
	// apm_config.instrumentation.targets[].selector.matchExpressions
	MatchExpressions []SelectorMatchExpression `mapstructure:"matchExpressions"`
}

// AsLabelSelector converts the NamespaceSelector to a labels.Selector. It returns an error if the conversion fails.
func (n NamespaceSelector) AsLabelSelector() (labels.Selector, error) {
	labelSelector := &metav1.LabelSelector{
		MatchLabels:      n.MatchLabels,
		MatchExpressions: make([]metav1.LabelSelectorRequirement, len(n.MatchExpressions)),
	}
	for i, expr := range n.MatchExpressions {
		labelSelector.MatchExpressions[i] = metav1.LabelSelectorRequirement{
			Key:      expr.Key,
			Operator: expr.Operator,
			Values:   expr.Values,
		}
	}

	return metav1.LabelSelectorAsSelector(labelSelector)
}

// TracerConfig is a struct that stores configuration options for a tracer. These will be injected as environment
// variables to the workload that matches targeting.
type TracerConfig struct {
	// Name is the name of the environment variable.
	Name string `mapstructure:"name" json:"name"`
	// Value is the value to use.
	Value string `mapstructure:"value" json:"value"`
}

// AsEnvVar converts the TracerConfig to a corev1.EnvVar.
func (c *TracerConfig) AsEnvVar() corev1.EnvVar {
	return corev1.EnvVar{
		Name:  c.Name,
		Value: c.Value,
	}
}

var (
	minimumCPULimit    resource.Quantity = resource.MustParse("0.05")  // 0.05 core, otherwise copying + library initialization is going to take forever
	minimumMemoryLimit resource.Quantity = resource.MustParse("100Mi") // 100 MB (recommended minimum by Alpine)
)

type initResourceRequirementConfiguration map[corev1.ResourceName]resource.Quantity

// getOptionalBoolValue returns a pointer to a bool corresponding to the config value if the key is set in the config
func getOptionalBoolValue(datadogConfig config.Component, key string) *bool {
	var value *bool
	if datadogConfig.IsSet(key) {
		tmp := datadogConfig.GetBool(key)
		value = &tmp
	}

	return value
}

// getOptionalBoolValue returns a pointer to a bool corresponding to the config value if the key is set in the config
func getOptionalStringValue(datadogConfig config.Component, key string) *string {
	var value *string
	if datadogConfig.IsSet(key) {
		tmp := datadogConfig.GetString(key)
		value = &tmp
	}

	return value
}

// getPinnedLibraries returns tracing libraries to inject as configured by apm_config.instrumentation.lib_versions
// given a registry.
func getPinnedLibraries(libVersions map[string]string, registry string) []libInfo {
	var res []libInfo
	for lang, version := range libVersions {
		l := language(lang)
		if !l.isSupported() {
			log.Warnf("APM Instrumentation detected configuration for unsupported language: %s. Tracing library for %s will not be injected", lang, lang)
			continue
		}

		log.Infof("Library version %s is specified for language %s", version, lang)
		res = append(res, l.libInfo("", l.libImageName(registry, version)))
	}

	return res
}

func initDefaultResources(datadogConfig config.Component) (initResourceRequirementConfiguration, error) {

	var conf = initResourceRequirementConfiguration{}

	if datadogConfig.IsSet("admission_controller.auto_instrumentation.init_resources.cpu") {
		quantity, err := resource.ParseQuantity(datadogConfig.GetString("admission_controller.auto_instrumentation.init_resources.cpu"))
		if err != nil {
			return conf, err
		}
		conf[corev1.ResourceCPU] = quantity
	} /* else {
		conf[corev1.ResourceCPU] = *resource.NewMilliQuantity(minimumCPULimit, resource.DecimalSI)
	}*/

	if datadogConfig.IsSet("admission_controller.auto_instrumentation.init_resources.memory") {
		quantity, err := resource.ParseQuantity(datadogConfig.GetString("admission_controller.auto_instrumentation.init_resources.memory"))
		if err != nil {
			return conf, err
		}
		conf[corev1.ResourceMemory] = quantity
	} /*else {
		conf[corev1.ResourceCPU] = *resource.NewMilliQuantity(minimumMemoryLimit, resource.DecimalSI)
	}*/

	return conf, nil
}

func parseInitSecurityContext(datadogConfig config.Component) (*corev1.SecurityContext, error) {
	securityContext := corev1.SecurityContext{}
	confKey := "admission_controller.auto_instrumentation.init_security_context"

	if datadogConfig.IsSet(confKey) {
		confValue := datadogConfig.GetString(confKey)
		err := json.Unmarshal([]byte(confValue), &securityContext)
		if err != nil {
			return nil, fmt.Errorf("failed to get init security context from configuration, %s=`%s`: %v", confKey, confValue, err)
		}
	}

	return &securityContext, nil
}
