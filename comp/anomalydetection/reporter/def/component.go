// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporter provides a component that formats and dispatches anomaly
// detection events to the Datadog backend or stdout.
package reporter

// team: q-branch

// Component is the reporter component type.
type Component interface{}
