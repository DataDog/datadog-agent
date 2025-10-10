// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infrabasic

import (
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type infraBasicWindowsSuite struct {
	infraBasicSuite
}

func TestInfraBasicWindowsSuite(t *testing.T) {
	t.Parallel()

	suite := &infraBasicWindowsSuite{
		infraBasicSuite{
			descriptor: e2eos.WindowsServerDefault,
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}

func (s *infraBasicWindowsSuite) TestAllowedChecksWork() {
	s.assertAllowedChecksWork()
}

func (s *infraBasicWindowsSuite) TestExcludedChecksAreBlocked() {
	s.assertExcludedChecksAreBlocked()
}

func (s *infraBasicWindowsSuite) TestAdditionalCheckWorks() {
	s.assertAdditionalCheckWorks()
}
