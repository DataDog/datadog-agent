// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet || !orchestrator

// Package pod is used for the orchestrator pod check
package pod

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName = "orchestrator_pod"
)

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewNoneOption[func() check.Check]()
}
