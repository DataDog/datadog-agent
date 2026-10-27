// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package btf

import (
	"testing"

	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetKernelFuncProto(t *testing.T) {
	if _, err := GetKernelSpec(); err != nil {
		t.Skipf("no usable kernel BTF: %s", err)
	}

	t.Run("tcp_recvmsg", func(t *testing.T) {
		proto, err := GetKernelFuncProto("tcp_recvmsg")
		require.NoError(t, err)
		// the first parameter is sk in every known signature
		require.NotEmpty(t, proto.Params)
		assert.Equal(t, "sk", proto.Params[0].Name)
	})

	t.Run("unknown function", func(t *testing.T) {
		_, err := GetKernelFuncProto("datadog_agent_no_such_kernel_function")
		require.ErrorIs(t, err, btf.ErrNotFound)
	})
}

func TestGetKernelFuncArgCount(t *testing.T) {
	if _, err := GetKernelSpec(); err != nil {
		t.Skipf("no usable kernel BTF: %s", err)
	}

	t.Run("tcp_close", func(t *testing.T) {
		n, err := GetKernelFuncArgCount("tcp_close")
		require.NoError(t, err)
		// tcp_close(struct sock *sk, long timeout) has kept this signature
		// across every supported kernel
		assert.Equal(t, 2, n)
	})

	t.Run("unknown function", func(t *testing.T) {
		_, err := GetKernelFuncArgCount("datadog_agent_no_such_kernel_function")
		require.ErrorIs(t, err, btf.ErrNotFound)
	})
}
