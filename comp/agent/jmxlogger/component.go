// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxlogger implements the logger for JMX.
package jmxlogger

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	JMXInfo(v ...interface{})
	JMXError(v ...interface{}) error
}
