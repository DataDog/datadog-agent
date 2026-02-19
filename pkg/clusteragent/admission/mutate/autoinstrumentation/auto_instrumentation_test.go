// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	admissioncommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
)

const (
	defaultInjectorVersion = "0"
	defaultTestContainer   = "test-container"
	defaultNamespace       = "application"
)

var defaultContainerNames = []string{defaultTestContainer}

var defaultLibraries = map[string]string{
	"dotnet": "v3",
	"java":   "v1",
	"js":     "v5",
	"php":    "v1",
	"python": "v4",
	"ruby":   "v2",
}

var defaultDeployments = []common.MockDeployment{
	{
		ContainerName:  defaultTestContainer,
		DeploymentName: "deployment",
		Namespace:      defaultNamespace,
	},
}

var defaultNamespaces = []workloadmeta.KubernetesMetadata{
	newTestNamespace(defaultNamespace, map[string]string{}),
}

// TestAutoinstrumentation is an integration style test that ensures user configuration maps to the expected pod
// mutation.
func TestAutoinstrumentation(t *testing.T) {
	// expected is a struct that must be defined if should mutate is true. If injection is expected, one of the two
	// options should be set (libraryVersions + injectorVersion) OR (initContainerImages).
	type expected struct {
		// injectorVersion (required) ensures the version of the injector matches the expected version.
		injectorVersion string
		// libraryVersions (required) ensures the versions of libraries expected exist in the pod.
		libraryVersions map[string]string
		// containerNames (required) is the list of containers where injection is expected.
		containerNames []string
		// initContainerImages (optional) ensures that the list of init container images exist in the pod rather then
		// relying on injectorVersion and libraryVersions. This is useful when you also need to test the registry or
		// custom image.
		initContainerImages []string
		// requiredEnvs (optional) ensures that the additional environment variables exist on every container.
		requiredEnvs map[string]string
		// unsetEnvs (optional) ensures that the environment variables do not exist on any container.
		unsetEnvs []string
		// initSecurityContext (optional) ensures that the init containers contain the additional security context.
		initSecurityContext *corev1.SecurityContext
		// initResourceRequirements (optional) ensures that the init containers contain the proper resource requirements.
		initResourceRequirements *corev1.ResourceRequirements
		// unmutatedContainers (optional) ensures that specific containers have NO Datadog-related env vars (DD_*, LD_PRELOAD).
		// This is useful to verify containers like istio-proxy are completely excluded from mutation.
		unmutatedContainers []string
		// expectedAnnotations (optional) ensures that the pod has the expected annotations.
		expectedAnnotations map[string]string
	}

	tests := map[string]struct {
		config       map[string]any
		pod          *corev1.Pod
		namespaces   []workloadmeta.KubernetesMetadata
		deployments  []common.MockDeployment
		shouldMutate bool
		expected     *expected
	}{
		"default configuration should not mutate": {
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with annotation but without mutate label and mutate unlabelled disabled should not mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"local lib injection should set install type": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_INSTRUMENTATION_INSTALL_TYPE": "k8s_lib_injection",
				},
			},
		},
		"local sdk injection with enabled label set to false in enabled namespace should not get injection": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     true,
				"admission_controller.mutate_unlabelled": false,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "false",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with mutate label and java annotation should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with mutate label and python annotation should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/python-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"python": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with mutate label and js annotation should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/js-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"js": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with mutate label and php annotation should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/php-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"php": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with mutate label and ruby annotation should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/ruby-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with mutate label and dotnet annotation should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/dotnet-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"dotnet": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local sdk injection debug enabled should get debug info": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
					"admission.datadoghq.com/apm-inject.debug": "true",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				requiredEnvs: map[string]string{
					"DD_APM_INSTRUMENTATION_DEBUG": "true",
					"DD_TRACE_STARTUP_LOGS":        "true",
					"DD_TRACE_DEBUG":               "true",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local sdk injection debug disabled should not get debug info": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
					"admission.datadoghq.com/apm-inject.debug": "false",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				unsetEnvs: []string{
					"DD_APM_INSTRUMENTATION_DEBUG",
					"DD_TRACE_STARTUP_LOGS",
					"DD_TRACE_DEBUG",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local sdk injection debug enabled but no library should not get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/apm-inject.debug": "true",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with mutate label disabled and annotation should not mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "false",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with mutate label disabled with mutate unlabelled and annotation should not mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "false",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with mutate label but no annotation and instrumentation disabled should not mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with mutate label but no annotation and instrumentation enabled should get default libs": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     true,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod without mutate label and mutate unlabelled enabled should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with annotation should take precedence over config": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.lib_versions": map[string]string{
					"java": "v2",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with instrumentation enabled should get all default libs": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod with instrumentation enabled and lib versions defined should get defined versions": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.lib_versions": map[string]string{
					"python": "v5",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"python": "v5",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with instrumentation enabled and lib versions defined with a magic tag should get resolved": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.lib_versions": map[string]string{
					"java": "default",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod outside of enabled namespace should not get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"foo",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod inside of enabled namespace should get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod outside of enabled namespace but with local lib injection should get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"foo",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod inside of disabled namespace should not get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.disabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod outside of disabled namespace should get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.disabled_namespaces": []string{
					"foo",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod inside of disabled namespace but with local lib injection should not get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.disabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod inside of disabled namespace but with local lib injection and instrumentation disabled should not get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": false,
				"apm_config.instrumentation.disabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with local lib injection and default tag should resolve to version": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "default",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local lib injection that defines two libs should get both": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
					"admission.datadoghq.com/js-lib.version":   "v3",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
					"js":   "v3",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local lib injection that sets injector version should get it": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "v1",
					"admission.datadoghq.com/apm-inject.version": "v3",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: "v3",
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local lib injection that defines an unknown version should not mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/unknown-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with local lib injection using all annotation gets all libs": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version": "latest",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod with local lib injection using all annotation but no label does not get mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version": "latest",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"pod with local lib injection using all annotation, no label, and mutated unlabelled gets mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version": "latest",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod with local lib injection using pinned versions gets mutated": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1.2.3",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1.2.3",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local lib injection using java and all annotations only gets the java image": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
					"admission.datadoghq.com/all-lib.version":  "latest",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with local lib injection using all annotations with unsupported tag still gets all versions": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version": "unsupported",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"pod with local lib injection and custom injector gets custom version": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":            false,
				"admission_controller.mutate_unlabelled":        false,
				"apm_config.instrumentation.injector_image_tag": "1.2.3",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/php-lib.version": "5.2.1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: "1.2.3",
				libraryVersions: map[string]string{
					"php": "5.2.1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"pod with enabled namespaces and custom injector gets custom version": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":            true,
				"apm_config.instrumentation.injector_image_tag": "1.2.3",
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: "1.2.3",
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"targets with matching rule mutates pod": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						TracerVersions: map[string]string{
							"ruby": "v2",
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v2",
				},
				containerNames: defaultContainerNames,
			},
		},
		"targets with matching rule sets install type": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						TracerVersions: map[string]string{
							"ruby": "v2",
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v2",
				},
				containerNames: defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_INSTRUMENTATION_INSTALL_TYPE": "k8s_single_step",
				},
			},
		},
		"targets with matching rule and additional config both apply to pod": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						TracerVersions: map[string]string{
							"ruby": "v2",
						},
						TracerConfigs: []autoinstrumentation.TracerConfig{
							{
								Name:  "DD_PROFILING_ENABLED",
								Value: "true",
							},
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v2",
				},
				requiredEnvs: map[string]string{
					"DD_PROFILING_ENABLED": "true",
				},
				containerNames: defaultContainerNames,
			},
		},
		"targets without matching rule does not mutate pod": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"foo",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"targets with matching rule and local sdk injection favors local sdk version": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						TracerVersions: map[string]string{
							"ruby": "v2",
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/ruby-lib.version": "v3",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v3",
				},
				containerNames: defaultContainerNames,
			},
		},
		"targets with matching rule and local sdk injection but no label favors target": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						TracerVersions: map[string]string{
							"ruby": "v2",
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/ruby-lib.version": "v3",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v2",
				},
				containerNames: defaultContainerNames,
			},
		},
		"targets with matching rule and local sdk injection with no label but with mutate unlabelled favors local sdk": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     true,
				"admission_controller.mutate_unlabelled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "test-target",
						TracerVersions: map[string]string{
							"ruby": "v2",
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/ruby-lib.version": "v3",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"ruby": "v3",
				},
				containerNames: defaultContainerNames,
			},
		},
		"local sdk injection with custom injector image gets custom image": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/php-lib.version":         "v1",
					"admission.datadoghq.com/apm-inject.custom-image": "docker.io/library/apm-inject-package:v27",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				initContainerImages: []string{
					"gcr.io/datadoghq/dd-lib-php-init:v1",
					"docker.io/library/apm-inject-package:v27",
				},
				containerNames: defaultContainerNames,
			},
		},
		"local sdk injection with custom library image gets custom image": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/php-lib.custom-image": "foo/bar:1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				initContainerImages: []string{
					"gcr.io/datadoghq/apm-inject:0",
					"foo/bar:1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"local sdk injection with configs get configs set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "v1",
					"admission.datadoghq.com/java-lib.config.v1": `{"runtime_metrics_enabled":true,"tracing_rate_limit":50}`,
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				requiredEnvs: map[string]string{
					"DD_RUNTIME_METRICS_ENABLED": "true",
					"DD_TRACE_RATE_LIMIT":        "50",
				},
				containerNames: defaultContainerNames,
			},
		},
		"instrumentation enabled and no mutate label with config annotation gets configs set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.lib_versions": map[string]string{
					"java": "v2",
				},
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.config.v1": `{"runtime_metrics_enabled":true,"tracing_rate_limit":50}`,
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v2",
				},
				requiredEnvs: map[string]string{
					"DD_RUNTIME_METRICS_ENABLED": "true",
					"DD_TRACE_RATE_LIMIT":        "50",
				},
				containerNames: defaultContainerNames,
			},
		},
		"local sdk injection with all lib get configs set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version":   "latest",
					"admission.datadoghq.com/all-lib.config.v1": `{"runtime_metrics_enabled":true,"tracing_rate_limit":50,"tracing_sampling_rate":0.3}`,
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				requiredEnvs: map[string]string{
					"DD_RUNTIME_METRICS_ENABLED": "true",
					"DD_TRACE_RATE_LIMIT":        "50",
					"DD_TRACE_SAMPLE_RATE":       "0.30",
				},
				containerNames: defaultContainerNames,
			},
		},
		"istio-proxy is not injected": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"kubernetes_pod_labels_as_tags": map[string]string{
					"app-version": "version",
					"environment": "env",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					"app-version": "v1.2.3",
					"environment": "production",
				},
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
					},
					{
						Name: "istio-proxy",
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames: []string{
					defaultTestContainer,
				},
				// Verify istio-proxy has NO Datadog env vars at all (DD_*, LD_PRELOAD)
				unmutatedContainers: []string{"istio-proxy"},
			},
		},
		"injection does not occur in the namespace where datadog is deployed": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"kube_resources_namespace":           "datadog-test",
			},
			pod: common.FakePodSpec{
				Name:       "test",
				NS:         "datadog-test",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []common.MockDeployment{
				{
					ContainerName:  defaultTestContainer,
					DeploymentName: "deployment",
					Namespace:      "datadog-test",
				},
			},
			shouldMutate: false,
		},
		"injection does occur in the outside the datadog namespace": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"kube_resources_namespace":           "datadog-test",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
			},
		},
		"language detection enabled limits the injected libraries": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                                       true,
				"language_detection.enabled":                                               true,
				"language_detection.reporting.enabled":                                     true,
				"admission_controller.auto_instrumentation.inject_auto_detected_libraries": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []common.MockDeployment{
				{
					ContainerName:  defaultTestContainer,
					DeploymentName: "deployment",
					Namespace:      "application",
					Languages:      languageSetOf("python"),
				},
			},
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"python": defaultLibraries["python"],
				},
				containerNames: defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_INSTRUMENTATION_LANGUAGES_DETECTED":                   "python",
					"DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED": "true",
				},
			},
		},
		"language detection disabled sets disabled env var": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                                       true,
				"language_detection.enabled":                                               true,
				"language_detection.reporting.enabled":                                     true,
				"admission_controller.auto_instrumentation.inject_auto_detected_libraries": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []common.MockDeployment{
				{
					ContainerName:  defaultTestContainer,
					DeploymentName: "deployment",
					Namespace:      "application",
					Languages:      languageSetOf("python"),
				},
			},
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED": "false",
				},
			},
		},
		"language detection enabled but no language found does not limit libraries": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                                       true,
				"language_detection.enabled":                                               true,
				"language_detection.reporting.enabled":                                     true,
				"admission_controller.auto_instrumentation.inject_auto_detected_libraries": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []common.MockDeployment{
				{
					ContainerName:  defaultTestContainer,
					DeploymentName: "deployment",
					Namespace:      "application",
					Languages:      languageSetOf(),
				},
			},
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_INSTRUMENTATION_LANGUAGES_DETECTED":                   "",
					"DD_INSTRUMENTATION_LANGUAGE_DETECTION_INJECTION_ENABLED": "true",
				},
			},
		},
		"local lib injection per container should mutate": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/test-container.java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"when a security context is set through config, it is applied": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                              true,
				"admission_controller.auto_instrumentation.init_security_context": `{"privileged":true}`,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				initSecurityContext: &corev1.SecurityContext{
					Privileged: ptr.To(true),
				},
			},
		},
		"when a pod is created in a privileged namespace that is not enabled, no mutation occurs": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"foo",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: defaultDeployments,
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", map[string]string{
					"pod-security.kubernetes.io/enforce":         "restricted",
					"pod-security.kubernetes.io/enforce-version": "latest",
					"pod-security.kubernetes.io/warn":            "restricted",
					"pod-security.kubernetes.io/audit":           "restricted",
				}),
			},
			shouldMutate: false,
		},
		"when a pod is created in a privileged namespace, the default security context is set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: defaultDeployments,
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", map[string]string{
					"pod-security.kubernetes.io/enforce":         "restricted",
					"pod-security.kubernetes.io/enforce-version": "latest",
					"pod-security.kubernetes.io/warn":            "restricted",
					"pod-security.kubernetes.io/audit":           "restricted",
				}),
			},
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				initSecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To(false),
					RunAsNonRoot:             ptr.To(true),
					SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{
							"ALL",
						},
					},
				},
			},
		},
		"a pod with resources has init container resources set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				initResourceRequirements: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("500Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("500Mi"),
					},
				},
			},
		},
		"a pod with only cpu resources has only cpu init container resources set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							},
						},
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				initResourceRequirements: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				},
			},
		},
		"a pod with only memory resources has only memory init container resources set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				initResourceRequirements: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Mi"),
					},
				},
			},
		},
		"a pod with init container resources has the highest value init container resources set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				InitContainers: []corev1.Container{
					{
						Name: "foo",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("700m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("700m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
						},
					},
					{
						Name: "bar",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("101m"),
								corev1.ResourceMemory: resource.MustParse("700Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("101m"),
								corev1.ResourceMemory: resource.MustParse("700Mi"),
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("102m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("102m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				initResourceRequirements: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("700m"),
						corev1.ResourceMemory: resource.MustParse("700Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("700m"),
						corev1.ResourceMemory: resource.MustParse("700Mi"),
					},
				},
			},
		},
		"a pod with multiple container resources is additive": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				InitContainers: []corev1.Container{
					{
						Name: "foo",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
					},
					{
						Name: "sidecar",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames: []string{
					defaultTestContainer,
					"sidecar",
				},
				initResourceRequirements: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("400m"),
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("400m"),
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
				},
			},
		},
		"when config is set, it takes precedence for resources": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                              true,
				"admission_controller.auto_instrumentation.init_resources.cpu":    "101m",
				"admission_controller.auto_instrumentation.init_resources.memory": "301Mi",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				InitContainers: []corev1.Container{
					{
						Name: "foo",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
					},
					{
						Name: "sidecar",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames: []string{
					defaultTestContainer,
					"sidecar",
				},
				initResourceRequirements: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("101m"),
						corev1.ResourceMemory: resource.MustParse("301Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("101m"),
						corev1.ResourceMemory: resource.MustParse("301Mi"),
					},
				},
			},
		},
		"tags from labels webhook applies to enabled namespace": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					"tags.datadoghq.com/env": "local",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_ENV": "local",
				},
			},
		},
		"tags from labels webhook does not apply when pod is created outside enabled namespaces": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"foo",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					"tags.datadoghq.com/env": "local",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		"UST env vars from pod_labels_as_tags injects DD_VERSION and DD_ENV": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
				"kubernetes_pod_labels_as_tags": map[string]string{
					"app-version": "version",
					"environment": "env",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					"app-version": "v1.2.3",
					"environment": "production",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				// DD_VERSION and DD_ENV are injected with ValueFrom.FieldRef, so Value is empty
				requiredEnvs: map[string]string{
					"DD_VERSION": "",
					"DD_ENV":     "",
				},
			},
		},
		"UST env vars from pod_labels_as_tags does not inject when namespace is not eligible": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"other-namespace",
				},
				"kubernetes_pod_labels_as_tags": map[string]string{
					"app-version": "version",
					"environment": "env",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Labels: map[string]string{
					"app-version":                   "v1.2.3",
					"environment":                   "production",
					admissioncommon.EnabledLabelKey: "true",
				},
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
			}.Create(),
			deployments: defaultDeployments,
			namespaces:  defaultNamespaces,
			// Pod is mutated via local lib injection, but UST env vars should NOT be injected
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
				// DD_VERSION and DD_ENV should NOT be present since namespace is not eligible
				unsetEnvs: []string{"DD_VERSION", "DD_ENV"},
			},
		},
		"lib config from annotations injects config for python language": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/python-lib.version":   "v1",
					"admission.datadoghq.com/python-lib.config.v1": `{"runtime_metrics_enabled":true,"tracing_sampling_rate":0.5}`,
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"python": "v1",
				},
				containerNames: defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_RUNTIME_METRICS_ENABLED": "true",
					"DD_TRACE_SAMPLE_RATE":       "0.50",
				},
			},
		},
		"lib config from annotations injects config for js language": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/js-lib.version":   "v1",
					"admission.datadoghq.com/js-lib.config.v1": `{"tracing_debug":true,"log_injection_enabled":true}`,
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"js": "v1",
				},
				containerNames: defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_TRACE_DEBUG":    "true",
					"DD_LOGS_INJECTION": "true",
				},
			},
		},
		"config webhook applies to enabled namespace": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
				"admission_controller.inject_config.mode": "hostip",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_AGENT_HOST": "",
				},
			},
		},
		"config webhook does not apply outside of enabled namespace": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"foo",
				},
				"admission_controller.inject_config.mode": "hostip",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		// Image configuration tests
		"custom container registry via config is used for init containers": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                           true,
				"admission_controller.auto_instrumentation.container_registry": "my-registry.example.com/datadog",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1.2.3",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				initContainerImages: []string{
					"my-registry.example.com/datadog/dd-lib-java-init:v1.2.3",
					"my-registry.example.com/datadog/apm-inject:0",
				},
				containerNames: defaultContainerNames,
			},
		},
		"custom injector version via annotation overrides config": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":            true,
				"apm_config.instrumentation.injector_image_tag": "1.0.0",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "v1",
					"admission.datadoghq.com/apm-inject.version": "2.0.0",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: "2.0.0",
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
			},
		},
		"multiple libraries with different versions are all injected": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "v1.5.0",
					"admission.datadoghq.com/python-lib.version": "v2.3.0",
					"admission.datadoghq.com/js-lib.version":     "v4.1.0",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java":   "v1.5.0",
					"python": "v2.3.0",
					"js":     "v4.1.0",
				},
				containerNames: defaultContainerNames,
			},
		},
		// Target annotation tests
		"target with matching rule sets applied-target annotation": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.targets": []autoinstrumentation.Target{
					{
						Name: "Python Apps",
						TracerVersions: map[string]string{
							"python": "v3",
						},
						NamespaceSelector: &autoinstrumentation.NamespaceSelector{
							MatchNames: []string{
								"application",
							},
						},
					},
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"python": "v3",
				},
				containerNames: defaultContainerNames,
				expectedAnnotations: map[string]string{
					"internal.apm.datadoghq.com/applied-target": `{"name":"Python Apps","namespaceSelector":{"matchNames":["application"]},"ddTraceVersions":{"python":"v3"}}`,
				},
			},
		},
		// Debug mode tests via annotation with single step
		"single step with debug enabled via annotation": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
				"apm_config.instrumentation.enabled_namespaces": []string{
					"application",
				},
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/apm-inject.debug": "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries,
				containerNames:  defaultContainerNames,
				requiredEnvs: map[string]string{
					"DD_APM_INSTRUMENTATION_DEBUG": "true",
					"DD_TRACE_STARTUP_LOGS":        "true",
					"DD_TRACE_DEBUG":               "true",
				},
			},
		},
		// Verify LD_PRELOAD and DD_INJECT_SENDER_TYPE are set correctly
		"injection sets LD_PRELOAD and DD_INJECT_SENDER_TYPE environment variables": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
				requiredEnvs: map[string]string{
					"LD_PRELOAD":            "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
					"DD_INJECT_SENDER_TYPE": "k8s",
				},
			},
		},
		// All supported languages tests
		"all supported languages can be injected simultaneously through local SDK injection": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version":   "v1",
					"admission.datadoghq.com/python-lib.version": "v2",
					"admission.datadoghq.com/js-lib.version":     "v3",
					"admission.datadoghq.com/dotnet-lib.version": "v4",
					"admission.datadoghq.com/ruby-lib.version":   "v5",
					"admission.datadoghq.com/php-lib.version":    "v6",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java":   "v1",
					"python": "v2",
					"js":     "v3",
					"dotnet": "v4",
					"ruby":   "v5",
					"php":    "v6",
				},
				containerNames: defaultContainerNames,
			},
		},
		// Default versions resolution test
		"default magic string resolves to correct versions for all languages": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/all-lib.version": "default",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: defaultLibraries, // Should resolve to v1, v3, v4, v2, v5, v1
				containerNames:  defaultContainerNames,
			},
		},
		// Unsupported language test
		"unsupported language annotation does not cause mutation": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/go-lib.version": "v1", // Go is not supported
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: false,
		},
		// Autoscaler safe-to-evict annotation test
		"injection sets cluster-autoscaler safe-to-evict annotation": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":     false,
				"admission_controller.mutate_unlabelled": false,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Annotations: map[string]string{
					"admission.datadoghq.com/java-lib.version": "v1",
				},
				Labels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			}.Create(),
			deployments:  defaultDeployments,
			namespaces:   defaultNamespaces,
			shouldMutate: true,
			expected: &expected{
				injectorVersion: defaultInjectorVersion,
				libraryVersions: map[string]string{
					"java": "v1",
				},
				containerNames: defaultContainerNames,
				expectedAnnotations: map[string]string{
					"cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes": "datadog-auto-instrumentation,datadog-auto-instrumentation-etc",
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks.
			mockConfig := common.FakeConfigWithValues(t, test.config)
			mockMeta := common.FakeStoreWithDeployment(t, test.deployments)
			mockDynamic := fake.NewSimpleDynamicClient(runtime.NewScheme())
			// Disable gradual rollout for this test to use the NoOpResolver.
			mockConfig.SetInTest("admission_controller.auto_instrumentation.gradual_rollout.enabled", false)

			// Add the namespaces.
			for _, ns := range test.namespaces {
				mockMeta.(workloadmetamock.Mock).Set(&ns)
			}

			webhook, err := autoinstrumentation.NewAutoInstrumentation(mockConfig, mockMeta, nil)
			require.NoError(t, err)

			// Mutate pod.
			in := test.pod.DeepCopy()
			mutated, err := webhook.MutatePod(in, in.Namespace, mockDynamic)
			require.NoError(t, err)

			// If no mutation is expected, the pod should be identical and the boolean returned must be false.
			if !test.shouldMutate {
				require.Nil(t, test.expected, "the test was not properly configured, no mutation should not have expectations")
				require.Equal(t, test.pod, in, "the pod was mutated by the webhook when no mutation is expected")
				require.False(t, mutated, "mutate pod should only return true if the pod is mutated")
				return
			}

			// Require the test to be setup correctly and for mutation to have occurred.
			require.NotNil(t, test.expected, "the test was not properly configured, mutation should have expectations")
			require.NotEmpty(t, test.expected.containerNames, "the test was not properly configured, containerNames must be set")
			require.NotEqual(t, test.pod, in, "the pod was not mutated when it was expected to be")
			require.True(t, mutated, "the pod was mutated but the webhook returned false")

			// Require injection to have occurred.
			validator := testutils.NewPodValidator(in, testutils.InjectionModeAuto)
			validator.RequireInjection(t, test.expected.containerNames)

			// Require the libraries and versions to match.
			if len(test.expected.initContainerImages) > 0 {
				require.Empty(t, test.expected.libraryVersions, "the test was not properly configured, library versions should only be set without init container images")
				require.Empty(t, test.expected.injectorVersion, "the test was not properly configured, injector version should only be set without init container images")
				validator.RequireInitContainerImages(t, test.expected.initContainerImages)
			} else {
				validator.RequireInjectorVersion(t, test.expected.injectorVersion)
				validator.RequireLibraryVersions(t, test.expected.libraryVersions)
			}

			// Require environments to be set.
			validator.RequireEnvs(t, test.expected.requiredEnvs, test.expected.containerNames)
			validator.RequireMissingEnvs(t, test.expected.unsetEnvs, test.expected.containerNames)

			// Require specific containers have NO Datadog env vars (e.g., istio-proxy should be completely excluded).
			validator.RequireUnmutatedContainers(t, test.expected.unmutatedContainers)

			// Require security context to match expected.
			validator.RequireInitSecurityContext(t, test.expected.initSecurityContext)

			// Require resources match expected.
			validator.RequireInitResourceRequirements(t, test.expected.initResourceRequirements)

			// Require annotations match expected.
			validator.RequireAnnotations(t, test.expected.expectedAnnotations)
		})
	}
}

func TestEnvVarsAlreadySet(t *testing.T) {
	tests := map[string]struct {
		config             map[string]any
		pod                *corev1.Pod
		namespaces         []workloadmeta.KubernetesMetadata
		deployments        []common.MockDeployment
		expected           map[string]string
		expectedContainers []string
	}{
		"a pod without LD_PRELOAD has it set": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: defaultDeployments,
			namespaces:  defaultNamespaces,
			expected: map[string]string{
				"LD_PRELOAD": "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
			},
			expectedContainers: defaultContainerNames,
		},
		"a pod with LD_PRELOAD already set has it appended": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Env: []corev1.EnvVar{
							{
								Name:  "LD_PRELOAD",
								Value: "/foo",
							},
						},
					},
				},
			}.Create(),
			deployments: defaultDeployments,
			namespaces:  defaultNamespaces,
			expected: map[string]string{
				"LD_PRELOAD": "/foo:/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
			},
			expectedContainers: defaultContainerNames,
		},
		"a pod with several LD_PRELOAD already set has it appended": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Env: []corev1.EnvVar{
							{
								Name:  "LD_PRELOAD",
								Value: "/foo:/bar",
							},
						},
					},
				},
			}.Create(),
			deployments: defaultDeployments,
			namespaces:  defaultNamespaces,
			expected: map[string]string{
				"LD_PRELOAD": "/foo:/bar:/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
			},
			expectedContainers: defaultContainerNames,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks.
			mockConfig := common.FakeConfigWithValues(t, test.config)
			mockMeta := common.FakeStoreWithDeployment(t, test.deployments)
			mockDynamic := fake.NewSimpleDynamicClient(runtime.NewScheme())
			mockConfig.SetInTest("admission_controller.auto_instrumentation.gradual_rollout.enabled", false)

			// Add the namespaces.
			for _, ns := range test.namespaces {
				mockMeta.(workloadmetamock.Mock).Set(&ns)
			}

			// Setup webhook.
			webhook, err := autoinstrumentation.NewAutoInstrumentation(mockConfig, mockMeta, nil)
			require.NoError(t, err)

			// Mutate pod.
			in := test.pod.DeepCopy()
			mutated, err := webhook.MutatePod(in, in.Namespace, mockDynamic)
			require.NoError(t, err)
			require.True(t, mutated, "the pod was mutated but the webhook returned false")

			// Setup validator.
			validator := testutils.NewPodValidator(in, testutils.InjectionModeAuto)

			// Require environment to match.
			validator.RequireEnvs(t, test.expected, test.expectedContainers)
		})
	}
}

func TestSkippedDueToResources(t *testing.T) {
	// NOTE: This test currently validates behavior under the *default* injection mode.
	// Today that effectively means init-container injection, so the expectations assert
	// init-container-style resource gating
	//
	// If/when the project default injection mode changes (e.g. to CSI or image_volume), this test’s expectations
	// will likely need to be updated (or the test can pin `apm_config.instrumentation.injection_mode` explicitly).
	tests := map[string]struct {
		config              map[string]any
		pod                 *corev1.Pod
		namespaces          []workloadmeta.KubernetesMetadata
		deployments         []common.MockDeployment
		skipped             bool
		expectedContainers  []string
		expectedAnnotations map[string]string
	}{
		"a pod with ample resources is not skipped": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
					},
				},
			}.Create(),
			deployments:        defaultDeployments,
			namespaces:         defaultNamespaces,
			skipped:            false,
			expectedContainers: defaultContainerNames,
		},
		"a pod with low memory is skipped": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("499m"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("499m"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:        defaultDeployments,
			namespaces:         defaultNamespaces,
			skipped:            true,
			expectedContainers: defaultContainerNames,
			expectedAnnotations: map[string]string{
				"internal.apm.datadoghq.com/injection-error": "The overall pod's containers limit is too low for injection, memory pod_limit=50Mi needed=100Mi",
			},
		},
		"a pod with low cpu is skipped": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("0.025"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("0.025"),
							},
						},
					},
				},
			}.Create(),
			deployments:        defaultDeployments,
			namespaces:         defaultNamespaces,
			skipped:            true,
			expectedContainers: defaultContainerNames,
			expectedAnnotations: map[string]string{
				"internal.apm.datadoghq.com/injection-error": "The overall pod's containers limit is too low for injection, cpu pod_limit=25m needed=50m",
			},
		},
		"a pod with low cpu and memory is skipped": {
			config: map[string]any{
				"apm_config.instrumentation.enabled": true,
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("0.025"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("0.025"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:        defaultDeployments,
			namespaces:         defaultNamespaces,
			skipped:            true,
			expectedContainers: defaultContainerNames,
			expectedAnnotations: map[string]string{
				"internal.apm.datadoghq.com/injection-error": "The overall pod's containers limit is too low for injection, cpu pod_limit=25m needed=50m, memory pod_limit=50Mi needed=100Mi",
			},
		},
		"a pod with low cpu and memory but with config override is not skipped": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":                              true,
				"admission_controller.auto_instrumentation.init_resources.cpu":    "101m",
				"admission_controller.auto_instrumentation.init_resources.memory": "301Mi",
			},
			pod: common.FakePodSpec{
				Name:       defaultTestContainer,
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-123",
				Containers: []corev1.Container{
					{
						Name: defaultTestContainer,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("0.025"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("0.025"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
						},
					},
				},
			}.Create(),
			deployments:        defaultDeployments,
			namespaces:         defaultNamespaces,
			skipped:            false,
			expectedContainers: defaultContainerNames,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks.
			mockConfig := common.FakeConfigWithValues(t, test.config)
			mockMeta := common.FakeStoreWithDeployment(t, test.deployments)
			mockDynamic := fake.NewSimpleDynamicClient(runtime.NewScheme())
			mockConfig.SetInTest("admission_controller.auto_instrumentation.gradual_rollout.enabled", false)

			// Add the namespaces.
			for _, ns := range test.namespaces {
				mockMeta.(workloadmetamock.Mock).Set(&ns)
			}

			// Setup webhook.
			webhook, err := autoinstrumentation.NewAutoInstrumentation(mockConfig, mockMeta, nil)
			require.NoError(t, err)

			// Mutate pod.
			in := test.pod.DeepCopy()
			mutated, err := webhook.MutatePod(in, in.Namespace, mockDynamic)
			require.NoError(t, err)
			require.True(t, mutated, "the pod was mutated but the webhook returned false")

			// Setup validator.
			validator := testutils.NewPodValidator(in, testutils.InjectionModeAuto)

			// Ensure the pod was properly skipped due to resources.
			if test.skipped {
				missingEnv := []string{
					"LD_PRELOAD",
				}
				validator.RequireMissingEnvs(t, missingEnv, test.expectedContainers)
				validator.RequireAnnotations(t, test.expectedAnnotations)
				return
			}

			// Otherwise, require injection.
			validator.RequireInjection(t, test.expectedContainers)
		})
	}
}

func newTestNamespace(name string, labels map[string]string) workloadmeta.KubernetesMetadata {
	return workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "namespaces", "", name)),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func languageSetOf(languages ...string) languagemodels.LanguageSet {
	set := languagemodels.LanguageSet{}
	for _, l := range languages {
		_ = set.Add(languagemodels.LanguageName(l))
	}
	return set
}
