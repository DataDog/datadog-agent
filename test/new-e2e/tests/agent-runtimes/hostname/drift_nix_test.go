// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package hostname

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type linuxHostnameDriftSuite struct {
	baseHostnameDriftSuite
}

func TestLinuxHostnameDriftSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxHostnameDriftSuite{}
	e2e.Run(t, suite, suite.getSuiteOptions(os.UbuntuDefault)...)
}
