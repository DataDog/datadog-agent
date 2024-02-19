// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
)

func TestEmbeddedBTFMatch(t *testing.T) {
	cd, err := curDir()
	require.NoError(t, err)
	loader := initBTFLoader(&Config{})
	loader.embeddedDir = filepath.Join(cd, "testdata")

	tests := []struct {
		platform                       btfPlatform
		platformVersion, kernelVersion string
		expectedPath                   string
		err                            bool
	}{
		// correct
		{platformAmazon, "2", "4.14.320-242.534.amzn2.aarch64", "amzn/4.14.320-242.534.amzn2.aarch64.btf.tar.xz", false},
		// unique BTF path, but wrong platform and version
		{platformUbuntu, "22.04", "4.14.320-242.534.amzn2.aarch64", "amzn/4.14.320-242.534.amzn2.aarch64.btf.tar.xz", false},
		// ubuntu, unique, but wrong platform version
		{platformUbuntu, "22.04", "4.15.0-1029-aws", "ubuntu/18.04/4.15.0-1029-aws.btf.tar.xz", false},
		// multiple BTFs in 18.04 and 20.04, unable to narrow down
		{platformUbuntu, "22.04", "5.4.0-80-generic", "", true},
		// non-existent kernel version
		{platformUbuntu, "22.04", "15.0", "", true},
		// kernel is in testdata for centos/7 and ubuntu/22.04, so multiple matches, but platform filter
		{platformRedhat, "7", "3.10.0-1062.0.0.0.1.el7.x86_64", "centos/3.10.0-1062.0.0.0.1.el7.x86_64.btf.tar.xz", false},
	}

	for i, test := range tests {
		path, err := loader.getEmbeddedBTF(test.platform, test.platformVersion, test.kernelVersion)
		if test.err {
			assert.Error(t, err, i)
		} else {
			if assert.NoError(t, err, i) {
				assert.Equal(t, test.expectedPath, path, i)
			}
		}
	}
}

func TestBTFTelemetry(t *testing.T) {
	loader := initBTFLoader(NewConfig())
	spec, result, err := loader.Get()
	require.NoError(t, err)
	require.NotNil(t, spec)
	require.NotEqual(t, ebpftelemetry.COREResult(ebpftelemetry.BtfNotFound), result)
}

func curDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
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
