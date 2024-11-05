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
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	minimumCPULimit    int64 = 50                // 0.05 core, otherwise copying + library initialization is going to take forever
	minimumMemoryLimit int64 = 100 * 1024 * 1024 // 100 MB (recommended minimum by Alpine)
)

// webhookConfig use to store options from the config.Component for the autoinstrumentation webhook
type webhookConfig struct {
	// isEnabled is the flag to enable the autoinstrumentation webhook
	isEnabled bool
	endpoint  string
	version   version // autoinstrumentation logic version

	// optional features
	languageDetectionEnabled          bool
	languageDetectionReportingEnabled bool
	injectAutoDetectedLibraries       bool
	// keep pointers to bool to differentiate between unset and false
	// for backward compatibility with the previous implementation.
	// TODO: remove the pointers when the backward compatibility is not needed anymore.
	asmEnabled       *bool
	iastEnabled      *bool
	asmScaEnabled    *bool
	profilingEnabled *string

	// configuration for the libraries init-containers to inject.
	containerRegistry           string
	injectorImageTag            string
	injectionFilter             mutatecommon.InjectionFilter
	pinnedLibraries             []libInfo
	initSecurityContext         *corev1.SecurityContext
	defaultResourceRequirements initResourceRequirementConfiguration
}

type initResourceRequirementConfiguration map[corev1.ResourceName]resource.Quantity

// retrieveConfig retrieves the configuration for the autoinstrumentation webhook from the datadog config
func retrieveConfig(datadogConfig config.Component, injectionFilter mutatecommon.InjectionFilter) (webhookConfig, error) {

	webhookConfig := webhookConfig{
		isEnabled: datadogConfig.GetBool("admission_controller.auto_instrumentation.enabled"),
		endpoint:  datadogConfig.GetString("admission_controller.auto_instrumentation.endpoint"),

		languageDetectionEnabled:          datadogConfig.GetBool("language_detection.enabled"),
		languageDetectionReportingEnabled: datadogConfig.GetBool("language_detection.reporting.enabled"),
		injectAutoDetectedLibraries:       datadogConfig.GetBool("admission_controller.auto_instrumentation.inject_auto_detected_libraries"),

		asmEnabled:       getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm.enabled"),
		iastEnabled:      getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.iast.enabled"),
		asmScaEnabled:    getOptionalBoolValue(datadogConfig, "admission_controller.auto_instrumentation.asm_sca.enabled"),
		profilingEnabled: getOptionalStringValue(datadogConfig, "admission_controller.auto_instrumentation.profiling.enabled"),

		containerRegistry: mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.auto_instrumentation.container_registry"),
		injectorImageTag:  datadogConfig.GetString("apm_config.instrumentation.injector_image_tag"),
		injectionFilter:   injectionFilter,
	}
	webhookConfig.pinnedLibraries = getPinnedLibraries(datadogConfig, webhookConfig.containerRegistry)

	var err error
	if webhookConfig.version, err = instrumentationVersion(datadogConfig.GetString("apm_config.instrumentation.version")); err != nil {
		return webhookConfig, fmt.Errorf("invalid version for key apm_config.instrumentation.version: %w", err)
	}

	webhookConfig.initSecurityContext, err = parseInitSecurityContext(datadogConfig)
	if err != nil {
		return webhookConfig, fmt.Errorf("unable to parse init-container's SecurityContext from configuration: %w", err)
	}

	webhookConfig.defaultResourceRequirements, err = initDefaultResources(datadogConfig)
	if err != nil {
		return webhookConfig, fmt.Errorf("unable to parse init-container's resources from configuration: %w", err)
	}

	return webhookConfig, nil
}

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
func getPinnedLibraries(datadogConfig config.Component, registry string) []libInfo {
	// If APM Instrumentation is enabled and configuration apm_config.instrumentation.lib_versions specified,
	// inject only the libraries from the configuration
	singleStepLibraryVersions := datadogConfig.GetStringMapString("apm_config.instrumentation.lib_versions")

	var res []libInfo
	for lang, version := range singleStepLibraryVersions {
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
