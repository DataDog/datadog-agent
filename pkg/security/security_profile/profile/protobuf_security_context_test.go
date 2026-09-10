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

func TestSecDumpSecurityContextRoundtrip(t *testing.T) {
	yes, no := true, false
	in := newProfileWithSelector(t)
	in.SecurityContext = &securitycontext.SecurityContext{
		Privileged: true,
		Seccomp: &securitycontext.SeccompProfile{
			Type:             securitycontext.SeccompLocalhost,
			LocalhostProfile: "profiles/audit.json",
		},
		CapabilitiesAdd:          []string{"NET_ADMIN", "SYS_PTRACE"},
		CapabilitiesDrop:         []string{"MKNOD"},
		RunAsNonRoot:             &yes,
		AllowPrivilegeEscalation: &no,
		ReadOnlyRootFilesystem:   &yes,
	}

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.NotNil(t, out.SecurityContext)
	assert.Equal(t, in.SecurityContext.Privileged, out.SecurityContext.Privileged)
	assert.Equal(t, in.SecurityContext.CapabilitiesAdd, out.SecurityContext.CapabilitiesAdd)
	assert.Equal(t, in.SecurityContext.CapabilitiesDrop, out.SecurityContext.CapabilitiesDrop)
	require.NotNil(t, out.SecurityContext.Seccomp)
	assert.Equal(t, securitycontext.SeccompLocalhost, out.SecurityContext.Seccomp.Type)
	assert.Equal(t, "profiles/audit.json", out.SecurityContext.Seccomp.LocalhostProfile)
	require.NotNil(t, out.SecurityContext.RunAsNonRoot)
	assert.True(t, *out.SecurityContext.RunAsNonRoot)
	require.NotNil(t, out.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *out.SecurityContext.AllowPrivilegeEscalation,
		"explicit false must round-trip as *bool(false), not be flattened into nil")
	require.NotNil(t, out.SecurityContext.ReadOnlyRootFilesystem)
	assert.True(t, *out.SecurityContext.ReadOnlyRootFilesystem)
}

func TestSecDumpSecurityContextTriStateAbsent(t *testing.T) {
	// If the pod spec never set the three tri-state fields, we must not
	// synthesize a false value on the wire — the profile would otherwise
	// claim a stance the workload never took.
	in := newProfileWithSelector(t)
	in.SecurityContext = &securitycontext.SecurityContext{}

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.NotNil(t, out.SecurityContext)
	assert.Nil(t, out.SecurityContext.RunAsNonRoot)
	assert.Nil(t, out.SecurityContext.AllowPrivilegeEscalation)
	assert.Nil(t, out.SecurityContext.ReadOnlyRootFilesystem)
}

func TestSecurityProfileSecurityContextRoundtrip(t *testing.T) {
	in := newProfileWithSelector(t)
	in.SecurityContext = &securitycontext.SecurityContext{
		Seccomp:         &securitycontext.SeccompProfile{Type: securitycontext.SeccompRuntimeDefault},
		CapabilitiesAdd: []string{"NET_ADMIN"},
	}

	buf, err := in.EncodeSecurityProfileProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
	require.NotNil(t, out.SecurityContext)
	assert.False(t, out.SecurityContext.Privileged)
	assert.Equal(t, []string{"NET_ADMIN"}, out.SecurityContext.CapabilitiesAdd)
	assert.Nil(t, out.SecurityContext.CapabilitiesDrop)
	require.NotNil(t, out.SecurityContext.Seccomp)
	assert.Equal(t, securitycontext.SeccompRuntimeDefault, out.SecurityContext.Seccomp.Type)
	assert.Empty(t, out.SecurityContext.Seccomp.LocalhostProfile)
}

func TestSecurityContextNilRoundtrip(t *testing.T) {
	t.Run("secdump", func(t *testing.T) {
		in := newProfileWithSelector(t)
		buf, err := in.EncodeSecDumpProtobuf()
		require.NoError(t, err)
		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
		assert.Nil(t, out.SecurityContext)
	})
	t.Run("securityprofile", func(t *testing.T) {
		in := newProfileWithSelector(t)
		buf, err := in.EncodeSecurityProfileProtobuf()
		require.NoError(t, err)
		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
		assert.Nil(t, out.SecurityContext)
	})
}

// TestSecurityContextEncoderShape guards the proto tag mapping.
func TestSecurityContextEncoderShape(t *testing.T) {
	got := securityContextToProto(&securitycontext.SecurityContext{
		Privileged: true,
		Seccomp: &securitycontext.SeccompProfile{
			Type:             securitycontext.SeccompLocalhost,
			LocalhostProfile: "audit.json",
		},
		CapabilitiesAdd:  []string{"NET_ADMIN"},
		CapabilitiesDrop: []string{"MKNOD"},
	})
	lp := "audit.json"
	assert.Equal(t, &adprotov1.SecurityContext{
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
