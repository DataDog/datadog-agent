// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package modules

import (
	"context"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
)

// wireSBOMUsage does nothing here: the SBOM resolver that would consume the file
// indexes is Linux only.
func wireSBOMUsage(*eventmonitor.Opts, *secconfig.Config, ipc.Component) func(context.Context) {
	return func(context.Context) {}
}
