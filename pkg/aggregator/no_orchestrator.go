// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !orchestrator

package aggregator

// Orchestrator Explorer is enabled by default but
// the forwarder is only created if the orchestrator
// build tag exists

// orchestratorForwarderSupport shows if the orchestrator build tag is enabled
const orchestratorForwarderSupport = false
