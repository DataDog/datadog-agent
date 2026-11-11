// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOTelRegistrationOnlyDroppedBySuccessfulExec(t *testing.T) {
	for _, nr := range []int{ExecveNr, ExecveatNr} {
		ctx := &CWSPtracerCtx{processCache: NewProcessCache()}
		process := NewProcess(-1)
		process.Pid, process.Tgid = 10, 10
		ctx.processCache.Add(process.Pid, process)
		ctx.processCache.setOTel(process.Tgid, &otelState{})

		ctx.handlePostHooks(nr, 0, syscall.PtraceRegs{}, process, syscallHandler{})
		assert.NotNil(t, ctx.processCache.getOTel(process.Tgid))

		ctx.unsetSpanContextOnExec(process.Pid)
		assert.Nil(t, ctx.processCache.getOTel(process.Tgid))
	}
}
