// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inframode

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
)

//go:embed fixtures/eudm_hostname.ps1
var eudmHostnameScript string

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

// TestSerialHostname exercises the deployed Agent's WMI lookup, hostname
// selection and host metadata forwarding rather than mocking device identity.
func (s *eudmWindowsSuite) TestSerialHostname() {
	output, err := s.Env().RemoteHost.Execute(eudmHostnameScript)
	s.Require().NoError(err)
	expected := strings.TrimSpace(output)
	s.Require().Equal(expected, s.Env().Agent.Client.Hostname())
	_, err = svcmanager.NewWindows(s.Env().RemoteHost).Restart("datadogagent")
	s.Require().NoError(err)
	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetHostTags(expected)
		if assert.NoError(c, err) {
			assert.NotEmpty(c, payloads, "host metadata must use the serial-suffixed hostname")
		}
	}, 5*time.Minute, 15*time.Second)
}
