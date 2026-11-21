// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"errors"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
)

// exitStatuser is the SSH exit error that carries the remote exit status.
type exitStatuser interface {
	error
	ExitStatus() int
}

// Fails to compile if the SSH library's exit error stops satisfying exitStatuser.
var _ exitStatuser = (*ssh.ExitError)(nil)

// assertMacosCommandFailed requires the remote command to have failed and not
// to have been stopped by SSH, sudo, or a missing binary.
func assertMacosCommandFailed(t assert.TestingT, out string, err error) bool {
	if !assert.Error(t, err) {
		return false
	}
	text := out + "\n" + err.Error()
	assert.NotContains(t, text, "failed to create session:")
	assert.NotContains(t, text, "sudo:")
	assert.NotContains(t, text, "command not found")
	return true
}

// assertMacosReportsAbsence requires the remote command to have run and reported
// the absence marker. An SSH failure, a sudo denial, or a missing binary is not
// an absence.
func assertMacosReportsAbsence(t assert.TestingT, out string, err error, marker string) {
	if !assertMacosCommandFailed(t, out, err) {
		return
	}
	assert.Contains(t, out+"\n"+err.Error(), marker)
}

// assertMacosExitStatus requires the remote command to have exited with exactly
// the given status. Matching the status text would also accept 10 to 19, 100 to
// 199, and signal deaths, so the status is read from the SSH exit error.
func assertMacosExitStatus(t assert.TestingT, out string, err error, want int) {
	if !assertMacosCommandFailed(t, out, err) {
		return
	}
	var statusErr exitStatuser
	if !assert.True(t, errors.As(err, &statusErr), "the error carries no exit status: %v", err) {
		return
	}
	assert.Equal(t, want, statusErr.ExitStatus())
}
