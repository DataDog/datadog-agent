// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package hostname

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type windowsHostnameDriftSuite struct {
	baseHostnameDriftSuite
}

func TestWindowsHostnameDriftSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsHostnameDriftSuite{}
	e2e.Run(t, suite, suite.getSuiteOptions(os.WindowsServerDefault)...)
}
