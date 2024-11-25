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

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	"github.com/kr/pretty"
)

func TestAnalyzeBinary(t *testing.T) {
	curDir, err := pwd()
	if err != nil {
		t.Error(err)
	}

	binPath, err := testutil.BuildGoBinaryWrapper(curDir, "../testutil/sample/sample_service")
	if err != nil {
		t.Error(err)
	}

	f, err := safeelf.Open(binPath)
	if err != nil {
		t.Error(err)
	}

	pretty.Log(x.TypeMap.Functions)
}

// func TestBinaryInspection(t *testing.T) {

// 	testFunctions := []string{
// 		"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_single_string",
// 		"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_nonembedded_struct",
// 		"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_struct",
// 		"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.test_uint_slice",
// 	}

// 	curDir, err := pwd()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	binPath, err := testutil.BuildGoBinaryWrapper(curDir, "../testutil/sample/sample_service")
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	f, err := elf.Open(binPath)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	result, err := bininspect.InspectWithDWARF(f, testFunctions, nil)
// 	if err != nil {
// 		t.Error(">", err)
// 	}

// 	for _, funcMetadata := range result.Functions {
// 		for paramName, paramMeta := range funcMetadata.Parameters {
// 			for _, piece := range paramMeta.Pieces {
// 				pretty.Log(paramName, piece)
// 			}
// 		}
// 	}
// }

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
