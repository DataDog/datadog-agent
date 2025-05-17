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

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/apm/instrumentation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
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
	Instrumentation *instrumentation.Config

	// containerRegistry is the container registry to use for the autoinstrumentation logic
	containerRegistry string

	// precomputed containerMutators for the security and profiling products
	securityClientLibraryMutator  containerMutator
	profilingClientLibraryMutator containerMutator

	// containerFilter is a pre-computed filter function
	// to be passed to the different mutators that dictates
	// whether or not given pod containers should be mutated by
	// this webhook.
	//
	// At the moment it is based on [[excludedContainerNames]].
	// See [[excludedContainerNamesContainerFilter]].
	//
	// This should be used in conjunction with containerMutators
	// via [[filteredContainerMutator]] and [[mutatePodContainers]]
	// to make sure we aren't applying mutations to containers we
	// should be skipping.
	containerFilter containerFilter

	// initResources is the resource requirements for the init containers.
	initResources initResourceRequirementConfiguration

	// initSecurityContext is the security context for the init containers.
	initSecurityContext *corev1.SecurityContext

	// defaultResourceRequirements is the default resource requirements
	// for the init containers.
	defaultResourceRequirements initResourceRequirementConfiguration

	// version is the version of the autoinstrumentation logic to use.
	// We don't expose this option to the user, and [[instrumentationV1]]
	// is deprecated and slated for removal.
	version version
}

var excludedContainerNames = map[string]bool{
	"istio-proxy": true, // https://datadoghq.atlassian.net/browse/INPLAT-454
}

func excludedContainerNamesContainerFilter(c *corev1.Container) bool {
	_, exclude := excludedContainerNames[c.Name]
	return !exclude
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
		Webhook:                       NewWebhookConfig(datadogConfig),
		LanguageDetection:             NewLanguageDetectionConfig(datadogConfig),
		Instrumentation:               instrumentationConfig,
		containerRegistry:             containerRegistry,
		initResources:                 initResources,
		initSecurityContext:           initSecurityContext,
		defaultResourceRequirements:   defaultResourceRequirements,
		securityClientLibraryMutator:  securityClientLibraryConfigMutators(datadogConfig),
		profilingClientLibraryMutator: profilingClientLibraryConfigMutators(datadogConfig),
		containerFilter:               excludedContainerNamesContainerFilter,
		version:                       version,
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

// NewInstrumentationConfig creates a new InstrumentationConfig from the datadog config. It returns an error if the
// configuration is invalid.
func NewInstrumentationConfig(datadogConfig config.Component) (*instrumentation.Config, error) {
	cfg := &instrumentation.Config{}
	err := structure.UnmarshalKey(datadogConfig, "apm_config.instrumentation", cfg, structure.ErrorUnused)
	if err != nil {
		return nil, fmt.Errorf("unable to parse apm_config.instrumentation: %w", err)
	}

	// Ensure both enabled and disabled namespaces are not set together.
	if len(cfg.EnabledNamespaces) > 0 && len(cfg.DisabledNamespaces) > 0 {
		return nil, fmt.Errorf("apm_config.instrumentation.enabled_namespaces and apm_config.instrumentation.disabled_namespaces are mutually exclusive and cannot be set together")
	}

	// Ensure both enabled namespaces and targets are not set together.
	if len(cfg.EnabledNamespaces) > 0 && len(cfg.Targets) > 0 {
		return nil, fmt.Errorf("apm_config.instrumentation.enabled_namespaces and apm_config.instrumentation.targets are mutually exclusive and cannot be set together")
	}

	// Ensure both library versions and targets are not set together.
	if len(cfg.LibVersions) > 0 && len(cfg.Targets) > 0 {
		return nil, fmt.Errorf("apm_config.instrumentation.lib_versions and apm_config.instrumentation.targets are mutually exclusive and cannot be set together")
	}

	// Ensure both namespace names and labels are not set together.
	for _, target := range cfg.Targets {
		if target.NamespaceSelector != nil && len(target.NamespaceSelector.MatchNames) > 0 && (len(target.NamespaceSelector.MatchLabels) > 0 || len(target.NamespaceSelector.MatchExpressions) > 0) {
			return nil, fmt.Errorf("apm_config.instrumentation.targets[].namespaceSelector.matchNames and apm_config.instrumentation.targets[].namespaceSelector.matchLabels/matchExpressions are mutually exclusive and cannot be set together")
		}
	}

	return cfg, nil
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

type pinnedLibraries struct {
	// libs are the pinned libraries themselves.
	libs []libInfo
	// areSetToDefaults is true when the libs coming from the configuration
	// are equivalent to what would be set if there was no configuration at all.
	areSetToDefaults bool
}

// getPinnedLibraries returns tracing libraries to inject as configured by apm_config.instrumentation.lib_versions
// given a registry.
func getPinnedLibraries(libVersions map[string]string, registry string, checkDefaults bool) pinnedLibraries {
	libs := []libInfo{}
	allDefaults := true

	for lang, version := range libVersions {
		l := language(lang)
		if !l.isSupported() {
			log.Warnf("APM Instrumentation detected configuration for unsupported language: %s. Tracing library for %s will not be injected", lang, lang)
			continue
		}

		info := l.libInfo("", l.libImageName(registry, version))
		log.Infof("Library version %s is specified for language %s, going to use %s", version, lang, info.image)
		libs = append(libs, info)

		if info.image != l.libImageName(registry, l.defaultLibVersion()) {
			allDefaults = false
		}
	}

	return pinnedLibraries{
		libs:             libs,
		areSetToDefaults: checkDefaults && allDefaults && len(libs) == len(defaultSupportedLanguagesMap()),
	}
}

func initDefaultResources(datadogConfig config.Component) (initResourceRequirementConfiguration, error) {
	conf := initResourceRequirementConfiguration{}

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
