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
	"github.com/stretchr/testify/require"

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
		require.NoError(c, err)
		matched := 0
		for _, raw := range payloads {
			data, err := aggregator.Inflate(raw.Data, raw.Encoding)
			require.NoError(c, err)
			var payload struct {
				Hostname string                 `json:"internalHostname"`
				Meta     map[string]interface{} `json:"meta"`
				Gohai    string                 `json:"gohai"`
			}
			require.NoError(c, json.Unmarshal(data, &payload))
			if payload.Meta == nil {
				continue // Other payload types share the intake endpoint.
			}
			matched++
			assert.Equal(c, expected, payload.Hostname)
			for _, field := range []string{"hostname", "agent-hostname", "socket-hostname", "socket-fqdn"} {
				assert.Equal(c, expected, payload.Meta[field], field)
			}
			for _, field := range []string{"host_aliases", "ec2-hostname", "instance-id", "legacy-resolution-hostname", "ccrid"} {
				assert.Empty(c, payload.Meta[field], field)
			}
			var hardware struct {
				Platform struct {
					Hostname string `json:"hostname"`
				} `json:"platform"`
			}
			require.NoError(c, json.Unmarshal([]byte(payload.Gohai), &hardware))
			assert.Equal(c, expected, hardware.Platform.Hostname)
		}
		assert.Positive(c, matched, "no host metadata received")
	}, 5*time.Minute, 15*time.Second)
}
