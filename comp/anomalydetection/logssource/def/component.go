// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logssource feeds logs into the observer. It reuses the Logs Agent
// message stream when available and otherwise starts a standalone container and
// kubelet collection path.
package logssource

// team: agent-anomaly-detection

// Component is the component type.
type Component interface{}
