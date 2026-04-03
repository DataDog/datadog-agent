// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectorcontribimpl

import (
	"testing"

	"go.opentelemetry.io/collector/component"
)

var (
	keptExporterTypes = []string{
		"loadbalancing",
	}

	keptProcessorTypes = []string{
	}

	keptReceiverTypes = []string{
		"hostmetrics",
	}

	removedConnectorTypes = []string{
		"spanmetrics",
	}

	removedExtensionTypes = []string{
		"docker_observer",
		"ecs_observer",
		"k8s_observer",
	}

	removedProcessorTypes = []string{
		"filter",
		"k8sattributes",
		"resourcedetection",
		"tail_sampling",
		"transform",
	}

	removedReceiverTypes = []string{
		"fluentforward",
		"jaeger",
		"prometheus",
		"receiver_creator",
		"zipkin",
	}
)

func TestSupportedFactorySurface(t *testing.T) {
	factories, err := components()
	if err != nil {
		t.Fatalf("components() returned error: %v", err)
	}

	assertFactoryTypesPresent(t, factories.Exporters, keptExporterTypes)
	assertFactoryTypesPresent(t, factories.Processors, keptProcessorTypes)
	assertFactoryTypesPresent(t, factories.Receivers, keptReceiverTypes)

	assertFactoryTypesAbsent(t, factories.Connectors, removedConnectorTypes)
	assertFactoryTypesAbsent(t, factories.Extensions, removedExtensionTypes)
	assertFactoryTypesAbsent(t, factories.Processors, removedProcessorTypes)
	assertFactoryTypesAbsent(t, factories.Receivers, removedReceiverTypes)
}

func assertFactoryTypesPresent[T any](t *testing.T, factories map[component.Type]T, types []string) {
	t.Helper()

	factoryTypes := collectFactoryTypes(factories)
	for _, factoryType := range types {
		if _, ok := factoryTypes[factoryType]; !ok {
			t.Fatalf("expected factory type %q to be registered", factoryType)
		}
	}
}

func assertFactoryTypesAbsent[T any](t *testing.T, factories map[component.Type]T, types []string) {
	t.Helper()

	factoryTypes := collectFactoryTypes(factories)
	for _, factoryType := range types {
		if _, ok := factoryTypes[factoryType]; ok {
			t.Fatalf("expected factory type %q to be removed", factoryType)
		}
	}
}

func collectFactoryTypes[T any](factories map[component.Type]T) map[string]struct{} {
	factoryTypes := make(map[string]struct{}, len(factories))
	for componentType := range factories {
		factoryTypes[componentType.String()] = struct{}{}
	}
	return factoryTypes
}
