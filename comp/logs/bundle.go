// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs provides the logs component bundle
package logs

import (
	kubehealthfx "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/fx"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	auditorfx "github.com/DataDog/datadog-agent/comp/logs/auditor/fx"
	streamlogs "github.com/DataDog/datadog-agent/comp/logs/streamlogs/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-log-pipelines

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		kubehealthfx.Module(),
		agentimpl.Module(),
		streamlogs.Module(),
		auditorfx.Module(),
	)
}
