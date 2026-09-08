// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package profile

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adprotov1 "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/securitycontext"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

func newProfileWithSelector(t *testing.T) *Profile {
	t.Helper()
	p := New()
	sel, err := cgroupModel.NewWorkloadSelector("nginx", "1.2.3")
	require.NoError(t, err)
	p.selector = sel
	p.ActivityTree = activity_tree.NewActivityTree(p, nil, "test")
	return p
}

func TestSecDumpDeclaredRoundtrip(t *testing.T) {
	in := newProfileWithSelector(t)
	in.Declared = &securitycontext.Declared{
		Privileged: true,
		Seccomp: &securitycontext.SeccompProfile{
			Type:             securitycontext.SeccompLocalhost,
			LocalhostProfile: "profiles/audit.json",
		},
		CapabilitiesAdd:  []string{"NET_ADMIN", "SYS_PTRACE"},
		CapabilitiesDrop: []string{"MKNOD"},
	}

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.NotNil(t, out.Declared)
	assert.Equal(t, in.Declared.Privileged, out.Declared.Privileged)
	assert.Equal(t, in.Declared.CapabilitiesAdd, out.Declared.CapabilitiesAdd)
	assert.Equal(t, in.Declared.CapabilitiesDrop, out.Declared.CapabilitiesDrop)
	require.NotNil(t, out.Declared.Seccomp)
	assert.Equal(t, securitycontext.SeccompLocalhost, out.Declared.Seccomp.Type)
	assert.Equal(t, "profiles/audit.json", out.Declared.Seccomp.LocalhostProfile)
}

func TestSecurityProfileDeclaredRoundtrip(t *testing.T) {
	in := newProfileWithSelector(t)
	in.Declared = &securitycontext.Declared{
		Seccomp:         &securitycontext.SeccompProfile{Type: securitycontext.SeccompRuntimeDefault},
		CapabilitiesAdd: []string{"NET_ADMIN"},
	}

	buf, err := in.EncodeSecurityProfileProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
	require.NotNil(t, out.Declared)
	assert.False(t, out.Declared.Privileged)
	assert.Equal(t, []string{"NET_ADMIN"}, out.Declared.CapabilitiesAdd)
	assert.Nil(t, out.Declared.CapabilitiesDrop)
	require.NotNil(t, out.Declared.Seccomp)
	assert.Equal(t, securitycontext.SeccompRuntimeDefault, out.Declared.Seccomp.Type)
	assert.Empty(t, out.Declared.Seccomp.LocalhostProfile)
}

func TestDeclaredNilRoundtrip(t *testing.T) {
	t.Run("secdump", func(t *testing.T) {
		in := newProfileWithSelector(t)
		buf, err := in.EncodeSecDumpProtobuf()
		require.NoError(t, err)
		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
		assert.Nil(t, out.Declared)
	})
	t.Run("securityprofile", func(t *testing.T) {
		in := newProfileWithSelector(t)
		buf, err := in.EncodeSecurityProfileProtobuf()
		require.NoError(t, err)
		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
		assert.Nil(t, out.Declared)
	})
}

// TestDeclaredEncoderShape guards the proto tag mapping.
func TestDeclaredEncoderShape(t *testing.T) {
	got := declaredToProto(&securitycontext.Declared{
		Privileged: true,
		Seccomp: &securitycontext.SeccompProfile{
			Type:             securitycontext.SeccompLocalhost,
			LocalhostProfile: "audit.json",
		},
		CapabilitiesAdd:  []string{"NET_ADMIN"},
		CapabilitiesDrop: []string{"MKNOD"},
	})
	lp := "audit.json"
	assert.Equal(t, &adprotov1.HardeningDeclared{
		Privileged: true,
		Seccomp: &adprotov1.SeccompProfile{
			Type:             adprotov1.SeccompProfile_TYPE_LOCALHOST,
			LocalhostProfile: &lp,
		},
		CapabilitiesAdd:  []string{"NET_ADMIN"},
		CapabilitiesDrop: []string{"MKNOD"},
	}, got)
}

func TestSeccompLocalhostPathDroppedForNonLocalhost(t *testing.T) {
	got := seccompToProto(&securitycontext.SeccompProfile{
		Type:             securitycontext.SeccompRuntimeDefault,
		LocalhostProfile: "bogus.json",
	})
	require.NotNil(t, got)
	assert.Equal(t, adprotov1.SeccompProfile_TYPE_RUNTIME_DEFAULT, got.GetType())
	assert.Nil(t, got.LocalhostProfile)
}
