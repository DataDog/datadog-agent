// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package stat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func runStat(
	t *testing.T,
	args []string,
	statFn func(context.Context, string) (builtins.FileSystemInfo, error),
) (string, string, builtins.Result) {
	t.Helper()

	fs := pflag.NewFlagSet("stat", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	handler := makeFlags(fs)
	require.NoError(t, fs.Parse(args))

	var stdout, stderr bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout:         &stdout,
		Stderr:         &stderr,
		FileSystemStat: statFn,
	}
	result := handler(context.Background(), callCtx, fs.Args())
	return stdout.String(), stderr.String(), result
}

func TestFileSystemOutput(t *testing.T) {
	info := builtins.FileSystemInfo{
		ID:                   0x1234,
		IDAvailable:          true,
		NameMax:              255,
		NameMaxAvailable:     true,
		TypeID:               0xef53,
		TypeIDAvailable:      true,
		TypeName:             "ext4",
		IOBlockSize:          4096,
		FundamentalBlockSize: 4096,
		Blocks:               1000,
		BlocksFree:           400,
		BlocksAvailable:      350,
		Files:                200,
		FilesFree:            150,
		FilesAvailable:       true,
	}

	stdout, stderr, result := runStat(t, []string{"-f", "dir"}, func(_ context.Context, path string) (builtins.FileSystemInfo, error) {
		assert.Equal(t, "dir", path)
		return info, nil
	})

	assert.EqualValues(t, 0, result.Code)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"  File: \"dir\"\n"+
		"    ID: 1234     Namelen: 255     Type: ext4\n"+
		"Block size: 4096       Fundamental block size: 4096\n"+
		"Blocks: Total: 1000       Free: 400        Available: 350\n"+
		"Inodes: Total: 200        Free: 150\n",
		stdout)
}

func TestUnavailableFields(t *testing.T) {
	info := builtins.FileSystemInfo{
		TypeName:             "NTFS",
		IOBlockSize:          4096,
		FundamentalBlockSize: 4096,
		Blocks:               10,
		BlocksFree:           4,
		BlocksAvailable:      3,
	}

	stdout, stderr, result := runStat(t, []string{"--file-system", `C:\data`}, func(context.Context, string) (builtins.FileSystemInfo, error) {
		return info, nil
	})

	assert.EqualValues(t, 0, result.Code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, `File: "C:\\data"`)
	assert.Contains(t, stdout, "ID: -")
	assert.Contains(t, stdout, "Namelen: ?")
	assert.Contains(t, stdout, "Type: NTFS")
	assert.Contains(t, stdout, "Inodes: Total: -")
	assert.Contains(t, stdout, "Free: -")
}

func TestUnknownFileSystemTypeFallsBackToTypeID(t *testing.T) {
	info := builtins.FileSystemInfo{
		TypeID:          0xfeed,
		TypeIDAvailable: true,
	}

	stdout, stderr, result := runStat(t, []string{"-f", "mnt"}, func(context.Context, string) (builtins.FileSystemInfo, error) {
		return info, nil
	})

	assert.EqualValues(t, 0, result.Code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Type: UNKNOWN (0xfeed)")
}

func TestMultipleOperandsContinueAfterErrors(t *testing.T) {
	seen := make([]string, 0, 3)
	stdout, stderr, result := runStat(t, []string{"-f", "first", "bad\nname", "last"}, func(_ context.Context, path string) (builtins.FileSystemInfo, error) {
		seen = append(seen, path)
		if path == "bad\nname" {
			return builtins.FileSystemInfo{}, errors.New("permission denied")
		}
		return builtins.FileSystemInfo{TypeName: "tmpfs"}, nil
	})

	assert.EqualValues(t, 1, result.Code)
	assert.Equal(t, []string{"first", "bad\nname", "last"}, seen)
	assert.Contains(t, stdout, `File: "first"`)
	assert.Contains(t, stdout, `File: "last"`)
	assert.NotContains(t, stdout, "bad\nname")
	assert.Equal(t, "stat: cannot read file system information for \"bad\\nname\": permission denied\n", stderr)
}

func TestPathErrorDoesNotRepeatOrInjectOperand(t *testing.T) {
	path := "file\nchild"
	stdout, stderr, result := runStat(t, []string{"-f", path}, func(context.Context, string) (builtins.FileSystemInfo, error) {
		return builtins.FileSystemInfo{}, &os.PathError{
			Op:   "statfs",
			Path: path,
			Err:  errors.New("not a directory"),
		}
	})

	assert.EqualValues(t, 1, result.Code)
	assert.Empty(t, stdout)
	assert.Equal(t, "stat: cannot read file system information for \"file\\nchild\": not a directory\n", stderr)
}

func TestStandardInputOperandIsRejectedAndProcessingContinues(t *testing.T) {
	calls := 0
	stdout, stderr, result := runStat(t, []string{"-f", "-", "dir"}, func(context.Context, string) (builtins.FileSystemInfo, error) {
		calls++
		return builtins.FileSystemInfo{TypeName: "apfs"}, nil
	})

	assert.EqualValues(t, 1, result.Code)
	assert.Equal(t, 1, calls)
	assert.Contains(t, stdout, `File: "dir"`)
	assert.Equal(t, "stat: using '-' to denote standard input does not work in file system mode\n", stderr)
}

func TestMissingCapability(t *testing.T) {
	stdout, stderr, result := runStat(t, []string{"-f", "file"}, nil)

	assert.EqualValues(t, 1, result.Code)
	assert.Empty(t, stdout)
	assert.Equal(t, "stat: file system status capability not available\n", stderr)
}

func TestHelp(t *testing.T) {
	stdout, stderr, result := runStat(t, []string{"-h"}, nil)

	assert.EqualValues(t, 0, result.Code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: stat -f FILE...")
	assert.Contains(t, stdout, "-f, --file-system")
	assert.Contains(t, stdout, "-h, --help")
	assert.NotContains(t, stdout, "\x00")
}

func TestNoArgumentFlagsRejectExplicitValues(t *testing.T) {
	for _, args := range [][]string{{"--file-system=true"}, {"--help=false"}} {
		fs := pflag.NewFlagSet("stat", pflag.ContinueOnError)
		fs.SetOutput(io.Discard)
		makeFlags(fs)
		err := fs.Parse(args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "flag does not allow an argument")
	}
}
