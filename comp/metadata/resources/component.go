// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resources implements a component to generate the 'resources' metadata payload.
package resources

// team: agent-shared-components

// Component is the component type.
type Component interface {
	Get() map[string]interface{}
}
