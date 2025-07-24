// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package softwareinventory defines the interface for the inventory software component.
// This component collects and reports software inventory information from the host system.
// It provides metadata about installed software applications, including their names,
// versions, installation dates, and other relevant details for inventory tracking.
package softwareinventory

// team: windows-agent

// Component is the interface for the inventory software component.
// This component is responsible for collecting software inventory data from the host system
// and providing it in a structured format for reporting and monitoring purposes.
// The component supports both automatic periodic collection and manual refresh triggers.
type Component interface {
}
