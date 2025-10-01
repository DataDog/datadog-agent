// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealth provides convenient imports for the logs agent health checker sub-component.
package logsagenthealth

import (
	// Import the definition package to expose the interface
	_ "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/def"
	// Import the FX module for dependency injection
	_ "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/fx"
)

// This file provides convenient imports for the logs agent health checker sub-component.
// To use this component, import the fx module and inject the Component interface.
