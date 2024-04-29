// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import "github.com/DataDog/datadog-agent/pkg/telemetry"

var emittedEvents = telemetry.NewCounterWithOpts(
	CheckName,
	"emitted_events",
	[]string{"event_type", "object_kind"},
	"Number of events emitted by the check",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)
