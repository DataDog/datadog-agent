// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

const (
	// customAgentSID stands in for a non-default Agent service account.
	customAgentSID = "S-1-5-21-1111111111-2222222222-3333333333-1234"

	// fileAllAccess is FILE_ALL_ACCESS, the SDDL "FA" right.
	fileAllAccess windows.ACCESS_MASK = 0x1f01ff
)

// daclGrants maps each SID in the SDDL's DACL to the access it allows.
func daclGrants(t *testing.T, sddl string) map[string]windows.ACCESS_MASK {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	require.NoError(t, err)
	control, _, err := sd.Control()
	require.NoError(t, err)
	assert.NotZero(t, control&windows.SE_DACL_PROTECTED, "the DACL must not inherit entries")
	dacl, _, err := sd.DACL()
	require.NoError(t, err)

	grants := make(map[string]windows.ACCESS_MASK, dacl.AceCount)
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(dacl, i, &ace))
		require.Equal(t, uint8(windows.ACCESS_ALLOWED_ACE_TYPE), ace.Header.AceType)
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		grants[sid.String()] = ace.Mask
	}
	return grants
}

func TestPipeSecurityDescriptorGrantsOnlyIntendedAccounts(t *testing.T) {
	sddl, err := formatPipeSecurityDescriptor(customAgentSID)
	require.NoError(t, err)

	const (
		localSystem    = "S-1-5-18"
		administrators = "S-1-5-32-544"
		ownerRights    = "S-1-3-4"
	)
	grants := daclGrants(t, sddl)
	assert.Equal(t, map[string]windows.ACCESS_MASK{
		localSystem:    fileAllAccess,
		administrators: fileAllAccess,
		ownerRights:    pipeServerAccess,
		customAgentSID: pipeClientAccess,
	}, grants)

	assert.Zero(t, grants[customAgentSID]&fileCreatePipeInstance,
		"the Agent account must not be able to create pipe instances")
	assert.Zero(t, grants[customAgentSID]&(windows.WRITE_DAC|windows.WRITE_OWNER))
	assert.NotZero(t, grants[ownerRights]&fileCreatePipeInstance,
		"sanity: the creator needs it for go-winio to accept connections")
}

func TestPipeSecurityDescriptorRejectsUnsafeAccounts(t *testing.T) {
	for _, sid := range []string{"S-1-1-0", "S-1-5-7", "S-1-5-11", "S-1-5-32-545", "S-1-5-32-546"} {
		t.Run(sid, func(t *testing.T) {
			_, err := formatPipeSecurityDescriptor(sid)
			assert.Error(t, err)
		})
	}
}

func TestListenFailsClosedWhenAgentAccountIsUnresolved(t *testing.T) {
	for name, resolve := range map[string]func() (*windows.SID, error){
		"error":   func() (*windows.SID, error) { return nil, errors.New("service not installed") },
		"nil SID": func() (*windows.SID, error) { return nil, nil },
	} {
		t.Run(name, func(t *testing.T) {
			address := testListenAddr(t)
			useAgentAccount(t, resolve)

			lis, err := Listen(address)
			require.Error(t, err)
			assert.Nil(t, lis)

			// No pipe, permissive or otherwise, may have been created.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = Dial(ctx, address, time.Second)
			assert.ErrorIs(t, err, windows.ERROR_FILE_NOT_FOUND)
		})
	}
}

// TestListenServesClientWithParControlAccess checks the pipe accepts repeated
// connections from the intended identity opening it with par-control's
// narrowed access. Each Accept makes go-winio create a further pipe instance,
// so this also proves the creator can do so under the restricted DACL.
func TestListenServesClientWithParControlAccess(t *testing.T) {
	address := testListenAddr(t)
	lis, err := Listen(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	const connections = 3
	accepted := make(chan error, connections)
	go func() {
		for i := 0; i < connections; i++ {
			conn, err := lis.Accept()
			if err != nil {
				accepted <- err
				return
			}
			_, err = fmt.Fprintf(conn, "ok %d", i)
			// Wait for the client to hang up so no unread data is discarded.
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
			accepted <- err
		}
	}()

	for i := 0; i < connections; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := Dial(ctx, address, 2*time.Second)
		cancel()
		require.NoError(t, err)
		want := fmt.Sprintf("ok %d", i)
		got := make([]byte, len(want))
		_, err = io.ReadFull(conn, got)
		_ = conn.Close()
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
		require.NoError(t, <-accepted)
	}
}
