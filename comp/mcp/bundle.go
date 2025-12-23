// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mcp implements the "mcp" bundle, which provides a Model Context Protocol
// server that communicates over stdio for AI coding assistants.
package mcp

import (
	"github.com/DataDog/datadog-agent/comp/mcp/agent"
	"github.com/DataDog/datadog-agent/comp/mcp/anomalyhandler"
	"github.com/DataDog/datadog-agent/comp/mcp/client"
	"github.com/DataDog/datadog-agent/comp/mcp/config"
	"github.com/DataDog/datadog-agent/comp/mcp/server"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		server.Module(),
		client.Module(),
		agent.Module(),
		anomalyhandler.Module(),
		config.Module(),
	)
}
