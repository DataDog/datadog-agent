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

func frontendWebKey() securitycontext.Key {
	return securitycontext.Key{
		Namespace:     "frontend",
		OwnerKind:     "Deployment",
		OwnerName:     "web",
		ContainerName: "nginx",
	}
}

func networkToolsDebugKey() securitycontext.Key {
	return securitycontext.Key{
		Namespace:     "network-tools",
		OwnerKind:     "DaemonSet",
		OwnerName:     "nginx-debug",
		ContainerName: "nginx",
	}
}

func TestSecDumpSecurityContextRoundtrip(t *testing.T) {
	yes, no := true, false
	in := newProfileWithSelector(t)
	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{
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
	})

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.Len(t, out.SecurityContexts, 1)
	sc := out.SecurityContexts[frontendWebKey()]
	require.NotNil(t, sc)
	assert.True(t, sc.Privileged)
	assert.Equal(t, []string{"NET_ADMIN", "SYS_PTRACE"}, sc.CapabilitiesAdd)
	assert.Equal(t, []string{"MKNOD"}, sc.CapabilitiesDrop)
	require.NotNil(t, sc.Seccomp)
	assert.Equal(t, securitycontext.SeccompLocalhost, sc.Seccomp.Type)
	assert.Equal(t, "profiles/audit.json", sc.Seccomp.LocalhostProfile)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.ReadOnlyRootFilesystem)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
}

func TestSecDumpSecurityContextTriStateAbsent(t *testing.T) {
	in := newProfileWithSelector(t)
	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{})

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.Len(t, out.SecurityContexts, 1)
	sc := out.SecurityContexts[frontendWebKey()]
	require.NotNil(t, sc)
	assert.Nil(t, sc.RunAsNonRoot)
	assert.Nil(t, sc.AllowPrivilegeEscalation)
	assert.Nil(t, sc.ReadOnlyRootFilesystem)
}

func TestSecurityProfileSecurityContextRoundtrip(t *testing.T) {
	in := newProfileWithSelector(t)
	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{
		Seccomp:         &securitycontext.SeccompProfile{Type: securitycontext.SeccompRuntimeDefault},
		CapabilitiesAdd: []string{"NET_ADMIN"},
	})

	buf, err := in.EncodeSecurityProfileProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
	require.Len(t, out.SecurityContexts, 1)
	sc := out.SecurityContexts[frontendWebKey()]
	require.NotNil(t, sc)
	assert.False(t, sc.Privileged)
	assert.Equal(t, []string{"NET_ADMIN"}, sc.CapabilitiesAdd)
	assert.Nil(t, sc.CapabilitiesDrop)
	require.NotNil(t, sc.Seccomp)
	assert.Equal(t, securitycontext.SeccompRuntimeDefault, sc.Seccomp.Type)
	assert.Empty(t, sc.Seccomp.LocalhostProfile)
}

func TestSecurityContextNilRoundtrip(t *testing.T) {
	t.Run("secdump", func(t *testing.T) {
		in := newProfileWithSelector(t)
		buf, err := in.EncodeSecDumpProtobuf()
		require.NoError(t, err)
		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
		assert.Nil(t, out.SecurityContexts)
	})
	t.Run("securityprofile", func(t *testing.T) {
		in := newProfileWithSelector(t)
		buf, err := in.EncodeSecurityProfileProtobuf()
		require.NoError(t, err)
		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
		assert.Nil(t, out.SecurityContexts)
	})
}

func TestSecDumpSecurityContextMultipleEntriesRoundtrip(t *testing.T) {
	in := newProfileWithSelector(t)
	yes := true
	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{
		RunAsNonRoot:           &yes,
		ReadOnlyRootFilesystem: &yes,
		CapabilitiesDrop:       []string{"ALL"},
		CapabilitiesAdd:        []string{"NET_BIND_SERVICE"},
		Seccomp:                &securitycontext.SeccompProfile{Type: securitycontext.SeccompRuntimeDefault},
	})
	in.UpsertSecurityContext(networkToolsDebugKey(), &securitycontext.SecurityContext{
		Privileged:      true,
		CapabilitiesAdd: []string{"NET_ADMIN", "NET_RAW"},
	})

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.Len(t, out.SecurityContexts, 2)

	web := out.SecurityContexts[frontendWebKey()]
	require.NotNil(t, web)
	require.NotNil(t, web.RunAsNonRoot)
	assert.True(t, *web.RunAsNonRoot)
	assert.Equal(t, []string{"NET_BIND_SERVICE"}, web.CapabilitiesAdd)
	assert.False(t, web.Privileged)

	debug := out.SecurityContexts[networkToolsDebugKey()]
	require.NotNil(t, debug)
	assert.True(t, debug.Privileged)
	assert.Equal(t, []string{"NET_ADMIN", "NET_RAW"}, debug.CapabilitiesAdd)
}

func TestSecDumpSecurityContextRepeatedUpsertLastWins(t *testing.T) {
	yes, no := true, false
	in := newProfileWithSelector(t)
	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{
		Privileged:             true,
		ReadOnlyRootFilesystem: &no,
	})
	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{
		Privileged:             false,
		ReadOnlyRootFilesystem: &yes,
	})

	buf, err := in.EncodeSecDumpProtobuf()
	require.NoError(t, err)

	out := newProfileWithSelector(t)
	require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
	require.Len(t, out.SecurityContexts, 1)
	sc := out.SecurityContexts[frontendWebKey()]
	require.NotNil(t, sc)
	assert.False(t, sc.Privileged)
	require.NotNil(t, sc.ReadOnlyRootFilesystem)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
}

func TestUpsertSecurityContextIgnoresZeroKeyAndNilValue(t *testing.T) {
	in := newProfileWithSelector(t)

	in.UpsertSecurityContext(securitycontext.Key{}, &securitycontext.SecurityContext{Privileged: true})
	assert.Nil(t, in.SecurityContexts)

	in.UpsertSecurityContext(frontendWebKey(), nil)
	assert.Nil(t, in.SecurityContexts)

	in.UpsertSecurityContext(frontendWebKey(), &securitycontext.SecurityContext{})
	assert.Len(t, in.SecurityContexts, 1)
}

func TestSecurityContextsToProtoDeterministicOrder(t *testing.T) {
	in := map[securitycontext.Key]*securitycontext.SecurityContext{
		networkToolsDebugKey(): {Privileged: true},
		frontendWebKey():       {},
	}
	got := securityContextsToProto(in)
	require.Len(t, got, 2)
	assert.Equal(t, "frontend", got[0].GetNamespace())
	assert.Equal(t, "network-tools", got[1].GetNamespace())
}

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
