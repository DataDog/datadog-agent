// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inframode

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
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
		payloads, err := s.Env().FakeIntake.Client().GetRawPayloads("/intake/")
		if !assert.NoError(c, err, "failed to fetch raw payloads from fakeintake") {
			return
		}
		matched := 0
		for _, raw := range payloads {
			data, err := aggregator.Inflate(raw.Data, raw.Encoding)
			if !assert.NoError(c, err, "failed to inflate intake payload") {
				continue
			}
			var payload struct {
				Hostname string                 `json:"internalHostname"`
				Meta     map[string]interface{} `json:"meta"`
			}
			if !assert.NoError(c, json.Unmarshal(data, &payload), "failed to unmarshal intake payload") {
				continue
			}
			if payload.Meta == nil {
				continue // Other payload types share the intake endpoint.
			}
			// Only assert on this suite's own agent: fakeintake can receive host
			// metadata from other senders (e.g. payloads with an empty hostname),
			// which are not this test's concern.
			if payload.Hostname != expected {
				continue
			}
			matched++
			assert.NotContains(c, payload.Meta, "agent-hostname")
			assert.NotEmpty(c, payload.Meta["socket-hostname"])
			assert.NotEmpty(c, payload.Meta["socket-fqdn"])
			assert.Contains(c, payload.Meta["host_aliases"], "eudm-test-alias")
		}
		// If the agent had picked the wrong hostname, no payload would match.
		assert.Positive(c, matched, "no host metadata received for host %s", expected)
	}, 5*time.Minute, 15*time.Second)
}
