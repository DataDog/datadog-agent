// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
)

// Options for telemetry metrics.
// Creating an Options struct without specifying any of its fields should be the
// equivalent of using the DefaultOptions var.
type Options telemetryComponent.Options

// DefaultOptions for telemetry metrics which don't need to specify any option.
var DefaultOptions Options = Options(telemetryComponent.DefaultOptions)
