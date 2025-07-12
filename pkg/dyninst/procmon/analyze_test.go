// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func makeEnviron(t *testing.T, keyVals ...string) []byte {
	require.Truef(
		t, len(keyVals)%2 == 0,
		"keyVals must be a multiple of 2, got %d: %v", len(keyVals), keyVals,
	)
	var buf bytes.Buffer
	for i := 0; i < len(keyVals); i += 2 {
		fmt.Fprintf(&buf, "%s=%s\x00", keyVals[i], keyVals[i+1])
	}
	return buf.Bytes()
}

type noopContainerResolver struct{}

func (r noopContainerResolver) GetContainerContext(uint32) (
	containerutils.ContainerID, model.CGroupContext, string, error,
) {
	return "", model.CGroupContext{}, "", nil
}

func TestAnalyzeProcess(t *testing.T) {
	envTrue := makeEnviron(t,
		ddDynInstEnabledEnvVar, "true",
		ddServiceEnvVar, "foo",
	)
	envFalse := makeEnviron(t,
		"FOO", "bar",
	)

	const exeTargetName = "exe_target"
	analyzer := makeExecutableAnalyzer(0)

	// makeProcFS creates a minimal on-disk proc-like structure under a temp dir
	// and returns the path that should be used as procfsRoot when calling
	// buildUpdate.
	makeProcFS := func(
		t *testing.T, pid uint32, env []byte, withExe bool,
	) (
		tmpDir string, procRoot string, cleanup func(),
	) {
		tmpDir = t.TempDir()
		procRoot = filepath.Join(tmpDir, "proc")

		procDir := filepath.Join(procRoot, strconv.Itoa(int(pid)))
		require.NoError(t, os.MkdirAll(procDir, 0o755))

		// /proc/<pid>/environ
		require.NoError(t, os.WriteFile(filepath.Join(procDir, "environ"), env, 0o644))

		if withExe {
			// create a fake executable file and a symlink named "exe" pointing to it
			exeTarget := filepath.Join(tmpDir, exeTargetName)
			require.NoError(t, os.WriteFile(exeTarget, []byte{}, 0o755))
			require.NoError(t, os.Symlink(exeTarget, filepath.Join(procDir, "exe")))
		}

		return tmpDir, procRoot, func() {
			os.RemoveAll(tmpDir)
		}
	}

	t.Run("not interesting env", func(t *testing.T) {
		_, procRoot, cleanup := makeProcFS(t, 102, envFalse, true)
		defer cleanup()
		res, err := analyzeProcess(102, procRoot, noopContainerResolver{}, analyzer)
		require.NoError(t, err)
		require.False(t, res.interesting)
	})

	t.Run("no interesting exe", func(t *testing.T) {
		_, procRoot, cleanup := makeProcFS(t, 101, envTrue, true)
		defer cleanup()
		res, err := analyzeProcess(101, procRoot, noopContainerResolver{}, analyzer)
		require.NoError(t, err)
		require.False(t, res.interesting)
		require.Empty(t, res.exe)
	})

	t.Run("exe missing", func(t *testing.T) {
		_, procRoot, cleanup := makeProcFS(t, 103, envTrue, false)
		defer cleanup()
		res, err := analyzeProcess(103, procRoot, noopContainerResolver{}, analyzer)
		require.NoError(t, err)
		require.False(t, res.interesting)
	})

	t.Run("interesting", func(t *testing.T) {
		cfgs := testprogs.MustGetCommonConfigs(t)
		bin := testprogs.MustGetBinary(t, "sample", cfgs[0])

		tmpDir, procRoot, cleanup := makeProcFS(t, 104, envTrue, true)
		defer cleanup()
		exeTarget := filepath.Join(tmpDir, exeTargetName)
		require.NoError(t, os.Remove(exeTarget), "failed to remove exe target", exeTarget)
		{
			f, err := os.Create(exeTarget)
			require.NoError(t, err)
			binReader, err := os.Open(bin)
			require.NoError(t, err)
			_, err = io.Copy(f, binReader)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			require.NoError(t, binReader.Close())
		}
		res, err := analyzeProcess(104, procRoot, noopContainerResolver{}, analyzer)
		require.NoError(t, err)
		require.True(t, res.interesting)
		require.NotEmpty(t, res.exe.Path)
		require.Equal(t, "foo", res.service)
	})
}

func BenchmarkIsGoElfBinaryWithDDTraceGo(b *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(b)
	progs := testprogs.MustGetPrograms(b)
	for _, prog := range progs {
		b.Run(prog, func(b *testing.B) {
			for _, cfg := range cfgs {
				b.Run(cfg.String(), func(b *testing.B) {
					expect := expectationsByProgramName[prog]
					bin := testprogs.MustGetBinary(b, prog, cfg)
					elfFile, err := safeelf.Open(bin)
					require.NoError(b, err)
					idx := slices.IndexFunc(elfFile.Sections, func(s *safeelf.Section) bool {
						return s.Name == ".strtab"
					})
					require.NotEqual(b, -1, idx)
					b.SetBytes(int64(elfFile.Sections[idx].Size))
					require.NoError(b, elfFile.Close())
					benchmarkIsGoElfBinaryWithDDTraceGo(b, bin, expect)
				})
			}
		})
	}
}

var expectationsByProgramName = map[string]bool{
	"sample":       true,
	"rc_tester":    true,
	"rc_tester_v1": true,
}

func benchmarkIsGoElfBinaryWithDDTraceGo(b *testing.B, binPath string, expect bool) {
	f, err := os.Open(binPath)
	require.NoError(b, err)
	defer f.Close()
	b.ResetTimer()

	var got bool
	for b.Loop() {
		got, err = isGoElfBinaryWithDDTraceGo(f)
	}
	b.StopTimer()
	require.NoError(b, err)
	require.Equal(b, expect, got)
}

func BenchmarkAnalyzeProcess(b *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(b)
	progs := testprogs.MustGetPrograms(b)
	for _, prog := range progs {
		b.Run(prog, func(b *testing.B) {
			for _, cfg := range cfgs {
				b.Run(cfg.String(), func(b *testing.B) {
					if cfg.GOARCH != runtime.GOARCH {
						b.Skipf("skipping %s on %s", cfg.String(), runtime.GOARCH)
					}
					for _, cached := range []string{"None", "FileKey", "HtlHash"} {
						b.Run(fmt.Sprintf("cached=%s", cached), func(b *testing.B) {
							for _, env := range []struct {
								name string
								env  []string
							}{
								{
									name: "disabled",
									env:  nil,
								},
								{
									name: "enabled",
									env: []string{
										"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
										"DD_SERVICE=foo",
									},
								},
							} {
								b.Run(fmt.Sprintf("env=%s", env.name), func(b *testing.B) {
									var analyzer executableAnalyzer
									switch cached {
									case "None":
										analyzer = &baseExecutableAnalyzer{}
									case "FileKey":
										analyzer = newFileKeyCacheExecutableAnalyzer(1, &baseExecutableAnalyzer{})
									case "HtlHash":
										analyzer = newHtlHashCacheExecutableAnalyzer(10, &baseExecutableAnalyzer{})
									default:
										b.Fatalf("unknown cache type: %s", cached)
									}
									bin := testprogs.MustGetBinary(b, prog, cfg)
									child := exec.Command(bin)
									child.Env = env.env
									child.Stdout = io.Discard
									child.Stderr = io.Discard
									require.NoError(b, child.Start())
									b.Cleanup(func() {
										_ = child.Process.Kill()
										_, _ = child.Process.Wait()
									})
									pid := child.Process.Pid
									for b.Loop() {
										_, err := analyzeProcess(
											uint32(pid), "/proc", noopContainerResolver{}, analyzer,
										)
										require.NoError(b, err)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}
