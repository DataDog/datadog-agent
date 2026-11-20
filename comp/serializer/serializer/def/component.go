// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package serializer defines the serializer component, which serializes all
// payload kinds and routes them to the forwarders.
package serializer

import (
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// team: agent-metric-pipelines

// Component serializes all payload kinds and routes them to the forwarders.
//
// The serializer implementation also provides each functional group on its
// own (serializer.MetricsSerializer, serializer.EventsSerializer,
// serializer.MetadataSerializer, serializer.OrchestratorSerializer); prefer
// depending on one of those unless the full set is needed.
type Component interface {
	serializer.MetricSerializer
}
