// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
	assert.NotContains(t, tags, "microvm_image_arn")
}

func TestMicroVMGetTagsMissingARN(t *testing.T) {
	m := &MicroVM{}
	tags := m.GetTags()
	assert.Equal(t, "unknown", tags["region"])
	assert.Equal(t, "unknown", tags["account_id"])
	assert.Equal(t, "unknown", tags["image_name"])
}

func TestMicroVMGetEnhancedMetricTags(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	m := &MicroVM{}
	cloudTags := m.GetTags()
	result := m.GetEnhancedMetricTags(cloudTags)

	assert.Equal(t, "us-east-1", result.Base["region"])
	assert.Equal(t, "123456789012", result.Base["account_id"])
	assert.Equal(t, "my-image", result.Base["image_name"])

	assert.Equal(t, result.Base["region"], result.Usage["region"])
	assert.Equal(t, result.Base["account_id"], result.Usage["account_id"])
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
	assert.Equal(t, microVMPrefix, (&MicroVM{}).GetMetricPrefix())
}
