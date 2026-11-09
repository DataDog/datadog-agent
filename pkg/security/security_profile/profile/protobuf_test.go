// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package profile

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

func TestUnattributedActivityRoundtrip(t *testing.T) {
	in := newProfileWithSelector(t)
	in.observedRollups = true
	in.versionContexts["v1"] = &VersionContext{}
	v1 := in.ActivityTree.GetOrInsertImageTag("v1")
	now := time.Now()
	ua := &activity_tree.ProcessNode{
		NodeBase: activity_tree.NewNodeBase(),
		Syscalls: map[int]*activity_tree.SyscallNode{42: activity_tree.NewSyscallNode(42, now, v1, activity_tree.Runtime)},
	}
	ua.AppendImageTagID(v1, now)
	in.ActivityTree.UnattributedActivity = ua

	assertDecoded := func(t *testing.T, out *Profile) {
		t.Helper()
		require.NotNil(t, out.ActivityTree.UnattributedActivity)
		assert.Empty(t, out.ActivityTree.ProcessNodes)
		assert.True(t, out.ActivityTree.UnattributedActivity.HasImageTag(out.ActivityTree.GetImageTagID("v1")))
		assert.Equal(t, []uint32{42}, out.ActivityTree.SyscallsByImageTagID()[out.ActivityTree.GetImageTagID("v1")])
	}

	t.Run("secdump", func(t *testing.T) {
		buf, err := in.EncodeSecDumpProtobuf()
		require.NoError(t, err)

		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecDumpProtobuf(bytes.NewReader(buf.Bytes())))
		assertDecoded(t, out)
	})

	t.Run("security profile", func(t *testing.T) {
		pp, err := profileToSecurityProfileProto(in)
		require.NoError(t, err)
		assert.Equal(t, []uint32{42}, pp.ProfileContexts["v1"].Syscalls, "the syscalls of a version include the unattributed ones")

		buf, err := in.EncodeSecurityProfileProtobuf()
		require.NoError(t, err)

		out := newProfileWithSelector(t)
		require.NoError(t, out.DecodeSecurityProfileProtobuf(bytes.NewReader(buf.Bytes())))
		assertDecoded(t, out)
	})
}
