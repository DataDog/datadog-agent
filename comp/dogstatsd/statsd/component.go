// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statsd implements a component to get a statsd client.
// Deprecated: import from comp/dogstatsd/statsd/def, comp/dogstatsd/statsd/fx,
// or comp/dogstatsd/statsd/mock instead.
package statsd

import (
	statsddef "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
)

// team: agent-metric-pipelines

// Component is the component type.
// Deprecated: Use comp/dogstatsd/statsd/def.Component directly.
type Component = statsddef.Component
