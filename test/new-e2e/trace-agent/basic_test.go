// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceagent

import (
	"flag"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	devMode = flag.Bool("devmode", false, "enable dev mode")
)

type dockerTCPSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestDockerTCPSuite runs basic Trace Agent tests over the TCP transport
func TestDockerTCPSuite(t *testing.T) {
	var suiteParams []e2e.SuiteOption
	isCI, _ := strconv.ParseBool(os.Getenv("CI"))
	if isCI {
		t.Skipf("blocked by APL-2786")
	}
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")

	if devModeE, err := strconv.ParseBool(devModeEnv); (err == nil && devModeE) || *devMode {
		t.Log("Running in Dev Mode.")
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}
	suiteParams = append(suiteParams, e2e.WithProvisioner(awsdocker.Provisioner()))
	e2e.Run(t, &dockerTCPSuite{}, suiteParams...)
}

func waitRemotePort(v *dockerTCPSuite, port uint16) error {
	var (
		c   net.Conn
		err error
	)
	for i := 0; i < 10; i++ {
		v.T().Logf("Waiting for remote:%v", port)
		c, err = v.Env().Host.DialRemotePort(port)
		if err != nil {
			v.T().Logf("Failed to dial remote:%v: %s\n", port, err)
			time.Sleep(1 * time.Second)
		} else {
			v.T().Logf("Connected to remote:%v\n", port)
			defer c.Close()
			break
		}
	}
	return err
}

func MustExecute(v *dockerTCPSuite, cmd string, options ...components.ExecuteOption) string {
	r, err := v.Env().Host.Execute(cmd)
	v.Require().NoError(err)
	return r
}

func (v *dockerTCPSuite) TestBasicAgent() {
	const testService = "tracegen-tcp"

	// Wait for agent to be live
	v.T().Log("Waiting for Trace Agent to be live.")
	v.Require().NoError(waitRemotePort(v, 8126))

	// Run Trace Generator
	v.T().Log("Starting Trace Generator.")
	run, rm := dockerRunTraceGen(testService)
	v.Env().Host.MustExecute(rm) // kill any existing leftover container
	v.Env().Host.MustExecute(run)
	defer v.Env().Host.MustExecute(rm)

	v.T().Log("Waiting for traces.")
	v.EventuallyWithTf(func(c *assert.CollectT) {
		traces, err := v.Env().FakeIntake.Client().GetTraces()
		require.NoError(c, err)
		require.NotEmpty(c, traces)

		trace := traces[0]
		require.NoError(c, err)
		assert.Equal(c, v.Env().Agent.Client.Hostname(), trace.HostName)
		assert.Equal(c, trace.Env, "none")
		require.NotEmpty(c, trace.TracerPayloads)

		tp := trace.TracerPayloads[0]
		assert.Equal(c, tp.LanguageName, "go")
		require.NotEmpty(c, tp.Chunks)
		require.NotEmpty(c, tp.Chunks[0].Spans)
		spans := tp.Chunks[0].Spans
		for _, s := range spans {
			assert.Equal(c, s.Service, testService)
			assert.Contains(c, s.Name, "tracegen")
			assert.Contains(c, s.Meta, "language")
			assert.Equal(c, s.Meta["language"], "go")
			assert.Contains(c, s.Metrics, "_sampling_priority_v1")
			if s.ParentID == 0 {
				assert.Equal(c, s.Metrics["_dd.top_level"], float64(1))
				assert.Equal(c, s.Metrics["_top_level"], float64(1))
			}
		}

	}, 2*time.Minute, 10*time.Second, "Failed to find traces with basic properties")
}
