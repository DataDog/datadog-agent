// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"math/rand"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
)

type packageApmInjectSuite struct {
	packageBaseSuite
}

func testApmInjectAgent(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageApmInjectSuite{
		packageBaseSuite: newPackageSuite("apm-inject", os, arch),
	}
}

func (s *packageApmInjectSuite) TestInstall() {
	s.RunInstallScript()
	defer s.Purge()
	s.InstallAgentPackage()
	s.InstallInjectorPackageTemp()
	s.InstallPackageLatest("datadog-apm-library-python")
	s.host.StartExamplePythonApp()
	defer s.host.StopExamplePythonApp()

	traceID := rand.Uint64()
	s.host.CallExamplePythonApp(fmt.Sprint(traceID))
	state := s.host.State()

	state.AssertFileExists("/etc/ld.so.preload", 0644, "root", "root")

	s.assertTraceReceived(traceID)
}

func (s *packageApmInjectSuite) assertTraceReceived(traceID uint64) {
	found := assert.Eventually(s.T(), func() bool {
		tracePayloads, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(s.T(), err)
		for _, tracePayload := range tracePayloads {
			for _, tracerPayload := range tracePayload.TracerPayloads {
				for _, chunk := range tracerPayload.Chunks {
					for _, span := range chunk.Spans {
						if span.TraceID == traceID {
							return true
						}
					}
				}
			}
		}
		return false
	}, time.Second*30, time.Second*1)
	if !found {
		tracePayloads, _ := s.Env().FakeIntake.Client().GetTraces()
		s.T().Logf("Traces received: %v", tracePayloads)
		s.T().Logf("Server logs: %v", s.Env().RemoteHost.MustExecute("cat /tmp/server.log"))
		s.T().Logf("Trace Agent logs: %v", s.Env().RemoteHost.MustExecute("cat /var/log/datadog/trace-agent.log"))
	}
}
