package verifier

import (
	"fmt"
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
	BPFComplexityLimit = 1000000
	EBPFStackLimit     = 512
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
	var programs []string

	kversion, err := kernel.HostVersion()
	require.NoError(t, err)

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

	stats, err := BuildVerifierStats(objectFiles)
	require.NoError(t, err)

	for _, file := range objectFiles {
		// skip fentry programs since we cannot load them on some kernels
		if objectFileBase(file) == "tracer_fentry" {
			continue
		}

		bc, err := os.Open(file)
		require.NoError(t, err)
		defer bc.Close()

		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		require.NoError(t, err)

		for _, progSpec := range collectionSpec.Programs {
			programs = append(programs, progSpec.Name)
		}
	}

	// ensure we go verifier stats for all programs
	for _, progName := range programs {
		_, ok := stats[fmt.Sprintf("Func_%s", progName)]
		if !ok {
			require.True(t, ok)
			break
		}
	}

	// sanity check the values we can somehow bound
	for _, stat := range stats {
		require.True(t, stat.VerificationTime > 0)
		require.True(t, stat.StackDepth >= 0 && stat.StackDepth <= EBPFStackLimit)
		require.True(t, stat.InstructionsProcessedLimit > 0 && stat.InstructionsProcessedLimit <= BPFComplexityLimit)
		require.True(t, stat.InstructionsProcessed > 0 && stat.InstructionsProcessed <= stat.InstructionsProcessedLimit)
	}
}
