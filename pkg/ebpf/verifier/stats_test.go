// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package verifier

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
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
		logLevel = "debug"
	}
	log.SetupLogger(seelog.Default, logLevel)
	os.Exit(m.Run())
}

func TestBuildVerifierStats(t *testing.T) {
	var objectFiles []string

	kversion, err := kernel.HostVersion()
	require.NoError(t, err)

	// TODO: reduce the allows kernel version for this test to 4.15 once the loading on those kernels has been fixed
	if kversion < kernel.VersionCode(5, 2, 0) {
		t.Skipf("Skipping because verifier statistics not available on kernel %s", kversion)
	}

	err = rlimit.RemoveMemlock()
	require.NoError(t, err)

	err = filepath.WalkDir(filepath.Join(os.Getenv("DD_SYSTEM_PROBE_BPF_DIR"), "co-re"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if strings.Contains(path, "-debug") || !strings.HasSuffix(path, ".o") {
			return nil
		}
		objectFiles = append(objectFiles, path)

		return nil
	})
	require.NoError(t, err)

	stats, failedToLoad, err := BuildVerifierStats(objectFiles)
	require.NoError(t, err)

	require.True(t, len(stats) > 0)

	// sanity check, since we should be able to load
	// most of the programs.
	require.True(t, len(stats) > len(failedToLoad))

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
			key := programKey(progSpec.Name, objectFileName)
			_, loaded := stats[key]
			_, notLoaded := failedToLoad[key]
			if !(loaded || notLoaded) {
				t.Logf("load not attempted for program %s/%s", objectFileName, progSpec.Name)
				require.True(t, loaded || notLoaded)
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
		if kversion >= kernel.VersionCode(5, 2, 0) {
			require.True(t, stat.VerificationTime.Value > 0)
		}
		require.True(t, stat.StackDepth.Value >= 0 && stat.StackDepth.Value <= EBPFStackLimit)
		require.True(t, stat.InstructionsProcessedLimit.Value > 0 && stat.InstructionsProcessedLimit.Value <= bpfComplexity)
		require.True(t, stat.InstructionsProcessed.Value > 0 && stat.InstructionsProcessed.Value <= stat.InstructionsProcessedLimit.Value)
	}
}
