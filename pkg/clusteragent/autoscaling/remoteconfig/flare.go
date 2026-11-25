// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	rcflare "github.com/DataDog/datadog-agent/pkg/config/remote/flare"
)

// AddFlareFiles adds the autoscaling client's uptane cache to a Cluster Agent
// flare, next to the default client's one. No-op when no autoscaling
// configuration endpoint is set, so the caller does not have to check.
func AddFlareFiles(fb flaretypes.FlareBuilder, cfg model.Reader) error {
	if !IsConfigured(cfg) {
		return nil
	}
	return rcflare.CopyRemoteConfigDBFile(fb, cfg.GetString("run_path"), DatabaseFileName)
}
