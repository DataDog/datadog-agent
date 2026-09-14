// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package agentruntimes contains end-to-end tests for Agent runtime behavior.
package agentruntimes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	serverlessInitRemotePath = "/tmp/serverless-init-e2e"
	microVMImageARN          = "arn:aws:lambda:us-east-1:123456789012:microvm-image:e2e"
	fakeIntakeAPIKey         = "00000000000000000000000000000000"
)

type serverlessInitMicroVMSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestServerlessInitMicroVM(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &serverlessInitMicroVMSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204)),
			),
		),
	))
}

func (s *serverlessInitMicroVMSuite) TestEnhancedCPUMetricsReachFakeIntake() {
	intake := s.Env().FakeIntake.Client()
	require.NoError(s.T(), intake.FlushServerAndResetAggregators())

	localBinary := findServerlessInitRunfile(s.T())
	s.Env().RemoteHost.CopyFile(localBinary, serverlessInitRemotePath)
	s.Env().RemoteHost.MustExecute("chmod 0755 " + serverlessInitRemotePath)

	runScript := fmt.Sprintf(`set -eu
mount --make-rprivate /
if mountpoint -q /sys/fs/cgroup; then
  umount -R /sys/fs/cgroup
fi
awk '$0 ~ / - cgroup(2)? / { found=1 } END { exit found ? 1 : 0 }' /proc/self/mountinfo
exec env DD_API_KEY=%s DD_DD_URL=%s DD_ENHANCED_METRICS=true AWS_LAMBDA_MICROVM_IMAGE_ARN=%s AWS_LAMBDA_MICROVM_IMAGE_VERSION=e2e %s /bin/sh -c 'sleep 8'
`, shellQuote(fakeIntakeAPIKey), shellQuote(s.Env().FakeIntake.URL), shellQuote(microVMImageARN), shellQuote(serverlessInitRemotePath))
	command := "sudo -n unshare --mount --propagation private /bin/sh -c " + shellQuote(runScript)
	s.Env().RemoteHost.MustExecute(command)

	s.EventuallyWithT(func(c *assert.CollectT) {
		usage, err := intake.FilterMetrics("aws.lambda.microvm.enhanced.cpu.usage")
		if !assert.NoError(c, err, "could not query enhanced CPU usage metrics") {
			return
		}
		limit, err := intake.FilterMetrics("aws.lambda.microvm.enhanced.cpu.limit")
		if !assert.NoError(c, err, "could not query enhanced CPU limit metrics") {
			return
		}
		assert.NotEmpty(c, usage, "enhanced CPU usage metric did not reach fakeintake")
		assert.NotEmpty(c, limit, "enhanced CPU limit metric did not reach fakeintake")
	}, 2*time.Minute, 10*time.Second)
}

func findServerlessInitRunfile(t *testing.T) string {
	t.Helper()

	workspace := os.Getenv("TEST_WORKSPACE")
	for _, root := range []string{os.Getenv("RUNFILES_DIR"), os.Getenv("TEST_SRCDIR")} {
		if root == "" {
			continue
		}
		for _, workspaceName := range []string{workspace, "_main", "datadog-agent", ""} {
			candidate := filepath.Join(root, workspaceName, "cmd", "serverless-init", "serverless-init")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	require.FailNow(t, "serverless-init binary was not found in Bazel runfiles")
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
