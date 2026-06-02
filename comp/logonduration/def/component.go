// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package logonduration provides a component that monitors the duration of a user logon after boot and forwards them to the Datadog Event Management v2 API.
package logonduration

// team: windows-products

// Component is the interface for the logon duration component.
// The component monitors the duration of a user logon after boot and forwards
// it to the Datadog Event Management v2 API intake.
//
// This component has no public methods as all operations are managed internally
// via fx.Lifecycle hooks (Start/Stop).
type Component interface {
}
