// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusrclistener implements the RC listener on AGENT_TASK that collects the agent
// status on demand and forwards it to the fleet-api backend.
package statusrclistener

// team: fleet

// Component is the component type.
type Component interface{}
