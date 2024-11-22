// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"testing"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

func TestUDSOriginDetectionDatagram(t *testing.T) {
	pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
		pkgconfigsetup.Datadog(),
	)

	testUDSOriginDetection(t, "unixgram")
}

func TestUDSOriginDetectionStream(t *testing.T) {
	pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
		pkgconfigsetup.Datadog(),
	)

	testUDSOriginDetection(t, "unix")
}
