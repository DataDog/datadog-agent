// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type eudmWindowsSuite struct {
	eudmSuite
}

func TestEUDMWindowsSuite(t *testing.T) {
	t.Parallel()

	suite := &eudmWindowsSuite{
		eudmSuite{
			descriptor: e2eos.WindowsServerDefault,
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}
