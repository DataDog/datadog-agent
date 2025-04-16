// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeBinary(t *testing.T) {

	testCases := []struct {
		FuncName           string
		ExpectedParameters []*ditypes.Parameter
	}{
		{
			FuncName: "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_int",
			ExpectedParameters: []*ditypes.Parameter{
				{
					Name:                "x",
					ID:                  "",
					Type:                "int",
					TotalSize:           8,
					Kind:                0x2,
					Location:            &ditypes.Location{InReg: true, StackOffset: 0, Register: 0, NeedsDereference: false, PointerOffset: 0x0},
					LocationExpressions: nil,
					FieldOffset:         0x0,
					NotCaptureReason:    0x0,
					ParameterPieces:     nil,
				},
			},
		},
	}

	for i := range testCases {
		t.Run(testCases[i].FuncName, func(t *testing.T) {

			curDir, err := pwd()
			if err != nil {
				t.Error(err)
			}

			binPath, err := testutil.BuildGoBinaryWrapper(curDir, "../testutil/sample/sample_service")
			if err != nil {
				t.Error(err)
			}

			procInfo := ditypes.ProcessInfo{
				BinaryPath: binPath,
				ProbesByID: func() *ditypes.ProbesByID {
					p := ditypes.NewProbesByID()
					p.Set(testCases[i].FuncName, &ditypes.Probe{
						ServiceName: "sample",
						FuncName:    testCases[i].FuncName,
					})
					return p
				}(),
			}
			err = AnalyzeBinary(&procInfo)
			if err != nil {
				t.Error(err)
			}
			require.Equal(t, testCases[i].ExpectedParameters, procInfo.TypeMap.Functions[testCases[i].FuncName])
		})
	}

}

// pwd returns the current directory of the caller.
func pwd() (string, error) {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}
