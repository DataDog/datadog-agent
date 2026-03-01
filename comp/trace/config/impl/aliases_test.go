// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package traceconfigimpl implements the trace-agent config component.
package traceconfigimpl

import (
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config/def"
)

// Component is an alias for the Component interface from the def package.
// Used for backward compatibility in tests.
type Component = traceconfig.Component
