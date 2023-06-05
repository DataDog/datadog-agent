// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !orchestrator

package aggregator

import "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"

// buildOrchestratorForwarder builds the orchestrator forwarder.
// This func has been extracted in this file to not include all the orchestrator
// dependencies (k8s, several MBs) while building binaries not needing these.
func buildOrchestratorForwarder() defaultforwarder.Forwarder {
	// do not return any forwarder for builds without the orchestrator tag
	return nil
}
