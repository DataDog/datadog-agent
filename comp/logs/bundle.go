// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs //nolint:revive // TODO(AML) Fix revive linter

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	streamlogs "github.com/DataDog/datadog-agent/comp/logs/streamlogs/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-log-pipelines

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		agentimpl.Module(),
		streamlogs.Module(),
	)
}
