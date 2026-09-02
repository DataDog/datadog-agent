// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// OpenTelemetry SDK env vars carrying Unified Service Tagging equivalents.
// https://opentelemetry.io/docs/languages/sdk-configuration/general
// https://opentelemetry.io/docs/specs/semconv/resource/
const (
	envVarOtelServiceName        = "OTEL_SERVICE_NAME"
	envVarOtelResourceAttributes = "OTEL_RESOURCE_ATTRIBUTES"
)

// otelResourceAttributeKeysByDDEnvVar maps a Datadog UST env var to the
// OTEL_RESOURCE_ATTRIBUTES keys that carry an equivalent value.
//
// Keep in sync with the tagger's own OTel resource-attribute mapping, see
// otelResourceAttributesMapping in
// comp/core/tagger/collectors/workloadmeta_extract.go.
var otelResourceAttributeKeysByDDEnvVar = map[string][]string{
	kubernetes.VersionTagEnvVar: {"service.version"},
	kubernetes.EnvTagEnvVar:     {"deployment.environment", "deployment.environment.name"},
}

// otelEquivalentCheck reports whether a container already carries an
// OpenTelemetry-native equivalent of a Datadog env var that SSI would
// otherwise inject. It's used to avoid clobbering or duplicating
// configuration that's already owned by an OTel-native SDK (e.g. a DDOT
// SDK), so that DD-native and OTel-native instrumentation can coexist
// during a progressive rollout.
type otelEquivalentCheck func(c *corev1.Container) bool

// otelEquivalentOf returns a check for whether a container already defines
// an OpenTelemetry equivalent of ddEnvVarName: OTEL_SERVICE_NAME for
// DD_SERVICE, or the relevant OTEL_RESOURCE_ATTRIBUTES key for DD_ENV /
// DD_VERSION. Returns nil if ddEnvVarName has no known OTel equivalent.
func otelEquivalentOf(ddEnvVarName string) otelEquivalentCheck {
	if ddEnvVarName == kubernetes.ServiceTagEnvVar {
		return func(c *corev1.Container) bool {
			return containerHasEnvVarName(c, envVarOtelServiceName)
		}
	}

	keys, ok := otelResourceAttributeKeysByDDEnvVar[ddEnvVarName]
	if !ok {
		return nil
	}

	return func(c *corev1.Container) bool {
		value, ok := containerStaticEnvVarValue(c, envVarOtelResourceAttributes)
		if !ok {
			return false
		}

		attrs := parseOtelResourceAttributes(value)
		for _, key := range keys {
			if _, present := attrs[key]; present {
				return true
			}
		}

		return false
	}
}

func containerHasEnvVarName(c *corev1.Container, name string) bool {
	for _, e := range c.Env {
		if e.Name == name {
			return true
		}
	}
	return false
}

// containerStaticEnvVarValue returns the literal value of a container env
// var. ok is false if the container doesn't declare the var, or declares it
// via ValueFrom: such values can't be resolved statically at admission time.
func containerStaticEnvVarValue(c *corev1.Container, name string) (string, bool) {
	for _, e := range c.Env {
		if e.Name != name {
			continue
		}
		if e.ValueFrom != nil {
			return "", false
		}
		return e.Value, true
	}
	return "", false
}

// parseOtelResourceAttributes parses an OTEL_RESOURCE_ATTRIBUTES value
// ("key1=value1,key2=value2"), the same way the tagger does. See
// addOpenTelemetryStandardTags in
// comp/core/tagger/collectors/workloadmeta_extract.go.
func parseOtelResourceAttributes(value string) map[string]string {
	attrs := make(map[string]string)
	for _, pair := range strings.Split(value, ",") {
		fields := strings.SplitN(pair, "=", 2)
		if len(fields) != 2 {
			continue
		}
		key, val := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
		if key != "" {
			attrs[key] = val
		}
	}
	return attrs
}
