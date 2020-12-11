// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tagger

import "github.com/DataDog/datadog-agent/pkg/telemetry"

var (
	storedEntities = telemetry.NewGaugeWithOpts("tagger", "stored_entities",
		[]string{"source", "prefix"}, "Number of entities in the store.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	updatedEntities = telemetry.NewCounterWithOpts("tagger", "updated_entities",
		[]string{}, "Number of updates made to entities.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	queries = telemetry.NewCounterWithOpts("tagger", "queries",
		[]string{"cardinality"}, "Queries made against the tagger.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)
