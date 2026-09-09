// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package otelinstrumentation

import (
	"testing"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// passthroughRequest builds what the resolver would hand to passthrough for one language.
func passthroughRequest(language Language, spec otelv1alpha1.InstrumentationSpec, containers ...string) LanguageRequest {
	return LanguageRequest{
		Language: language,
		Instrumentation: &otelv1alpha1.Instrumentation{
			ObjectMeta: metav1.ObjectMeta{Namespace: "workloads", Name: "default"},
			Spec:       spec,
		},
		Containers: containers,
		Mode:       ModeOTel,
		ModeSource: ModeSourceOperatorDefault,
	}
}

// injectionFor builds an injection and fails the test if there is none.
func injectionFor(t *testing.T, language Language, spec otelv1alpha1.InstrumentationSpec, pod *corev1.Pod, containers ...string) Injection {
	t.Helper()
	injection, ok := BuildInjection(passthroughRequest(language, spec, containers...), pod)
	require.True(t, ok, "expected an injection for %s", language)
	return injection
}

// envMap flattens one container's variables, applied to an empty container.
func envMap(t *testing.T, injection Injection, container string) map[string]string {
	t.Helper()
	values := map[string]string{}
	for _, ci := range injection.Containers {
		if ci.Container != container {
			continue
		}
		for _, env := range ci.EnvVars {
			values[env.Name] = env.Apply("")
		}
		return values
	}
	t.Fatalf("container %q was not injected", container)
	return nil
}

// containerEnv reads a variable off a pod's container after Apply.
func containerEnv(t *testing.T, pod *corev1.Pod, container, name string) (string, bool) {
	t.Helper()
	for _, ctr := range pod.Spec.Containers {
		if ctr.Name != container {
			continue
		}
		for _, env := range ctr.Env {
			if env.Name == name {
				return env.Value, true
			}
		}
	}
	return "", false
}

func TestPassthroughJava(t *testing.T) {
	pod := translatePod("app")
	injection := injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod, "app")

	assert.Equal(t, "opentelemetry-auto-instrumentation-java", injection.Volume.Name)
	require.NotNil(t, injection.Volume.EmptyDir)
	assert.Equal(t, "opentelemetry-auto-instrumentation-java", injection.InitContainer.Name)
	defaultImage, _ := DefaultImage(Java)
	assert.Equal(t, defaultImage, injection.InitContainer.Image)
	assert.Equal(t,
		[]string{"cp", "/javaagent.jar", "/otel-auto-instrumentation-java/javaagent.jar"},
		injection.InitContainer.Command,
	)
	require.Len(t, injection.InitContainer.VolumeMounts, 1)
	assert.Equal(t, "/otel-auto-instrumentation-java", injection.InitContainer.VolumeMounts[0].MountPath)

	require.Len(t, injection.Containers, 1)
	// Java is the one language whose application mount path carries the container name.
	assert.Equal(t, "/otel-auto-instrumentation-java-app", injection.Containers[0].Mount.MountPath)
	assert.Equal(t,
		" -javaagent:/otel-auto-instrumentation-java-app/javaagent.jar",
		envMap(t, injection, "app")["JAVA_TOOL_OPTIONS"],
	)
}

func TestPassthroughJavaPerContainerMountPath(t *testing.T) {
	pod := translatePod("web", "worker")
	injection := injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod, "web", "worker")

	require.Len(t, injection.Containers, 2)
	assert.Equal(t, "/otel-auto-instrumentation-java-web", injection.Containers[0].Mount.MountPath)
	assert.Equal(t, "/otel-auto-instrumentation-java-worker", injection.Containers[1].Mount.MountPath)
	assert.Contains(t, envMap(t, injection, "worker")["JAVA_TOOL_OPTIONS"], "-java-worker/javaagent.jar")
}

func TestPassthroughNodeJS(t *testing.T) {
	pod := translatePod("app")
	injection := injectionFor(t, NodeJS, otelv1alpha1.InstrumentationSpec{}, pod)

	assert.Equal(t,
		[]string{"cp", "-r", "/autoinstrumentation/.", "/otel-auto-instrumentation-nodejs"},
		injection.InitContainer.Command,
	)
	assert.Equal(t, "/otel-auto-instrumentation-nodejs", injection.Containers[0].Mount.MountPath)

	env := envMap(t, injection, "app")
	assert.Equal(t, " --require /otel-auto-instrumentation-nodejs/autoinstrumentation.js", env["NODE_OPTIONS"])
	assert.Equal(t, "otlp", env["OTEL_METRICS_EXPORTER"])
}

func TestPassthroughPython(t *testing.T) {
	pod := translatePod("app")
	injection := injectionFor(t, Python, otelv1alpha1.InstrumentationSpec{}, pod)

	env := envMap(t, injection, "app")
	// The auto_instrumentation directory has to come first so its sitecustomize.py wins.
	assert.Equal(t,
		"/otel-auto-instrumentation-python/opentelemetry/instrumentation/auto_instrumentation:/otel-auto-instrumentation-python",
		env["PYTHONPATH"],
	)
	assert.Equal(t, "http/protobuf", env["OTEL_EXPORTER_OTLP_PROTOCOL"])
	assert.Equal(t, "otlp", env["OTEL_TRACES_EXPORTER"])
	assert.Equal(t, "otlp", env["OTEL_LOGS_EXPORTER"])
}

func TestPassthroughDotNet(t *testing.T) {
	pod := translatePod("app")
	injection := injectionFor(t, DotNet, otelv1alpha1.InstrumentationSpec{}, pod)

	env := envMap(t, injection, "app")
	assert.Equal(t, "1", env["CORECLR_ENABLE_PROFILING"])
	assert.Equal(t, "{918728DD-259F-4A6A-AC2B-B85E1B658318}", env["CORECLR_PROFILER"])
	assert.Equal(t,
		"/otel-auto-instrumentation-dotnet/linux/OpenTelemetry.AutoInstrumentation.Native.so",
		env["CORECLR_PROFILER_PATH"],
	)
	assert.Equal(t, "/otel-auto-instrumentation-dotnet", env["OTEL_DOTNET_AUTO_HOME"])
	assert.Equal(t,
		"/otel-auto-instrumentation-dotnet/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll",
		env["DOTNET_STARTUP_HOOKS"],
	)
	assert.Equal(t, "/otel-auto-instrumentation-dotnet/store", env["DOTNET_SHARED_STORE"])
}

func TestPassthroughMergesWithExistingValues(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		variable string
		existing string
		expected string
	}{
		{
			name:     "java appends",
			language: Java,
			variable: "JAVA_TOOL_OPTIONS",
			existing: "-Xmx512m",
			expected: "-Xmx512m -javaagent:/otel-auto-instrumentation-java-app/javaagent.jar",
		},
		{
			name:     "nodejs appends",
			language: NodeJS,
			variable: "NODE_OPTIONS",
			existing: "--max-old-space-size=512",
			expected: "--max-old-space-size=512 --require /otel-auto-instrumentation-nodejs/autoinstrumentation.js",
		},
		{
			// The user's path has to stay reachable, so it is wrapped rather than
			// replaced or appended to.
			name:     "python wraps",
			language: Python,
			variable: "PYTHONPATH",
			existing: "/app",
			expected: "/otel-auto-instrumentation-python/opentelemetry/instrumentation/auto_instrumentation:/app:/otel-auto-instrumentation-python",
		},
		{
			name:     "dotnet concatenates",
			language: DotNet,
			variable: "DOTNET_STARTUP_HOOKS",
			existing: "/app/hook.dll",
			expected: "/app/hook.dll:/otel-auto-instrumentation-dotnet/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := translatePod("app")
			pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: test.variable, Value: test.existing}}

			injection := injectionFor(t, test.language, otelv1alpha1.InstrumentationSpec{}, pod)
			injection.Apply(pod)

			value, ok := containerEnv(t, pod, "app", test.variable)
			require.True(t, ok)
			assert.Equal(t, test.expected, value)
		})
	}
}

func TestPassthroughSkipsContainerWithValueFrom(t *testing.T) {
	pod := translatePod("web", "worker")
	// Upstream refuses to merge into a variable it cannot read, and drops the container.
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name: "JAVA_TOOL_OPTIONS",
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "opts"},
		},
	}}

	injection := injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod, "web", "worker")
	require.Len(t, injection.Containers, 1)
	assert.Equal(t, "worker", injection.Containers[0].Container)
}

func TestPassthroughNoInjectionWhenEveryContainerIsSkipped(t *testing.T) {
	pod := translatePod("app")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name: "JAVA_TOOL_OPTIONS",
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "opts"},
		},
	}}

	_, ok := BuildInjection(passthroughRequest(Java, otelv1alpha1.InstrumentationSpec{}), pod)
	assert.False(t, ok)
}

// A variable set with valueFrom that this mode only sets when absent is the user's, and
// is left alone rather than blocking the container.
func TestPassthroughValueFromOnSetIfAbsentVariableIsKept(t *testing.T) {
	pod := translatePod("app")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name:      "OTEL_METRICS_EXPORTER",
		ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "exporter"}},
	}}

	injection := injectionFor(t, NodeJS, otelv1alpha1.InstrumentationSpec{}, pod)
	require.Len(t, injection.Containers, 1)

	injection.Apply(pod)
	value, ok := containerEnv(t, pod, "app", "OTEL_METRICS_EXPORTER")
	assert.True(t, ok)
	assert.Empty(t, value, "the reference must not be replaced with a literal")
}

func TestPassthroughResourceAttributes(t *testing.T) {
	pod := translatePod("app")
	pod.Annotations["resource.opentelemetry.io/team"] = "checkout"
	spec := otelv1alpha1.InstrumentationSpec{
		Resource: otelv1alpha1.Resource{Attributes: map[string]string{"tier": "backend"}},
	}

	injection := injectionFor(t, Python, spec, pod)
	// Keys are sorted, as upstream sorts them.
	assert.Equal(t,
		"k8s.container.name=app,k8s.namespace.name=workloads,service.namespace=workloads,team=checkout,tier=backend",
		envMap(t, injection, "app")[envOTelResourceAttributes],
	)
}

// Upstream never overwrites an attribute the container already carries, and neither does
// this: the shared keys are dropped from what we contribute, and the rest is appended.
func TestPassthroughResourceAttributesKeepExistingKeys(t *testing.T) {
	pod := translatePod("app")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name:  envOTelResourceAttributes,
		Value: "k8s.namespace.name=mine,own=value",
	}}

	injection := injectionFor(t, Python, otelv1alpha1.InstrumentationSpec{}, pod)
	injection.Apply(pod)

	value, ok := containerEnv(t, pod, "app", envOTelResourceAttributes)
	require.True(t, ok)
	assert.Equal(t,
		"k8s.namespace.name=mine,own=value,k8s.container.name=app,service.namespace=workloads",
		value,
	)
}

// The Kubernetes attributes needing a downward-API reference are not emitted, so no value
// depends on the env var ordering Kubernetes imposes on $(VAR) expansion.
func TestPassthroughEmitsNoVariableReferences(t *testing.T) {
	pod := translatePod("app")
	for _, language := range MappableLanguages {
		injection := injectionFor(t, language, otelv1alpha1.InstrumentationSpec{}, pod)
		for name, value := range envMap(t, injection, "app") {
			assert.NotContains(t, value, "$(", "%s of %s", name, language)
		}
	}
}

func TestPassthroughServiceName(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*corev1.Pod, *otelv1alpha1.InstrumentationSpec)
		expected string
	}{
		{
			name: "annotation wins",
			prepare: func(pod *corev1.Pod, _ *otelv1alpha1.InstrumentationSpec) {
				pod.Annotations["resource.opentelemetry.io/service.name"] = "annotated"
			},
			expected: "annotated",
		},
		{
			name: "label when the custom resource opted in",
			prepare: func(pod *corev1.Pod, spec *otelv1alpha1.InstrumentationSpec) {
				useLabels := true
				spec.Defaults.UseLabelsForResourceAttributes = useLabels
				pod.Labels["app.kubernetes.io/name"] = "labelled"
			},
			expected: "labelled",
		},
		{
			name: "custom resource attribute",
			prepare: func(_ *corev1.Pod, spec *otelv1alpha1.InstrumentationSpec) {
				spec.Resource = otelv1alpha1.Resource{Attributes: map[string]string{"service.name": "authored"}}
			},
			expected: "authored",
		},
		{
			// The owner-derived tiers need API reads, so the container name is the last
			// resort here where upstream would have used the Deployment name.
			name:     "container name as the last resort",
			prepare:  func(*corev1.Pod, *otelv1alpha1.InstrumentationSpec) {},
			expected: "app",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := translatePod("app")
			spec := otelv1alpha1.InstrumentationSpec{}
			test.prepare(pod, &spec)

			injection := injectionFor(t, Java, spec, pod)
			assert.Equal(t, test.expected, envMap(t, injection, "app")[envOTelServiceName])
		})
	}
}

// The node and pod addresses are emitted, and must be declared before any variable that
// references them — Kubernetes expands $(VAR) only against earlier declarations.
func TestPassthroughNodeAddressComesBeforeTheEndpoint(t *testing.T) {
	pod := translatePod("app")
	spec := otelv1alpha1.InstrumentationSpec{
		Exporter: otelv1alpha1.Exporter{Endpoint: "http://$(OTEL_NODE_IP):4318"},
	}

	injectionFor(t, Java, spec, pod).Apply(pod)

	var names []string
	for _, env := range pod.Spec.Containers[0].Env {
		names = append(names, env.Name)
		if env.Name == "OTEL_NODE_IP" {
			require.NotNil(t, env.ValueFrom)
			assert.Equal(t, "status.hostIP", env.ValueFrom.FieldRef.FieldPath)
			assert.Empty(t, env.Value)
		}
	}
	assert.Less(t,
		indexOf(names, "OTEL_NODE_IP"), indexOf(names, envOTelExporterEndpoint),
		"the endpoint may reference $(OTEL_NODE_IP), so the address must be declared first",
	)
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

// Passthrough forwards the endpoint as written, where swap mode drops it.
func TestPassthroughExporterEndpoint(t *testing.T) {
	pod := translatePod("app")
	spec := otelv1alpha1.InstrumentationSpec{
		Exporter:    otelv1alpha1.Exporter{Endpoint: "http://collector:4318"},
		Propagators: []otelv1alpha1.Propagator{"tracecontext", "baggage"},
		Sampler:     otelv1alpha1.Sampler{Type: "parentbased_traceidratio", Argument: "0.25"},
	}

	env := envMap(t, injectionFor(t, Java, spec, pod), "app")
	assert.Equal(t, "http://collector:4318", env[envOTelExporterEndpoint])
	assert.Equal(t, "tracecontext,baggage", env[envOTelPropagators])
	assert.Equal(t, "parentbased_traceidratio", env[envOTelTracesSampler])
	assert.Equal(t, "0.25", env[envOTelTracesSamplerArg])
}

func TestPassthroughCustomResourceEnv(t *testing.T) {
	pod := translatePod("app")
	spec := otelv1alpha1.InstrumentationSpec{
		Env:  []corev1.EnvVar{{Name: "OTEL_LOG_LEVEL", Value: "debug"}},
		Java: otelv1alpha1.Java{Env: []corev1.EnvVar{{Name: "OTEL_JAVA_OPTION", Value: "on"}}},
	}

	env := envMap(t, injectionFor(t, Java, spec, pod), "app")
	assert.Equal(t, "debug", env["OTEL_LOG_LEVEL"])
	assert.Equal(t, "on", env["OTEL_JAVA_OPTION"])
}

func TestPassthroughVolumeSizeLimit(t *testing.T) {
	pod := translatePod("app")
	injection := injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod)
	assert.Equal(t, "200Mi", injection.Volume.EmptyDir.SizeLimit.String())

	limit := resource.MustParse("64Mi")
	spec := otelv1alpha1.InstrumentationSpec{Java: otelv1alpha1.Java{VolumeSizeLimit: &limit}}
	injection = injectionFor(t, Java, spec, pod)
	assert.Equal(t, "64Mi", injection.Volume.EmptyDir.SizeLimit.String())
}

func TestPassthroughInitContainerResources(t *testing.T) {
	pod := translatePod("app")
	spec := otelv1alpha1.InstrumentationSpec{
		Java: otelv1alpha1.Java{Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
		}},
	}

	injection := injectionFor(t, Java, spec, pod)
	assert.Equal(t, "500m", injection.InitContainer.Resources.Limits.Cpu().String())
}

func TestPassthroughUsesTheCustomResourceImage(t *testing.T) {
	pod := translatePod("app")
	spec := otelv1alpha1.InstrumentationSpec{Java: otelv1alpha1.Java{Image: "registry.local/java:1.2.3"}}

	injection := injectionFor(t, Java, spec, pod)
	assert.Equal(t, "registry.local/java:1.2.3", injection.InitContainer.Image)
}

func TestPassthroughNoInjectionCases(t *testing.T) {
	spec := otelv1alpha1.InstrumentationSpec{}

	t.Run("nil pod", func(t *testing.T) {
		_, ok := BuildInjection(passthroughRequest(Java, spec), nil)
		assert.False(t, ok)
	})

	t.Run("language with no contract", func(t *testing.T) {
		_, ok := BuildInjection(passthroughRequest(Language("go"), spec), translatePod("app"))
		assert.False(t, ok)
	})

	t.Run("selection matching nothing", func(t *testing.T) {
		_, ok := BuildInjection(passthroughRequest(Java, spec, "sidecar"), translatePod("app"))
		assert.False(t, ok)
	})
}

// The unsupported fields are reported, not fatal: the injection still happens with the
// behaviour this mode does implement.
func TestPassthroughUnsupportedFieldsDoNotStopTheInjection(t *testing.T) {
	pod := translatePod("app")
	pod.Annotations["instrumentation.opentelemetry.io/otel-python-platform"] = "musl"
	spec := otelv1alpha1.InstrumentationSpec{
		Resource: otelv1alpha1.Resource{AddK8sUIDAttributes: true},
		Python: otelv1alpha1.Python{
			VolumeClaimTemplate: corev1.PersistentVolumeClaimTemplate{
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			},
		},
	}

	injection := injectionFor(t, Python, spec, pod)
	assert.NotNil(t, injection.Volume.EmptyDir, "the volume claim is ignored, not honoured")
	assert.Contains(t,
		envMap(t, injection, "app")["PYTHONPATH"],
		"/otel-auto-instrumentation-python",
	)
}

func TestPassthroughApply(t *testing.T) {
	pod := translatePod("web", "worker")
	injection := injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod, "web")
	injection.Apply(pod)

	require.Len(t, pod.Spec.InitContainers, 1)
	require.Len(t, pod.Spec.Volumes, 1)
	assert.Equal(t, "opentelemetry-auto-instrumentation-java", pod.Spec.Volumes[0].Name)

	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, "/otel-auto-instrumentation-java-web", pod.Spec.Containers[0].VolumeMounts[0].MountPath)
	// Only the selected container is touched.
	assert.Empty(t, pod.Spec.Containers[1].VolumeMounts)
	assert.Empty(t, pod.Spec.Containers[1].Env)
}

// A selection may name an init container, and it must get the mount as well as the
// environment: the environment alone points at a path that is not there.
func TestPassthroughApplyToInitContainer(t *testing.T) {
	pod := translatePod("app")
	pod.Spec.InitContainers = []corev1.Container{{Name: "migrate", Image: "registry/app"}}

	injection := injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod, "migrate")
	injection.Apply(pod)

	// The copy container is prepended, so it runs before the one being instrumented.
	require.Len(t, pod.Spec.InitContainers, 2)
	assert.Equal(t, "opentelemetry-auto-instrumentation-java", pod.Spec.InitContainers[0].Name)

	migrate := pod.Spec.InitContainers[1]
	require.Len(t, migrate.VolumeMounts, 1)
	assert.Equal(t, "/otel-auto-instrumentation-java-migrate", migrate.VolumeMounts[0].MountPath)
	require.NotEmpty(t, migrate.Env)
}

// Two languages share nothing, so both injections land side by side.
func TestPassthroughApplyTwoLanguages(t *testing.T) {
	pod := translatePod("app")
	injectionFor(t, Java, otelv1alpha1.InstrumentationSpec{}, pod).Apply(pod)
	injectionFor(t, Python, otelv1alpha1.InstrumentationSpec{}, pod).Apply(pod)

	assert.Len(t, pod.Spec.InitContainers, 2)
	assert.Len(t, pod.Spec.Volumes, 2)
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 2)

	_, java := containerEnv(t, pod, "app", "JAVA_TOOL_OPTIONS")
	_, python := containerEnv(t, pod, "app", "PYTHONPATH")
	assert.True(t, java && python)
}

// Applying the same injection to a pod that already carries it replaces the volume and
// the init container instead of adding a second one.
func TestPassthroughApplyIsIdempotentForVolumesAndInitContainers(t *testing.T) {
	pod := translatePod("app")
	injection := injectionFor(t, Python, otelv1alpha1.InstrumentationSpec{}, pod)
	injection.Apply(pod)
	injection.Apply(pod)

	assert.Len(t, pod.Spec.InitContainers, 1)
	assert.Len(t, pod.Spec.Volumes, 1)
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1)
}
