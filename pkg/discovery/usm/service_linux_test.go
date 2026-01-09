// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/envs"
)

func TestResolveWorkingDirRelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	cwdDir := filepath.Join(tmpDir, "cwd")
	pwdDir := filepath.Join(tmpDir, "pwd")
	err := os.Mkdir(cwdDir, 0755)
	require.NoError(t, err)
	err = os.Mkdir(pwdDir, 0755)
	require.NoError(t, err)

	pwdTxt := filepath.Join(pwdDir, "pwd.txt")
	f, err := os.Create(pwdTxt)
	require.NoError(t, err)
	f.Close()
	cwdTxt := filepath.Join(cwdDir, "cwd.txt")
	f, err = os.Create(cwdTxt)
	require.NoError(t, err)
	f.Close()

	f, err = os.Create(filepath.Join(pwdDir, "both.txt"))
	require.NoError(t, err)
	f.Close()
	f, err = os.Create(filepath.Join(cwdDir, "both.txt"))
	require.NoError(t, err)
	f.Close()

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })
	err = os.Chdir(cwdDir)
	require.NoError(t, err)

	self := os.Getpid()

	tests := []struct {
		name     string
		pid      int
		envs     envs.Variables
		filename string
		wantPath string
	}{
		{
			name:     "file exists in procfs cwd",
			pid:      self,
			envs:     envs.Variables{},
			filename: "cwd.txt",
			wantPath: cwdTxt,
		},
		{
			name:     "file exists in PWD env",
			pid:      self,
			envs:     envs.NewVariables(map[string]string{"PWD": pwdDir}),
			filename: "pwd.txt",
			wantPath: pwdTxt,
		},
		{
			name:     "file exists in both",
			pid:      self,
			envs:     envs.NewVariables(map[string]string{"PWD": pwdDir}),
			filename: "both.txt",
			wantPath: filepath.Join(pwdDir, "both.txt"),
		},
		{
			name:     "no working dir candidates",
			pid:      -1,
			envs:     envs.Variables{},
			filename: "nonexistent.txt",
			wantPath: "nonexistent.txt",
		},
		{
			name:     "file doesn't exist but has working dir",
			pid:      self,
			envs:     envs.Variables{},
			filename: "nonexistent.txt",
			wantPath: filepath.Join(cwdDir, "nonexistent.txt"),
		},
		{
			name:     "file doesn't exist but has PWD",
			pid:      self,
			envs:     envs.NewVariables(map[string]string{"PWD": pwdDir}),
			filename: "nonexistent.txt",
			wantPath: filepath.Join(pwdDir, "nonexistent.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewDetectionContext(nil, tt.envs, RealFs{})
			ctx.Pid = tt.pid

			gotPath := ctx.resolveWorkingDirRelativePath(tt.filename)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}
