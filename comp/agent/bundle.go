// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agent implements the "agent" bundle,
package agent

import (
	"go.uber.org/fx"

	autoexitfx "github.com/DataDog/datadog-agent/comp/agent/autoexit/fx"
	cloudfoundrycontainerfx "github.com/DataDog/datadog-agent/comp/agent/cloudfoundrycontainer/fx"
	expvarserverfx "github.com/DataDog/datadog-agent/comp/agent/expvarserver/fx"
	jmxlogger "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/def"
	jmxloggerfx "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-runtimes

// Bundle defines the fx options for this bundle.
func Bundle(params jmxlogger.Params) fxutil.BundleOptions {
	return fxutil.Bundle(
		autoexitfx.Module(),
		jmxloggerfx.Module(),
		fx.Supply(params),
		expvarserverfx.Module(),
		cloudfoundrycontainerfx.Module(),
	)
}
