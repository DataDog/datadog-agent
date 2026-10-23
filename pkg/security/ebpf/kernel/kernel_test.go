// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBPFInLSMList(t *testing.T) {
	assert.True(t, isBPFInLSMList("lockdown,capability,landlock,yama,apparmor,bpf\n"))
	assert.True(t, isBPFInLSMList("bpf"))
	assert.False(t, isBPFInLSMList("lockdown,capability,landlock,yama,apparmor,ima,evm\n"))
	assert.False(t, isBPFInLSMList("lockdown,capability,bpffoo"))
	assert.False(t, isBPFInLSMList(""))
}
