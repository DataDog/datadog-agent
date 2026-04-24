// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hookbenchsubscriber is a benchmark-only hook subscriber component.
// It subscribes N times to the metrics pipeline hook, discarding every payload,
// so that regression experiments can measure the CPU/memory overhead of hook
// delivery as a function of subscriber count.
package hookbenchsubscriber

// team: agent-shared-components

// Component is the hook benchmark subscriber component interface.
type Component interface{}
