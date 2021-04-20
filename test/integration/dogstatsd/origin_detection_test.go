// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"testing"

	"github.com/StackVista/stackstate-agent/pkg/config"
)

func TestUDSOriginDetection(t *testing.T) {
	// [STS] We're not using UDS right now and we're getting flakiness in testing
	t.Skip()
	config.SetupLogger(
		"debug",
		"",
		"",
		false,
		true,
		false,
	)

	testUDSOriginDetection(t)
}
