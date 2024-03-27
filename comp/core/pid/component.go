// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pid writes the current PID to a file, ensuring that the file
// doesn't exist or doesn't contain a PID for a running process.
package pid

// team: agent-shared-components

// Component is the component type.
type Component interface{}
