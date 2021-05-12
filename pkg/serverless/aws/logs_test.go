// +build !windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldProcessLog(t *testing.T) {

	validLog := LogMessage{
		Type:         LogTypePlatformReport,
		StringRecord: "toto",
	}

	invalidLog := LogMessage{
		Type:         "",
		StringRecord: "",
	}

	nonEmptyARN := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	emptyARN := ""

	nonEmptyRequestId := "8286a188-ba32-4475-8077-530cd35c09a9"
	emptyRequestId := ""

	assert.True(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestId, validLog))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestId, validLog))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestId, validLog))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestId, validLog))

	assert.False(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestId, invalidLog))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestId, invalidLog))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestId, invalidLog))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestId, invalidLog))

}
