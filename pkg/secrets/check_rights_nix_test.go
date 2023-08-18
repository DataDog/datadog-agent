// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets && !windows

package secrets

import (
	"os"
	"os/user"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func setCorrectRight(path string) {
	os.Chmod(path, 0700)
}

// testCheckRightsStub is a dummy checkRights stub for *nix
func testCheckRightsStub() {
}

func TestWrongPath(t *testing.T) {
	require.NotNil(t, checkRights("does not exists", false))
}

func TestGroupOtherRights(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "agent-collector-test")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())

	allowGroupExec := false

	// file exists
	require.NotNil(t, checkRights("/does not exists", allowGroupExec))

	require.Nil(t, os.Chmod(tmpfile.Name(), 0700))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// we should at least be able to execute it
	require.Nil(t, os.Chmod(tmpfile.Name(), 0100))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// owner have R&W but not X permission
	require.Nil(t, os.Chmod(tmpfile.Name(), 0600))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// group should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0710))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// other should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0701))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	allowGroupExec = true

	// even if allowGroupExec=true, group may have no permission
	require.Nil(t, os.Chmod(tmpfile.Name(), 0700))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// group can have read and exec permission
	require.Nil(t, os.Chmod(tmpfile.Name(), 0750))
	require.Nil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// group should not have write right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0770))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// other should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0701))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))

	// other should not have write permission
	require.Nil(t, os.Chmod(tmpfile.Name(), 0702))
	require.NotNil(t, checkRights(tmpfile.Name(), allowGroupExec))
}

func Test_checkGroupPermission(t *testing.T) {
	type args struct {
		stat       *syscall.Stat_t
		usr        *user.User
		userGroups []string
		path       string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "User doesn't own the file, but one of his group does => Valid",
			args: args{
				stat: &syscall.Stat_t{
					Uid:  13,
					Gid:  42,
					Mode: 0510,
				},
				usr:        &user.User{},
				userGroups: []string{"1", "42"},
				path:       "/foo",
			},
			wantErr: false,
		},
		{
			name: "User doesn't own the file, one of his group does, but Group doesn't have exec",
			args: args{
				stat: &syscall.Stat_t{
					Uid:  13,
					Gid:  42,
					Mode: 0500,
				},
				usr:        &user.User{},
				userGroups: []string{"1", "42"},
				path:       "/foo",
			},
			wantErr: true,
		},
		{
			name: "User or User'Group doesn't own the file =. Fail",
			args: args{
				stat: &syscall.Stat_t{
					Uid:  13,
					Gid:  5,
					Mode: 0510,
				},
				usr:        &user.User{},
				userGroups: []string{"1", "42"},
				path:       "/foo",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkGroupPermission(tt.args.stat, tt.args.usr, tt.args.userGroups, tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("checkGroupPermission() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
