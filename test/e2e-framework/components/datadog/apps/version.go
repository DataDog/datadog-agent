// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package apps

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const Version = "v0.0.8"

func Image(e config.Env, repo string) string {
	if reg := e.InternalRegistry(); reg != "" && reg != "none" {
		return reg + "/" + repo + ":" + Version
	}
	return "ghcr.io/datadog/" + repo + ":" + Version
}
