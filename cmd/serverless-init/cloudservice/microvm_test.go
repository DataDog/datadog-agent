// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
)

const testImageARN = "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image"

func TestIsMicroVM(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	assert.True(t, isMicroVM())
}

func TestIsMicroVMNotSet(t *testing.T) {
	assert.False(t, isMicroVM())
}

func TestMicroVMGetTags(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	m := &MicroVM{}
	tags := m.GetTags()

	assert.Equal(t, "us-east-1", tags["region"])
	assert.Equal(t, "123456789012", tags["account_id"])
	assert.Equal(t, "my-image", tags["image_name"])
	assert.Equal(t, MicroVMOrigin, tags["origin"])
	assert.Equal(t, MicroVMOrigin, tags["_dd.origin"])
	assert.Equal(t, MicroVMResourceType, tags["resource_type"])
	assert.Equal(t, "aws", tags["resource_provider"])
	assert.Equal(t, testImageARN, tags["resource_id"])
	assert.NotContains(t, tags, "microvm_image_arn")
}

func TestMicroVMGetTagsMissingARN(t *testing.T) {
	m := &MicroVM{}
	tags := m.GetTags()
	assert.Equal(t, "unknown", tags["region"])
	assert.Equal(t, "unknown", tags["account_id"])
	assert.Equal(t, "unknown", tags["image_name"])
	assert.Equal(t, "unknown", tags["resource_id"])
	assert.Equal(t, MicroVMResourceType, tags["resource_type"])
	assert.Equal(t, "aws", tags["resource_provider"])
}

func TestMicroVMGetEnhancedMetricTags(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	m := &MicroVM{}
	cloudTags := m.GetTags()
	result := m.GetEnhancedMetricTags(cloudTags)

	assert.Equal(t, "us-east-1", result.Base["region"])
	assert.Equal(t, "123456789012", result.Base["account_id"])
	assert.Equal(t, "my-image", result.Base["image_name"])
	assert.Equal(t, MicroVMResourceType, result.Base["resource_type"])
	assert.Equal(t, "aws", result.Base["resource_provider"])
	assert.Equal(t, testImageARN, result.Base["resource_id"])
	assert.NotContains(t, result.Base, "instance_id", "Base must not carry the high-cardinality instance_id")

	assert.Equal(t, result.Base["region"], result.Usage["region"])
	assert.Equal(t, result.Base["account_id"], result.Usage["account_id"])
	assert.Equal(t, result.Base["resource_type"], result.Usage["resource_type"])
	assert.Equal(t, result.Base["resource_provider"], result.Usage["resource_provider"])
	assert.Equal(t, result.Base["resource_id"], result.Usage["resource_id"])
	assert.NotContains(t, result.Usage, "instance_id", "instance_id is absent until SetInstanceID is called from /launch")
}

func TestMicroVMGetEnhancedMetricTagsMissingARN(t *testing.T) {
	m := &MicroVM{}
	cloudTags := m.GetTags() // ARN not set → all "unknown"
	result := m.GetEnhancedMetricTags(cloudTags)

	assert.Equal(t, "unknown", result.Base["region"])
	assert.Equal(t, "unknown", result.Base["account_id"])
	assert.Equal(t, "unknown", result.Base["resource_id"])
	assert.Equal(t, MicroVMResourceType, result.Base["resource_type"])
	assert.Equal(t, "aws", result.Base["resource_provider"])
	assert.Equal(t, result.Base["resource_id"], result.Usage["resource_id"])
}

// Compile-time guard: *MicroVM must satisfy the CloudService interface,
// including the new Run method.
var _ CloudService = (*MicroVM)(nil)

// TestMicroVM_Run_InitMode_ThreadsChildLiveness verifies that MicroVM.Run in
// init-container mode threads m.child into RunInit so the lifecycle server's
// /ready alive-check reflects the user app's actual state.
func TestMicroVM_Run_InitMode_ThreadsChildLiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "sleep 0.3"}

	child := lifecycle.NewChild()
	m := &MicroVM{child: child}

	var midRunAlive atomic.Bool
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		time.Sleep(100 * time.Millisecond)
		midRunAlive.Store(child.IsAlive())
	}()

	err := m.Run(mode.Conf{SidecarMode: false}, &serverlessInitLog.Config{})
	<-probeDone

	require.NoError(t, err)
	assert.True(t, midRunAlive.Load(), "child must be alive while Run is blocked on the subprocess")
	assert.False(t, child.IsAlive(), "child must be dead after Run returns")
}

func TestParseMicroVMARNWithColonInName(t *testing.T) {
	region, accountID, imageName := parseMicroVMARN("arn:aws:lambda:eu-west-1:999:microvm-image:my:image:v2")
	assert.Equal(t, "eu-west-1", region)
	assert.Equal(t, "999", accountID)
	assert.Equal(t, "my:image:v2", imageName)
}

func TestMicroVMOrigin(t *testing.T) {
	assert.Equal(t, MicroVMOrigin, (&MicroVM{}).GetOrigin())
}

func TestMicroVMMetricPrefix(t *testing.T) {
	assert.Equal(t, MicroVMPrefix, (&MicroVM{}).GetMetricPrefix())
}

// TestIsSupportedArch pins the MicroVM arch allowlist — lives here because
// isSupportedArch is defined in microvm.go.
func TestIsSupportedArch(t *testing.T) {
	for _, arch := range []string{archAMD64, archARM64} {
		assert.True(t, isSupportedArch(arch), "%s must be supported for MicroVM", arch)
	}
	for _, arch := range []string{"386", "mips", "mips64", "riscv64", "s390x", ""} {
		assert.False(t, isSupportedArch(arch), "%s must be unsupported", arch)
	}
}
