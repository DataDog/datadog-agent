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

type infraBasicLinuxSuite struct {
	infraBasicSuite
}

func TestInfraBasicLinuxSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t,
		&infraBasicLinuxSuite{
			infraBasicSuite{
				descriptor: e2eos.Ubuntu2204,
			},
		},
	)
}

func (s *infraBasicLinuxSuite) TestBasicChecksWork() {
	s.assertBasicChecksWork()
}

func (s *infraBasicLinuxSuite) TestExcludedIntegrationsDoNotRun() {
	s.assertExcludedIntegrationsDoNotRun()
}

func (s *infraBasicLinuxSuite) TestAdditionalChecksConfiguration() {
	s.assertAdditionalChecksConfiguration()
}

func (s *infraBasicLinuxSuite) TestSchedulerFiltering() {
	s.assertSchedulerFiltering()
}
