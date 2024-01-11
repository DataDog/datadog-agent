// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !docker

//nolint:revive // TODO(CINT) Fix revive linter
package docker

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// Enabled is true if the check is enabled
	Enabled = false
	// CheckName is the name of the check
	CheckName = "docker"
)

func Factory(_ workloadmeta.Component) func() check.Check {
	return nil
}
