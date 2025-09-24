// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector contains tests for the collector
package collector

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	osVM "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

const pythonCheck = `
import time

from datadog_checks.base import AgentCheck

class MyCustomCheck(AgentCheck):
	def __init__(self, name, init_config, instances):
		super(MyCustomCheck, self).__init__(name, init_config, instances)
		self.id = self.instance.get('id')
		
		print(f"Initializing check {self.id}")
		time.sleep(self.instance.get('init_time'))
		print(f"Check {self.id} initialized")

	def check(self, _):
		print(f"Running check {self.id}")
		time.sleep(self.instance.get('run_time'))
		print(f"Done running check {self.id}")

	def cancel(self):
		print(f"Canceling check {self.id}")
		time.sleep(self.instance.get('stop_time'))
		print(f"Check {self.id} canceled")
`

const pythonCheckConfig = `
init_config:
instances:
  - id: -1
	init_time: 0
	run_time: 0
	stop_time: 0
`

type baseCollectorSuite struct {
	e2e.BaseSuite[environments.DockerHost]

	checksdPath string
}

func (v *baseCollectorSuite) getSuiteOptions(osInstance osVM.Descriptor) []e2e.SuiteOption {
	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithFile(v.checksdPath, pythonCheck, true),
				agentparams.WithIntegration("my_check.d", pythonCheckConfig),
			),
			awshost.WithEC2InstanceOptions(ec2.WithOS(osInstance)),
		),
	))

	return suiteOptions
}

func (v *baseCollectorSuite) TestCollectorParallelContainers() {
	v.testParallelContainers(50)
}

func (v *baseCollectorSuite) testParallelContainers(containerCount int) {
	imgName := "alpine"
	containerPrefix := "test-collector-container"

	// Pull the image first
	err := v.Env().Docker.Client.PullImage(imgName)
	require.NoError(v.T(), err)

	rng := rand.New(rand.NewPCG(12, 10))

	// Start containers in parallel
	v.T().Logf("Running %d containers in parallel...", containerCount)

	var wg sync.WaitGroup
	errChan := make([]error, containerCount)
	for i := range containerCount {
		wg.Add(1)
		go func(i int, rng *rand.Rand) {
			defer wg.Done()

			containerName := fmt.Sprintf("%s-%d", containerPrefix, i)

			initialWaitTime := time.Duration(rng.Float64() * 3 * float64(time.Second))
			runWaitTime := time.Duration(rng.Float64() * 3 * float64(time.Second))
			stopWaitTime := time.Duration(rng.Float64() * 3 * float64(time.Second))
			checkInitTime := time.Duration(rng.Float64() * 1 * float64(time.Second))
			checkRunTime := time.Duration(rng.Float64() * 1 * float64(time.Second))
			checkStopTime := time.Duration(rng.Float64() * 1 * float64(time.Second))

			// Create autodiscovery labels for MyCustomCheck
			autodiscoveryLabels := map[string]string{
				"com.datadoghq.ad.checks": fmt.Sprintf(`{"MyCustomCheck": {"instances": [{"id": "%%host%%", "init_time": %f, "run_time": %f, "stop_time": %f}]}}`, checkInitTime.Seconds(), checkRunTime.Seconds(), checkStopTime.Seconds()),
				"com.datadoghq.ad.logs":   `[{"source": "test", "service": "test-service"}]`,
			}

			time.Sleep(time.Duration(initialWaitTime))

			config := &container.Config{
				Image:  imgName,
				Cmd:    []string{"sleep", "300"}, // Sleep for 5 minutes
				Labels: autodiscoveryLabels,
			}

			id, err := v.Env().Docker.Client.RunContainer(containerName, config)
			if err != nil {
				errChan[i] = fmt.Errorf("failed to start container %s: %w", containerName, err)
				return
			}

			defer func() {
				time.Sleep(time.Duration(stopWaitTime))
				err := v.Env().Docker.Client.RemoveContainer(id)
				if err != nil && errChan[i] == nil {
					errChan[i] = fmt.Errorf("failed to remove container %s: %w", containerName, err)
				}
			}()

			time.Sleep(time.Duration(runWaitTime))
			err = v.Env().Docker.Client.StopContainer(id)
			if err != nil {
				errChan[i] = fmt.Errorf("failed to stop container %s: %w", containerName, err)
			}
		}(i, rand.New(rand.NewPCG(rng.Uint64(), rng.Uint64())))
	}

	wg.Wait()

	for i, err := range errChan {
		assert.NoError(v.T(), err, "routine %d", i)
	}
}
