// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

// Params is the input parameter struct for the RC client Component.
type Params struct {
	AgentName     string
	AgentVersion  string
	IsSystemProbe bool
}
