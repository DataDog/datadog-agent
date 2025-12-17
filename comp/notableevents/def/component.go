// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package notableevents provides a component that monitors notable system events and forwards them to the Datadog Event Management v2 API.
package notableevents

// team: windows-products

// Component is the interface for the notable events component.
// The component monitors notable system events and forwards
// them to the Datadog Event Management v2 API intake.
//
// This component has no public methods as all operations are managed internally
// via fx.Lifecycle hooks (Start/Stop).
type Component interface {
}
