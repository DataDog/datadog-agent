// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !gnmi

// Package gnmi implements the gNMI core check for network device monitoring
// (stub implementation, for agent flavors built without the gnmi tag such as
// the IoT agent).
package gnmi

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the gNMI check.
const CheckName = "gnmi"

// Factory returns no check when the agent is built without the gnmi tag.
func Factory() option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
