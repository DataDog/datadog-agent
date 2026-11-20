// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import "github.com/stretchr/testify/assert"

// assertMacosReportsAbsence requires the remote command to have run and reported
// the absence marker. An SSH failure, a sudo denial, or a missing binary is not
// an absence.
func assertMacosReportsAbsence(t assert.TestingT, out string, err error, marker string) {
	if !assert.Error(t, err) {
		return
	}
	text := out + "\n" + err.Error()
	assert.NotContains(t, text, "failed to create session:")
	assert.NotContains(t, text, "sudo:")
	assert.NotContains(t, text, "command not found")
	assert.Contains(t, text, marker)
}
