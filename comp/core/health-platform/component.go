// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatform provides the health platform component.
// This component collects and reports health information from the host system,
// sending it to the Datadog backend with hostname, host ID, organization ID,
// and a list of issues.
package healthplatform

import (
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
)

// team: agent-runtimes

// Component is the component type.
type Component = healthplatform.Component

// Issue represents an individual issue to be reported
type Issue = healthplatform.Issue
