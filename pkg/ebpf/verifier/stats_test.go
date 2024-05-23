// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package verifier

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	NewBPFComplexityLimit = 1000000
	OldBPFComplexityLimit = 4000
	EBPFStackLimit        = 512
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	os.Exit(m.Run())
}

func TestBuildVerifierStats(t *testing.T) {

	kversion, err := kernel.HostVersion()
	require.NoError(t, err)

	// TODO: reduce the allows kernel version for this test to 4.15 once the loading on those kernels has been fixed
	if kversion < kernel.VersionCode(5, 2, 0) {
		t.Skipf("Skipping because verifier statistics not available on kernel %s", kversion)
	}

	require.NoError(t, rlimit.RemoveMemlock())

	objectFiles := make(map[string]string)
	directory := ddebpf.NewConfig().BPFDir
	err = filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if strings.Contains(path, "-debug") || !strings.HasSuffix(path, ".o") {
			return nil
		}
		coreFile := filepath.Join(directory, "co-re", d.Name())
		if _, err := os.Stat(coreFile); err == nil {
			objectFiles[d.Name()] = coreFile
			return nil
		}

		// if not co-re file present then save normal path
		if _, ok := objectFiles[d.Name()]; !ok {
			objectFiles[d.Name()] = path
		}
		return nil
	})
	require.NoError(t, err)

	var files []string
	for _, path := range objectFiles {
		files = append(files, path)
	}
	results, failedToLoad, err := BuildVerifierStats(&StatsOptions{ObjectFiles: files})
	stats := results.Stats
	require.NoError(t, err)

	assert.True(t, len(stats) > 0)

	// sanity check, since we should be able to load
	// most of the programs.
	assert.True(t, len(stats) > len(failedToLoad))

	for _, file := range objectFiles {
		objectFileName := strings.ReplaceAll(
			strings.Split(filepath.Base(file), ".")[0], "-", "_",
		)

		bc, err := os.Open(file)
		require.NoError(t, err)
		defer bc.Close()

		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		require.NoError(t, err)

		for _, progSpec := range collectionSpec.Programs {
			// ensure all programs were attempted
			key := fmt.Sprintf("%s/%s", objectFileName, progSpec.Name)
			_, loaded := stats[key]
			_, notLoaded := failedToLoad[key]
			if !(loaded || notLoaded) {
				t.Logf("load not attempted for program %s/%s", objectFileName, progSpec.Name)
				assert.True(t, loaded || notLoaded)
				break
			}
		}
	}

	bpfComplexity := OldBPFComplexityLimit
	if kversion >= kernel.VersionCode(5, 2, 0) {
		bpfComplexity = NewBPFComplexityLimit
	}

	// sanity check the values we can somehow bound
	for _, stat := range stats {
		assert.True(t, stat.StackDepth.Value >= 0 && stat.StackDepth.Value <= EBPFStackLimit)
		assert.True(t, stat.InstructionsProcessedLimit.Value > 0 && stat.InstructionsProcessedLimit.Value <= bpfComplexity)
		assert.True(t, stat.InstructionsProcessed.Value > 0 && stat.InstructionsProcessed.Value <= stat.InstructionsProcessedLimit.Value)
	}
}

func TestParseRegisterState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *RegisterState
	}{
		{
			name:  "SingleScalar",
			input: "R0=inv0",
			expected: &RegisterState{
				Register: 0,
				Live:     "",
				Type:     "scalar",
				Value:    "0",
				Precise:  false,
			},
		},
		{
			name:  "WithOnlyMaxValues",
			input: "R2_w=inv(id=2,smax_value=9223372032559808512,umax_value=18446744069414584320,var_off=(0x0;0xffffffff00000000),s32_min_value=0,s32_max_value=0,u32_max_value=0)",
			expected: &RegisterState{
				Register: 2,
				Live:     "written",
				Type:     "scalar",
				Value:    "[0, 2^63 - 1]",
				Precise:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := singleRegStateRegex.FindStringSubmatch(tt.input)
			result, err := parseRegisterState(parts)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}
