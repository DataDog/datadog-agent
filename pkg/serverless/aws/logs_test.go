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

	nonEmptyRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	emptyRequestID := ""

	assert.True(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestID, validLog))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestID, validLog))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestID, validLog))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestID, validLog))

	assert.False(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestID, invalidLog))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestID, invalidLog))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestID, invalidLog))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestID, invalidLog))

}
